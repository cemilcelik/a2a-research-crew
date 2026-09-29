// Package userstore, kullanıcı kimlik bilgilerini PostgreSQL üzerinde bcrypt
// hash'leriyle saklar ve doğrular.
package userstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials, kullanıcı adı veya parola hatalı olduğunda döner.
// Kullanıcı sayımını (enumeration) önlemek için her iki durumda da aynı hata
// döner.
var ErrInvalidCredentials = errors.New("userstore: invalid credentials")

const schemaSQL = `
CREATE TABLE IF NOT EXISTS users (
    username      text PRIMARY KEY,
    password_hash text NOT NULL,
    roles         text[] NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now()
);
`

// dummyHash, kullanıcı bulunamadığında zamanlama farkını azaltmak için
// karşılaştırılan sabit bir bcrypt hash'idir ("invalid-password").
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

// Postgres, kullanıcı deposunun PostgreSQL uygulamasıdır.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres, verilen DSN ile bağlanır.
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("userstore: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("userstore: ping database: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

// Migrate, kullanıcı tablosunu (idempotent olarak) oluşturur.
func (s *Postgres) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("userstore: apply schema: %w", err)
	}
	return nil
}

// Close, bağlantı havuzunu kapatır.
func (s *Postgres) Close() { s.pool.Close() }

// Upsert, bir kullanıcıyı oluşturur veya günceller; parolayı bcrypt ile saklar.
func (s *Postgres) Upsert(ctx context.Context, username, password string, roles []string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("userstore: hash password: %w", err)
	}
	if roles == nil {
		roles = []string{}
	}
	const query = `
INSERT INTO users (username, password_hash, roles)
VALUES ($1, $2, $3)
ON CONFLICT (username) DO UPDATE SET password_hash = EXCLUDED.password_hash, roles = EXCLUDED.roles`
	if _, err := s.pool.Exec(ctx, query, username, string(hash), roles); err != nil {
		return fmt.Errorf("userstore: upsert user: %w", err)
	}
	return nil
}

// Authenticate, kimlik bilgilerini doğrular ve kullanıcının rollerini döner.
func (s *Postgres) Authenticate(ctx context.Context, username, password string) ([]string, error) {
	const query = `SELECT password_hash, roles FROM users WHERE username = $1`
	var (
		hash  string
		roles []string
	)
	err := s.pool.QueryRow(ctx, query, username).Scan(&hash, &roles)
	if errors.Is(err, pgx.ErrNoRows) {
		// Kullanıcı yoksa da bcrypt maliyetini ödeyerek zamanlama sızıntısını azalt.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("userstore: query user: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return roles, nil
}
