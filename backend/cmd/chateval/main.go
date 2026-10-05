// Command chateval scores the chat agent over the golden question set. It
// drives the stateless POST /api/chat/answer and costs money.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"urara-vision/backend/internal/chateval"
)

// Relative to the module root, where `make eval` runs it.
const evalDir = "tests/eval"

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func main() {
	os.Exit(run())
}

func run() int {
	var sets, categories, ids list
	flag.Var(&sets, "set", "demo set; repeatable")
	flag.Var(&categories, "category", "category; repeatable")
	flag.Var(&ids, "id", "question id; repeatable")
	model := flag.String("model", "", "fail if the chat service answers with a different model")
	repeat := flag.Int("repeat", 1, "times to ask each question")
	// The service caps turns at MAX_CONCURRENT_TURNS; more only collects 429s.
	concurrency := flag.Int("concurrency", 2, "questions in flight")
	out := flag.String("out", "", "results file (default: results/<timestamp>.json)")
	chatURL := flag.String("chat-url", envOr("EVAL_CHAT_URL", "http://localhost:8090"), "chat service")
	backendURL := flag.String("backend-url", envOr("EVAL_BACKEND_URL", "http://localhost:8080"), "backend")
	token := flag.String("token", envOr("EVAL_API_TOKEN", "relviz-dev-token-not-for-production"), "backend API token")
	flag.Parse()

	fail := func(format string, a ...any) int {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
		return 2
	}
	if *repeat < 1 || *concurrency < 1 {
		return fail("--repeat and --concurrency must be at least 1")
	}
	qs, err := chateval.Load(filepath.Join(evalDir, "questions.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return fail("%v (run from backend/, as make eval does)", err)
	}
	if err != nil {
		return fail("%v", err)
	}
	selected := chateval.Select(qs, sets, categories, ids)
	if len(selected) == 0 {
		return fail("no questions match the selection")
	}
	thresholds, err := chateval.LoadThresholds(filepath.Join(evalDir, "thresholds.yaml"))
	if err != nil {
		return fail("%v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startedAt := time.Now().UTC()
	started := time.Now()
	userID, err := chateval.AdminID(ctx, *backendURL,
		envOr("EVAL_ADMIN_USERNAME", "admin"), envOr("EVAL_ADMIN_PASSWORD", "relviz-dev-admin-password"))
	if err != nil {
		return fail("%v", err)
	}
	ingester := &chateval.Ingester{
		BackendURL: *backendURL, Token: *token, ActingUser: userID,
		Label:  fmt.Sprintf("eval-%s-%d", startedAt.Format("20060102T150405Z"), os.Getpid()),
		Dirs:   []string{envOr("EVAL_DEMO_DIR", filepath.Join("..", "docs", "demo")), filepath.Join(evalDir, "fixtures")},
		Client: &http.Client{Timeout: 60 * time.Second},
		Out:    os.Stdout,
	}

	results, runErr := ask(ctx, ingester, selected, chateval.RunOptions{
		Chat: &chateval.Chat{
			URL: *chatURL, UserID: userID, ExpectModel: *model, Out: os.Stdout,
			Client: &http.Client{Timeout: chateval.AnswerTimeout},
		},
		Questions: selected, Repeat: *repeat, Concurrency: *concurrency,
	})
	// A new context: the run's may already be cancelled.
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if failed := ingester.Cleanup(cleanupCtx); len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "warning: snapshots not deleted: %s\n", strings.Join(failed, ", "))
	}
	// From the run, not ctx: a signal during cleanup must not drop a finished run.
	if errors.Is(runErr, context.Canceled) {
		fmt.Fprintln(os.Stderr, "\ninterrupted")
		return 130
	}
	if runErr != nil {
		return fail("%v", runErr)
	}

	wall := time.Since(started)
	summary := chateval.Summarise(results)
	chateval.Report(os.Stdout, summary, *repeat, wall)

	path := *out
	if path == "" {
		path = chateval.ResultsFile(filepath.Join(evalDir, "results"), startedAt)
	}
	args := chateval.Args{
		Set: sets, Category: categories, ID: ids, Repeat: *repeat, Concurrency: *concurrency,
		ChatURL: *chatURL, BackendURL: *backendURL, Model: orNil(*model), Out: orNil(*out),
	}
	meta := chateval.Meta{StartedAt: startedAt, Wall: wall, Args: args, Thresholds: thresholds}
	if err := chateval.WriteResults(path, summary, results, meta); err != nil {
		return fail("writing results: %v", err)
	}
	fmt.Println("results: " + path)

	missed := chateval.CheckThresholds(summary, thresholds)
	for _, m := range missed {
		fmt.Fprintln(os.Stderr, "THRESHOLD MISSED: "+m)
	}
	if len(missed) > 0 {
		return 1
	}
	return 0
}

// ask ingests every selected set, then runs the questions.
func ask(ctx context.Context, in *chateval.Ingester, selected []chateval.Question, o chateval.RunOptions) ([]chateval.Result, error) {
	var names []string
	for _, q := range selected {
		if !slices.Contains(names, q.Set) {
			names = append(names, q.Set)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Println("ingesting " + name)
		if _, err := in.Ingest(ctx, name); err != nil {
			return nil, err
		}
	}
	fmt.Printf("asking %d question(s) x %d\n", len(selected), o.Repeat)
	o.SnapshotIDs = in.IDs
	return chateval.RunAll(ctx, o)
}

func orNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
