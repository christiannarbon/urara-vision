// Package apiclient is the chat service's typed client of the backend's /api/v1.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"urara-vision/backend/internal/chat/reqctx"
	"urara-vision/backend/internal/model"
)

const api = "/api/v1"

// Client calls one backend over one connection pool. No retries, as in Python.
type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{base: baseURL, token: token, http: &http.Client{Timeout: timeout}}
}

func (c *Client) send(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// An empty token is the backend's unauthenticated mode; "Bearer " alone is refused.
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if id := reqctx.RequestID(ctx); id != "" {
		req.Header.Set("X-Request-Id", id)
	}
	// The backend trusts this only alongside the service token.
	if id := reqctx.UserID(ctx); id != "" {
		req.Header.Set("X-Acting-User", id)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, unreachable(err)
	}
	return resp, nil
}

// do sends a request and decodes a successful body into out, when out is non-nil.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	resp, err := c.send(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return failure(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding %s %s: %w", method, path, err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}

// failure never fails itself: an HTML or empty body keeps the status text.
func failure(resp *http.Response) *Error {
	msg := http.StatusText(resp.StatusCode)
	if msg == "" {
		msg = "HTTP " + strconv.Itoa(resp.StatusCode)
	}
	var body struct {
		Error *string `json:"error"`
	}
	if raw, err := io.ReadAll(resp.Body); err == nil && json.Unmarshal(raw, &body) == nil && body.Error != nil {
		msg = *body.Error
	}
	return &Error{Status: resp.StatusCode, Message: msg}
}

// missing reports the first empty required field, as pydantic would refuse it.
func missing(what string, values ...string) error {
	for _, v := range values {
		if v == "" {
			return fmt.Errorf("backend response: %s is missing", what)
		}
	}
	return nil
}

func snapshotPath(sid, rest string) string {
	return api + "/snapshots/" + url.PathEscape(sid) + rest
}

func conversationPath(cid string) string {
	return api + "/conversations/" + url.PathEscape(cid)
}

