package chateval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	AnswerTimeout = 180 * time.Second
	retriesOn429  = 5
)

// AdminID logs in as the bootstrap admin: chat needs a user, and deleting a
// snapshot an acting admin.
func AdminID(ctx context.Context, backendURL, username, password string) (string, error) {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 60 * time.Second}
	call := func(method, path string, body any) (*http.Response, error) {
		req, err := newJSONRequest(ctx, method, backendURL+path, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Requested-With", "urara")
		return checked(c.Do(req))
	}
	res, err := call("POST", "/api/v1/auth/login", map[string]string{"username": username, "password": password})
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	_ = res.Body.Close()
	res, err = call("GET", "/api/v1/auth/me", nil)
	if err != nil {
		return "", fmt.Errorf("auth/me: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(res.Body).Decode(&me); err != nil || me.User.ID == "" {
		return "", fmt.Errorf("auth/me: no user id (%v)", err)
	}
	return me.User.ID, nil
}

// Ingester ingests each set once and deletes what this run created.
type Ingester struct {
	BackendURL, Token, Label, ActingUser string
	Dirs                                 []string // searched in order for a set
	Client                               *http.Client
	Out                                  io.Writer // nil is silent

	IDs     map[string]string
	created []string
}

func (in *Ingester) Ingest(ctx context.Context, name string) (string, error) {
	files, err := in.files(name)
	if err != nil {
		return "", err
	}
	body := map[string]any{"name": name, "sourceLabel": in.Label, "files": files}
	res, err := in.do(ctx, "POST", "/api/v1/ingest", body, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()

	var sid string
	// A version ingests once. Reuse it, not rename: the project name is in the prompt.
	if res.StatusCode == http.StatusConflict {
		var c struct{ Project, Version string }
		if err := json.NewDecoder(res.Body).Decode(&c); err != nil {
			return "", fmt.Errorf("ingest %s: 409: %w", name, err)
		}
		existing, err := checked(in.do(ctx, "GET", "/api/v1/projects/"+url.PathEscape(c.Project)+"/versions/"+url.PathEscape(c.Version), nil, nil))
		if err != nil {
			return "", fmt.Errorf("ingest %s: existing version: %w", name, err)
		}
		defer func() { _ = existing.Body.Close() }()
		var v struct{ ID string }
		if err := json.NewDecoder(existing.Body).Decode(&v); err != nil {
			return "", fmt.Errorf("ingest %s: existing version: %w", name, err)
		}
		sid = v.ID
		if in.Out != nil {
			fmt.Fprintf(in.Out, "  %s %s already ingested; scoring that snapshot\n", c.Project, c.Version)
		}
	} else {
		if _, err := checked(res, nil); err != nil {
			return "", fmt.Errorf("ingest %s: %w", name, err)
		}
		var out struct {
			Snapshot struct{ ID string } `json:"snapshot"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			return "", fmt.Errorf("ingest %s: %w", name, err)
		}
		sid = out.Snapshot.ID
		in.created = append(in.created, sid)
	}
	if in.IDs == nil {
		in.IDs = map[string]string{}
	}
	in.IDs[name] = sid
	return sid, nil
}

func (in *Ingester) files(name string) ([]map[string]string, error) {
	for _, dir := range in.Dirs {
		root := filepath.Join(dir, name)
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			continue
		}
		var files []map[string]string
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			// A fixture's README is for people; ingested, it would add a diagnostic.
			if ext := filepath.Ext(p); (ext != ".md" && ext != ".toml") || rel == "README.md" {
				return nil
			}
			content, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			files = append(files, map[string]string{"path": filepath.ToSlash(rel), "content": string(content)})
			return nil
		})
		return files, err
	}
	return nil, fmt.Errorf("set %q not found in %s", name, strings.Join(in.Dirs, ", "))
}

// Cleanup deletes the snapshots this run created, and any carrying its label,
// which catches an ingest that landed after an interrupt. It returns failures.
func (in *Ingester) Cleanup(ctx context.Context) []string {
	ids := map[string]bool{}
	for _, id := range in.created {
		ids[id] = true
	}
	if res, err := checked(in.do(ctx, "GET", "/api/v1/snapshots", nil, nil)); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not list snapshots for cleanup: %v\n", err)
	} else {
		var listed struct {
			Snapshots []struct{ ID, SourceLabel string }
		}
		if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not read snapshots for cleanup: %v\n", err)
		}
		_ = res.Body.Close()
		for _, s := range listed.Snapshots {
			if s.SourceLabel == in.Label {
				ids[s.ID] = true
			}
		}
	}

	var failed []string
	for id := range ids {
		res, err := in.do(ctx, "DELETE", "/api/v1/snapshots/"+id, nil, map[string]string{"X-Acting-User": in.ActingUser})
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", id, err))
			continue
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusNotFound {
			failed = append(failed, fmt.Sprintf("%s (%d)", id, res.StatusCode))
		}
	}
	return failed
}

func (in *Ingester) do(ctx context.Context, method, path string, body any, headers map[string]string) (*http.Response, error) {
	req, err := newJSONRequest(ctx, method, in.BackendURL+path, body)
	if err != nil {
		return nil, err
	}
	if in.Token != "" {
		req.Header.Set("Authorization", "Bearer "+in.Token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return in.Client.Do(req)
}

// Chat asks questions as one user.
type Chat struct {
	URL, UserID string
	Client      *http.Client
	// ExpectModel, when set, stops the run if the service answers with another model.
	ExpectModel string
	Out         io.Writer
}

// ErrWrongModel stops a run: its answers would be scored under the wrong name.
var ErrWrongModel = errors.New("wrong model")

// Ask returns an error only when the run must stop; a failed question is
// recorded on the result.
func (c *Chat) Ask(ctx context.Context, q Question, snapshotID string, run int) (Result, error) {
	r := Result{ID: q.ID, Set: q.Set, Category: q.Category, Language: q.Language, Question: q.Question, Run: run,
		Citations: []string{}, ToolCalls: []ToolCall{}, Usage: map[string]int{}}
	started := time.Now()
	body, status, err := c.post(ctx, map[string]string{"snapshotId": snapshotID, "question": q.Question, "language": q.Language})
	r.WallMS = int(math.RoundToEven(float64(time.Since(started).Microseconds()) / 1000))
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	if err != nil {
		r.Error = err.Error()
		return r, nil
	}
	if status != http.StatusOK {
		r.Error = fmt.Sprintf("%d %s", status, firstRunes(string(body), 300))
		return r, nil
	}

	var a AnswerResponse
	if err := json.Unmarshal(body, &a); err != nil {
		r.Error = "decoding the answer: " + err.Error()
		return r, nil
	}
	if c.ExpectModel != "" && a.Model != c.ExpectModel {
		return r, fmt.Errorf("%w: the chat service answered with %q, not %q; restart it with LLM_MODEL=%s",
			ErrWrongModel, a.Model, c.ExpectModel, c.ExpectModel)
	}
	r.Answer, r.Model, r.LatencyMS = a.Text, a.Model, a.LatencyMS
	if a.Citations != nil {
		r.Citations = a.Citations
	}
	if a.ToolCalls != nil {
		r.ToolCalls = a.ToolCalls
	}
	if a.Usage != nil {
		r.Usage = a.Usage
	}
	s := Score(q, a)
	r.Scores = &s
	verdict := "FAIL"
	if s.Passed() {
		verdict = "pass"
	}
	if c.Out != nil {
		fmt.Fprintf(c.Out, "  %s  %s (run %d)\n", verdict, q.ID, run)
	}
	return r, nil
}

// post retries a 429 after Retry-After, or 2^attempt seconds without one.
func (c *Chat) post(ctx context.Context, body any) ([]byte, int, error) {
	for attempt := 0; ; attempt++ {
		req, err := newJSONRequest(ctx, "POST", c.URL+"/api/chat/answer", body)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("X-User-Id", c.UserID)
		res, err := c.Client.Do(req)
		if err != nil {
			return nil, 0, err
		}
		raw, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil {
			return nil, 0, err
		}
		if res.StatusCode != http.StatusTooManyRequests || attempt == retriesOn429 {
			return raw, res.StatusCode, nil
		}
		wait := time.Duration(1<<attempt) * time.Second
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s >= 0 {
			wait = time.Duration(s) * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(wait):
		}
	}
}

func (c *Chat) Ready(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.URL+"/readyz", nil)
	if err != nil {
		return err
	}
	res, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("chat service is not ready: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("chat service is not ready: %d %s", res.StatusCode, firstRunes(string(raw), 200))
	}
	return nil
}

type RunOptions struct {
	Chat        *Chat
	Questions   []Question
	SnapshotIDs map[string]string // by set
	Repeat      int
	Concurrency int
}

// RunAll asks every question Repeat times; results are in run, then question, order.
func RunAll(ctx context.Context, o RunOptions) ([]Result, error) {
	if err := o.Chat.Ready(ctx); err != nil {
		return nil, err
	}
	results := make([]Result, o.Repeat*len(o.Questions))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(o.Concurrency)
	for run := 1; run <= o.Repeat; run++ {
		for i, q := range o.Questions {
			at := (run-1)*len(o.Questions) + i
			g.Go(func() error {
				r, err := o.Chat.Ask(gctx, q, o.SnapshotIDs[q.Set], run)
				results[at] = r
				return err
			})
		}
	}
	if err := g.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return results, nil
}

// Args are the run's flags as the results file records them; the token is left out.
type Args struct {
	Set         []string `json:"set"`
	Category    []string `json:"category"`
	ID          []string `json:"id"`
	Model       *string  `json:"model"`
	Repeat      int      `json:"repeat"`
	Concurrency int      `json:"concurrency"`
	Out         *string  `json:"out"`
	ChatURL     string   `json:"chat_url"`
	BackendURL  string   `json:"backend_url"`
}

type Meta struct {
	StartedAt  time.Time
	Wall       time.Duration
	Args       Args
	Thresholds Thresholds
}

// ResultsFile is <dir>/YYYYMMDDTHHMMSSZ.json for the run's UTC start.
func ResultsFile(dir string, started time.Time) string {
	return filepath.Join(dir, started.UTC().Format("20060102T150405Z")+".json")
}

// WriteResults writes the file run_eval.py wrote, with the same keys.
func WriteResults(path string, s Summary, results []Result, m Meta) error {
	payload := struct {
		StartedAt   string     `json:"startedAt"`
		WallSeconds float64    `json:"wallSeconds"`
		Args        Args       `json:"args"`
		Thresholds  Thresholds `json:"thresholds"`
		Summary     Summary    `json:"summary"`
		Results     []Result   `json:"results"`
	}{
		StartedAt:   m.StartedAt.UTC().Format("2006-01-02T15:04:05.000000-07:00"),
		WallSeconds: math.RoundToEven(m.Wall.Seconds()*10) / 10,
		Args:        m.Args, Thresholds: m.Thresholds, Summary: s, Results: results,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func newJSONRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err == nil && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, err
}

// checked turns a non-2xx response into an error, closing its body.
func checked(res *http.Response, err error) (*http.Response, error) {
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		raw, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		return nil, fmt.Errorf("%s %s: %d %s", res.Request.Method, res.Request.URL.Path, res.StatusCode, firstRunes(string(raw), 300))
	}
	return res, nil
}

func firstRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
