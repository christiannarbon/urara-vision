// Package agent answers one question: the context card, prompt and tool loop.
package agent

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"urara-vision/backend/internal/chat/apiclient"
)

// noGrain keeps a row aligned when a table has no grain.
const noGrain = "—"

// MaxCachedCards bounds the snapshots one pod keeps cards for.
const MaxCachedCards = 32

// RenderCard is the whole snapshot as a compact inventory for the system prompt.
// No descriptions: reader-written prose in the system message was obeyed (08.7).
func RenderCard(c apiclient.Context) string {
	lines := append(header(c), "")
	lines = append(lines, domains(c)...)
	lines = append(lines, "")
	lines = append(lines, tables(c)...)
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

func header(c apiclient.Context) []string {
	project := c.Snapshot.Project.Project
	name := project.Name
	if name == "" {
		name = c.Snapshot.Name
	}
	if name == "" {
		name = c.Snapshot.ID
	}
	version := ""
	if project.Version != "" {
		version = " (v" + project.Version + ")"
	}
	lines := []string{"PROJECT: " + name + version}

	i18n := c.Snapshot.Project.Internationalization
	var others []string
	for _, lang := range i18n.Supported {
		if lang != i18n.Primary {
			others = append(others, lang)
		}
	}
	// A single language tells the model nothing, so the line is left out.
	if len(others) > 0 {
		lines = append(lines, "LANGUAGES: primary "+i18n.Primary+", also "+strings.Join(others, ", "))
	}
	return append(lines, "COUNTS: "+counts(c))
}

func counts(c apiclient.Context) string {
	s := c.Snapshot.Stats
	parts := []string{
		fmt.Sprintf("%d domains", s.Domains),
		fmt.Sprintf("%d tables", s.Tables),
		fmt.Sprintf("%d columns", s.Columns),
		fmt.Sprintf("%d relationships", s.Relationships),
	}
	total := 0
	for _, n := range c.Diagnostics {
		total += n
	}
	if total != 0 {
		var breakdown []string
		for _, level := range []string{"error", "warning", "info"} {
			if n := c.Diagnostics[level]; n != 0 {
				breakdown = append(breakdown, fmt.Sprintf("%d %s", n, level))
			}
		}
		parts = append(parts, fmt.Sprintf("%d diagnostics (%s)", total, strings.Join(breakdown, ", ")))
	}
	return strings.Join(parts, ", ")
}

func domains(c apiclient.Context) []string {
	lines := []string{"DOMAINS"}
	for _, d := range c.Domains {
		detail := "(no tables documented)"
		switch {
		case d.TableCount == 1:
			detail = "1 table"
		case d.TableCount != 0:
			detail = fmt.Sprintf("%d tables", d.TableCount)
		}
		lines = append(lines, "  "+d.ID+" — "+detail)
	}
	return lines
}

func tables(c apiclient.Context) []string {
	if c.Truncated {
		// Silence would read as a model with no tables.
		return []string{fmt.Sprintf("TABLES: %d tables, too many to list here. "+
			"Use search_model to find them by name or description, then get_tables "+
			"to read them.", c.Snapshot.Stats.Tables)}
	}
	lines := []string{"TABLES  (id | kind | grain | columns)"}
	for _, t := range c.Tables {
		grain := t.Grain
		if grain == "" {
			grain = noGrain
		}
		row := fmt.Sprintf("  %s | %s | %s | %d", t.ID, t.Kind, grain, t.ColumnCount)
		if t.Conformed {
			row += " | conformed"
		}
		lines = append(lines, row)
	}
	return lines
}

// CardBackend is the part of apiclient.Client the cache calls.
type CardBackend interface {
	Context(ctx context.Context, sid string) (apiclient.Context, error)
}

type cardEntry struct {
	id      string
	lock    chan struct{} // held while fetching; a channel so a waiter can give up
	card    string        // empty until a fetch succeeds, and again once expired
	expires time.Time
}

// CardCache holds rendered cards by snapshot ID, least recently used first out.
type CardCache struct {
	ttl        time.Duration
	maxEntries int
	now        func() time.Time

	mu      sync.Mutex // guards entries, order and each entry's card and expiry
	entries map[string]*list.Element
	order   *list.List
}

func NewCardCache(ttl time.Duration, maxEntries int, now func() time.Time) *CardCache {
	if now == nil {
		now = time.Now
	}
	return &CardCache{ttl: ttl, maxEntries: maxEntries, now: now, entries: map[string]*list.Element{}, order: list.New()}
}

var errLatest = errors.New("the card cache requires a concrete snapshot ID, not 'latest'; resolve it first")

// Get returns the card for a snapshot, fetching once per snapshot on a miss.
// One lock per snapshot: turns on different snapshots do not wait for each other.
func (c *CardCache) Get(ctx context.Context, b CardBackend, snapshotID string) (string, error) {
	// Caching under the alias would serve a stale model after a re-ingest.
	if snapshotID == "latest" {
		return "", errLatest
	}
	c.mu.Lock()
	e := c.slot(snapshotID)
	card := c.live(e)
	c.mu.Unlock()
	if card != "" {
		return card, nil
	}

	select {
	case e.lock <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-e.lock }()

	// Checked again: concurrent cold callers pay for one fetch.
	c.mu.Lock()
	card = c.live(e)
	c.mu.Unlock()
	if card != "" {
		return card, nil
	}
	snap, err := b.Context(ctx, snapshotID)
	if err != nil {
		return "", err
	}
	card = RenderCard(snap)
	c.mu.Lock()
	e.card, e.expires = card, c.now().Add(c.ttl)
	c.mu.Unlock()
	return card, nil
}

// Len is the number of entries, cards or not.
func (c *CardCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// slot finds or makes the entry, marking it most recent. Caller holds mu.
func (c *CardCache) slot(id string) *cardEntry {
	if el, ok := c.entries[id]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*cardEntry)
	}
	e := &cardEntry{id: id, lock: make(chan struct{}, 1)}
	c.entries[id] = c.order.PushFront(e)
	for c.order.Len() > c.maxEntries {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*cardEntry).id)
	}
	return e
}

// live is the entry's card, or "" once expired. The entry and its lock stay:
// a waiter may hold the lock. Caller holds mu.
func (c *CardCache) live(e *cardEntry) string {
	if e.card != "" && !c.now().Before(e.expires) {
		e.card = ""
	}
	return e.card
}