// Health reports whether the backend is ready. Never an error.
func (c *Client) Health(ctx context.Context) bool {
	resp, err := c.send(ctx, http.MethodGet, "/readyz", nil, nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

func (c *Client) Features(ctx context.Context) (Features, error) {
	var out struct {
		Chat *ChatFeature `json:"chat"`
	}
	if err := c.get(ctx, api+"/features", nil, &out); err != nil {
		return Features{}, err
	}
	if out.Chat == nil {
		return Features{}, missing("features.chat", "")
	}
	return Features{Chat: *out.Chat}, nil
}

// ResolveSnapshot turns a reference such as "latest" into a concrete ID.
func (c *Client) ResolveSnapshot(ctx context.Context, sid string) (string, error) {
	var snap model.Snapshot
	if err := c.get(ctx, snapshotPath(sid, ""), nil, &snap); err != nil {
		return "", err
	}
	return snap.ID, missing("snapshot.id", snap.ID)
}

func (c *Client) Context(ctx context.Context, sid string) (Context, error) {
	var out Context
	if err := c.get(ctx, snapshotPath(sid, "/context"), nil, &out); err != nil {
		return Context{}, err
	}
	return out, missing("context.snapshot.id", out.Snapshot.ID)
}

func (c *Client) Domains(ctx context.Context, sid string) ([]model.Domain, error) {
	var out struct {
		Domains []model.Domain `json:"domains"`
	}
	if err := c.get(ctx, snapshotPath(sid, "/domains"), nil, &out); err != nil {
		return nil, err
	}
	for _, d := range out.Domains {
		if err := missing("domain.id", d.ID); err != nil {
			return nil, err
		}
	}
	return out.Domains, nil
}

// Tables lists table summaries; an empty domain lists every table.
func (c *Client) Tables(ctx context.Context, sid, domain string) ([]TableSummary, error) {
	q := url.Values{}
	if domain != "" {
		q.Set("domain", domain)
	}
	var out struct {
		Tables []TableSummary `json:"tables"`
	}
	if err := c.get(ctx, snapshotPath(sid, "/tables"), q, &out); err != nil {
		return nil, err
	}
	for _, t := range out.Tables {
		if err := missing("table.id and name", t.ID, t.Name); err != nil {
			return nil, err
		}
	}
	return out.Tables, nil
}

// Table IDs contain a slash, so they travel as a query parameter.
func (c *Client) Table(ctx context.Context, sid, tableID string) (TableDetail, error) {
	var out TableDetail
	if err := c.get(ctx, snapshotPath(sid, "/table"), url.Values{"id": {tableID}}, &out); err != nil {
		return TableDetail{}, err
	}
	return out, missing("table.id and name", out.Table.ID, out.Table.Name)
}

func (c *Client) TablesDetail(ctx context.Context, sid string, ids []string) (TablesDetail, error) {
	var out TablesDetail
	q := url.Values{"ids": {strings.Join(ids, ",")}}
	if err := c.get(ctx, snapshotPath(sid, "/tables/detail"), q, &out); err != nil {
		return TablesDetail{}, err
	}
	for _, d := range out.Tables {
		if err := missing("table.id and name", d.Table.ID, d.Table.Name); err != nil {
			return TablesDetail{}, err
		}
	}
	return out, nil
}

func (c *Client) Search(ctx context.Context, sid, query string, limit int) ([]SearchHit, error) {
	q := url.Values{"q": {query}, "limit": {strconv.Itoa(limit)}}
	var out struct {
		Hits []SearchHit `json:"hits"`
	}
	if err := c.get(ctx, snapshotPath(sid, "/search"), q, &out); err != nil {
		return nil, err
	}
	for _, h := range out.Hits {
		if err := missing("hit.tableId", h.TableID); err != nil {
			return nil, err
		}
	}
	return out.Hits, nil
}

// Neighbourhood is the subgraph within depth hops of a table. The route is spelt the American way.
func (c *Client) Neighbourhood(ctx context.Context, sid, tableID string, depth int, sources bool) (Graph, error) {
	q := url.Values{
		"table":   {tableID},
		"depth":   {strconv.Itoa(depth)},
		"sources": {strconv.FormatBool(sources)},
	}
	var out Graph
	if err := c.get(ctx, snapshotPath(sid, "/neighborhood"), q, &out); err != nil {
		return Graph{}, err
	}
	for _, n := range out.Nodes {
		if err := missing("node.id", n.ID); err != nil {
			return Graph{}, err
		}
	}
	for _, l := range out.Links {
		if err := missing("link.id, source and target", l.ID, l.Source, l.Target); err != nil {
			return Graph{}, err
		}
	}
	return out, nil
}

func (c *Client) JoinPaths(ctx context.Context, sid, from, to string, maxDepth, limit int) ([]JoinPath, error) {
	q := url.Values{
		"from":     {from},
		"to":       {to},
		"maxDepth": {strconv.Itoa(maxDepth)},
		"limit":    {strconv.Itoa(limit)},
	}
	var out struct {
		Paths []JoinPath `json:"paths"`
	}
	if err := c.get(ctx, snapshotPath(sid, "/paths"), q, &out); err != nil {
		return nil, err
	}
	return out.Paths, nil
}

// Lineage is the upstream sources of a table, or the downstream tables of a source.
func (c *Client) Lineage(ctx context.Context, sid, tableID, direction string) ([]LineageEntry, error) {
	q := url.Values{"id": {tableID}, "direction": {direction}}
	var out struct {
		Entries []LineageEntry `json:"entries"`
	}
	if err := c.get(ctx, snapshotPath(sid, "/lineage"), q, &out); err != nil {
		return nil, err
	}
	for _, e := range out.Entries {
		if err := missing("entry.id", e.ID); err != nil {
			return nil, err
		}
	}
	return out.Entries, nil
}

// Diagnostics lists diagnostics; an empty severity lists all of them.
func (c *Client) Diagnostics(ctx context.Context, sid, severity string) ([]model.Diagnostic, error) {
	q := url.Values{}
	if severity != "" {
		q.Set("severity", severity)
	}
	var out struct {
		Diagnostics []model.Diagnostic `json:"diagnostics"`
	}
	if err := c.get(ctx, snapshotPath(sid, "/diagnostics"), q, &out); err != nil {
		return nil, err
	}
	for _, d := range out.Diagnostics {
		if err := missing("diagnostic.severity", d.Severity); err != nil {
			return nil, err
		}
	}
	return out.Diagnostics, nil
}

func (c *Client) Sources(ctx context.Context, sid string) ([]model.SourceTable, error) {
	var out struct {
		Sources []model.SourceTable `json:"sources"`
	}
	if err := c.get(ctx, snapshotPath(sid, "/sources"), nil, &out); err != nil {
		return nil, err
	}
	for _, s := range out.Sources {
		if err := missing("source.id", s.ID); err != nil {
			return nil, err
		}
	}
	return out.Sources, nil
}

func (c *Client) conversation(ctx context.Context, method, path string, body any) (model.Conversation, error) {
	var out model.Conversation
	if err := c.do(ctx, method, path, nil, body, &out); err != nil {
		return model.Conversation{}, err
	}
	return out, missing("conversation.id", out.ID)
}

// CreateConversation passes "latest" through; the backend resolves and stores the concrete ID.
func (c *Client) CreateConversation(ctx context.Context, snapshotID, title string) (model.Conversation, error) {
	body := map[string]string{"snapshotId": snapshotID, "title": title}
	return c.conversation(ctx, http.MethodPost, api+"/conversations", body)
}

// ListConversations omits limit when it is 0, leaving the backend's default.
func (c *Client) ListConversations(ctx context.Context, snapshotID string, limit int) ([]model.Conversation, error) {
	q := url.Values{"snapshot": {snapshotID}}
	if limit != 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out struct {
		Conversations []model.Conversation `json:"conversations"`
	}
	if err := c.get(ctx, api+"/conversations", q, &out); err != nil {
		return nil, err
	}
	for _, conv := range out.Conversations {
		if err := missing("conversation.id", conv.ID); err != nil {
			return nil, err
		}
	}
	return out.Conversations, nil
}

func (c *Client) GetConversation(ctx context.Context, cid string) (model.Conversation, error) {
	return c.conversation(ctx, http.MethodGet, conversationPath(cid), nil)
}

// SetConversationTitle patches only the title.
func (c *Client) SetConversationTitle(ctx context.Context, cid, title string) (model.Conversation, error) {
	return c.conversation(ctx, http.MethodPatch, conversationPath(cid), map[string]string{"title": title})
}

func (c *Client) DeleteConversation(ctx context.Context, cid string) error {
	return c.do(ctx, http.MethodDelete, conversationPath(cid), nil, nil, nil)
}

type appendRequest struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Citations []string       `json:"citations"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// AppendMessage stores one message. Nil citations are sent as [], so "cited nothing"
// never reads as null; nil meta is omitted.
func (c *Client) AppendMessage(ctx context.Context, cid, role, content string, citations []string, meta map[string]any) (model.Message, error) {
	if citations == nil {
		citations = []string{}
	}
	body := appendRequest{Role: role, Content: content, Citations: citations, Meta: meta}
	var out model.Message
	if err := c.do(ctx, http.MethodPost, conversationPath(cid)+"/messages", nil, body, &out); err != nil {
		return model.Message{}, err
	}
	return out, missing("message.role", out.Role)
}
