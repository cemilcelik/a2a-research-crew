// Package sqlitemcp, competitor-analyst ajanına izole bir SQL çalışma alanı
// sunan in-process bir MCP sunucusu üretir. Veritabanı olarak sqlite kullanılır;
// böylece LLM'in ürettiği SQL, uygulamanın operasyonel PostgreSQL verisine
// erişemez (belirgin izolasyon).
package sqlitemcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"a2a-research-crew/internal/config"
	"a2a-research-crew/internal/mcpx"
)

// Araç adları.
const (
	ToolRunSQL     = "run_sql"
	ToolListTables = "list_tables"
)

// maxRows, tek bir sorguda modele döndürülecek en fazla satır sayısıdır.
const maxRows = 200

// New, izole sqlite veritabanı üzerinde çalışan bir MCP istemcisi ve kapatma
// fonksiyonu döner.
func New(ctx context.Context, cfg *config.Config) (*mcpx.Client, func(), error) {
	if err := os.MkdirAll(cfg.MCP.SQLiteDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("sqlitemcp: create sqlite dir: %w", err)
	}
	dbPath := filepath.Join(cfg.MCP.SQLiteDir, "competitor-analyst.sqlite")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlitemcp: open database: %w", err)
	}
	// sqlite için tek bağlantı, kilit hatalarını önler ve davranışı öngörülebilir
	// kılar.
	db.SetMaxOpenConns(1)

	client, cleanup, err := mcpx.NewInProcessServer(ctx, "sqlite", []mcpx.InProcessTool{
		{
			Name:        ToolRunSQL,
			Description: "Runs a SQL statement on the isolated analytics database. SELECT-like statements return JSON rows; others return the number of affected rows.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql": map[string]any{"type": "string", "description": "the SQL statement to execute"},
				},
				"required": []any{"sql"},
			},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				return runSQL(ctx, db, args)
			},
		},
		{
			Name:        ToolListTables,
			Description: "Lists tables and their columns in the analytics database.",
			Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(ctx context.Context, _ map[string]any) (string, error) {
				return listTables(ctx, db)
			},
		},
	})
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	return client, func() {
		cleanup()
		_ = db.Close()
	}, nil
}

func runSQL(ctx context.Context, db *sql.DB, args map[string]any) (string, error) {
	statement, _ := args["sql"].(string)
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return "", fmt.Errorf("sql is required")
	}

	if isQuery(statement) {
		rows, err := db.QueryContext(ctx, statement)
		if err != nil {
			return "", fmt.Errorf("query failed: %w", err)
		}
		defer rows.Close()

		result, err := rowsToMaps(rows)
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{"rows": result, "count": len(result)})
	}

	res, err := db.ExecContext(ctx, statement)
	if err != nil {
		return "", fmt.Errorf("exec failed: %w", err)
	}
	affected, _ := res.RowsAffected()
	return marshal(map[string]any{"affectedRows": affected})
}

func listTables(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return "", fmt.Errorf("list tables failed: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	tables := make([]map[string]any, 0, len(names))
	for _, name := range names {
		columns, err := tableColumns(ctx, db, name)
		if err != nil {
			return "", err
		}
		tables = append(tables, map[string]any{"name": name, "columns": columns})
	}
	return marshal(map[string]any{"tables": tables})
}

func tableColumns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	// Tablo adı sqlite_master'dan geldiği için tanımlayıcı olarak güvenli kabul
	// edilir; yine de tırnaklayarak enjekte riskini kapatırız.
	quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoted+")")
	if err != nil {
		return nil, fmt.Errorf("table info failed for %s: %w", table, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var (
			cid       int
			name, typ string
			notnull   int
			dflt      any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

func isQuery(statement string) bool {
	keyword := strings.ToUpper(firstToken(statement))
	switch keyword {
	case "SELECT", "WITH", "PRAGMA", "EXPLAIN":
		return true
	default:
		return false
	}
}

func firstToken(statement string) string {
	fields := strings.Fields(statement)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func rowsToMaps(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		if len(out) >= maxRows {
			break
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = normalizeValue(values[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case []byte:
		return string(v)
	default:
		return v
	}
}

func marshal(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(raw), nil
}
