package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Run, komut satırı argümanlarını ilgili komuta yönlendirir.
func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.usage()
		return nil
	}
	command := args[0]
	rest := args[1:]

	switch command {
	case "login":
		return a.cmdLogin(ctx, rest)
	case "logout":
		return a.cmdLogout()
	case "send":
		return a.cmdSend(ctx, rest)
	case "tasks":
		return a.cmdTasks(ctx, rest)
	case "task":
		return a.cmdTask(ctx, rest)
	case "artifact":
		return a.cmdArtifact(ctx, rest)
	case "cancel":
		return a.cmdCancel(ctx, rest)
	case "card":
		return a.cmdCard(ctx)
	case "help", "-h", "--help":
		a.usage()
		return nil
	default:
		a.usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func (a *App) usage() {
	fmt.Fprint(a.Out, `a2a-research-crew CLI

Usage: cli [global flags] <command> [args]

Commands:
  login      Authenticate and store JWT tokens (-username, -password)
  logout     Remove stored tokens
  send       Send a research request and stream the response
  tasks      List tasks (-context, -limit)
  task       Show a task by id
  artifact   Print the artifacts of a task by id
  cancel     Cancel a task by id
  card       Print the orchestrator agent card

Global flags:
  -server       Orchestrator card base URL (default http://127.0.0.1:9200)
  -token-file   Path to the token file
`)
}

func (a *App) cmdLogin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	username := fs.String("username", "", "username")
	password := fs.String("password", os.Getenv("A2A_PASSWORD"), "password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" || *password == "" {
		return fmt.Errorf("login requires -username and -password (or A2A_PASSWORD)")
	}

	base, err := a.restBase(ctx)
	if err != nil {
		return err
	}
	tokens, err := a.loginRequest(ctx, base, *username, *password)
	if err != nil {
		return err
	}
	if err := a.Tokens.Save(tokens); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "logged in as %s\n", *username)
	return nil
}

func (a *App) cmdLogout() error {
	if err := a.Tokens.Clear(); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "logged out")
	return nil
}

func (a *App) cmdSend(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("send requires a message")
	}
	text := strings.Join(fs.Args(), " ")

	client, err := a.authenticatedClient(ctx)
	if err != nil {
		return err
	}

	request := &a2a.SendMessageRequest{
		Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text)),
	}

	var collected strings.Builder
	for event, err := range client.SendStreamingMessage(ctx, request) {
		if err != nil {
			return fmt.Errorf("stream failed: %w", err)
		}
		renderEvent(a.Out, event, &collected)
	}

	if collected.Len() > 0 {
		fmt.Fprintln(a.Out, "----")
		fmt.Fprintln(a.Out, collected.String())
	}
	return nil
}

func (a *App) cmdTasks(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("tasks", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	contextID := fs.String("context", "", "filter by context id")
	limit := fs.Int("limit", 20, "page size")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := a.authenticatedClient(ctx)
	if err != nil {
		return err
	}
	res, err := client.ListTasks(ctx, &a2a.ListTasksRequest{ContextID: *contextID, PageSize: *limit})
	if err != nil {
		return err
	}
	for _, task := range res.Tasks {
		fmt.Fprintf(a.Out, "%s\t%s\t%s\n", task.ID, task.Status.State, task.ContextID)
	}
	fmt.Fprintf(a.Out, "total=%d\n", res.TotalSize)
	return nil
}

func (a *App) cmdTask(ctx context.Context, args []string) error {
	task, err := a.getTask(ctx, args, "task")
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "id=%s\nstate=%s\ncontext=%s\nartifacts=%d\n",
		task.ID, task.Status.State, task.ContextID, len(task.Artifacts))
	if text := artifactText(task); text != "" {
		fmt.Fprintln(a.Out, "----")
		fmt.Fprintln(a.Out, text)
	}
	return nil
}

func (a *App) cmdArtifact(ctx context.Context, args []string) error {
	task, err := a.getTask(ctx, args, "artifact")
	if err != nil {
		return err
	}
	text := artifactText(task)
	if text == "" {
		fmt.Fprintln(a.Out, "no artifacts")
		return nil
	}
	fmt.Fprintln(a.Out, text)
	return nil
}

func (a *App) getTask(ctx context.Context, args []string, name string) (*a2a.Task, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() == 0 {
		return nil, fmt.Errorf("%s requires a task id", name)
	}
	client, err := a.authenticatedClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.GetTask(ctx, &a2a.GetTaskRequest{ID: a2a.TaskID(fs.Arg(0))})
}

func (a *App) cmdCancel(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("cancel requires a task id")
	}

	client, err := a.authenticatedClient(ctx)
	if err != nil {
		return err
	}
	task, err := client.CancelTask(ctx, &a2a.CancelTaskRequest{ID: a2a.TaskID(fs.Arg(0))})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "state=%s\n", task.Status.State)
	return nil
}

func (a *App) cmdCard(ctx context.Context) error {
	card, err := a.resolve(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "name=%s\nversion=%s\ndescription=%s\n", card.Name, card.Version, card.Description)
	for _, iface := range card.SupportedInterfaces {
		fmt.Fprintf(a.Out, "interface=%s %s\n", iface.ProtocolBinding, iface.URL)
	}
	for _, skill := range card.Skills {
		fmt.Fprintf(a.Out, "skill=%s (%s)\n", skill.ID, skill.Name)
	}
	return nil
}
