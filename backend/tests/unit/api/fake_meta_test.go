// A fake record-of-truth store.
//
// Every method is a hook the test fills in, so a spec only sets up the calls it
// cares about and an unexpected call returns a zero value rather than panicking.
package api_test

import (
	"context"
	"errors"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

// errBoom stands in for any store failure the handlers do not special-case.
var errBoom = errors.New("boom")

// fakeMeta records what it was asked for and returns whatever the test set.
// Each field is a hook, so a test only fills in the calls it cares about and
// any unexpected call returns the zero value rather than panicking.
type fakeMeta struct {
	snapshots []model.Snapshot
	snapshot  *model.Snapshot
	latest    string
	domains   []model.Domain
	tables    []postgres.TableSummary
	table     *model.Table
	// tablesByID lets one fake answer for several IDs, which the batch endpoint
	// needs. When it is set it replaces table entirely: an ID not in the map is
	// ErrNotFound.
	tablesByID  map[string]*model.Table
	incoming    []postgres.Referrer
	hits        []postgres.SearchHit
	diagnostics []model.Diagnostic
	sources     []model.SourceTable

	conversation  *model.Conversation
	conversations []model.Conversation
	messages      []model.Message
	// appended is what AppendMessage hands back. When it is nil the fake echoes
	// the message it was given with the next ordinal, which is enough for a
	// handler test that only cares that the stored row is what is returned.
	appended *model.Message

	// Errors, keyed by the method they should come out of.
	errGetSnapshot  error
	errLatest       error
	errSave         error
	errDelete       error
	errList         error
	errPing         error
	errConversation error
	errAppend       error

	// Recorded calls.
	saved         *model.Model
	deleted       []string
	tablesDomain  string
	searchQuery   string
	searchLimit   int
	diagsSeverity string
	getTableID    string
	getTableIDs   []string
	createdFor    string
	createdTitle  string
	listedFor     string
	convID        string
	appendedTo    string
	appendedMsg   model.Message
}

func (f *fakeMeta) SaveSnapshot(_ context.Context, m *model.Model) error {
	f.saved = m
	return f.errSave
}

func (f *fakeMeta) ListSnapshots(context.Context) ([]model.Snapshot, error) {
	return f.snapshots, f.errList
}

func (f *fakeMeta) GetSnapshot(_ context.Context, id string) (*model.Snapshot, error) {
	if f.errGetSnapshot != nil {
		return nil, f.errGetSnapshot
	}
	if f.snapshot != nil {
		return f.snapshot, nil
	}
	return &model.Snapshot{ID: id, Name: "snap"}, nil
}

func (f *fakeMeta) DeleteSnapshot(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return f.errDelete
}

func (f *fakeMeta) LatestSnapshotID(context.Context) (string, error) {
	return f.latest, f.errLatest
}

func (f *fakeMeta) ListDomains(context.Context, string) ([]model.Domain, error) {
	return f.domains, f.errList
}

func (f *fakeMeta) ListTables(_ context.Context, _, domainID string) ([]postgres.TableSummary, error) {
	f.tablesDomain = domainID
	return f.tables, f.errList
}

func (f *fakeMeta) GetTable(_ context.Context, _, tableID string) (*model.Table, error) {
	f.getTableID = tableID
	f.getTableIDs = append(f.getTableIDs, tableID)
	if f.tablesByID != nil {
		t, ok := f.tablesByID[tableID]
		if !ok {
			return nil, postgres.ErrNotFound
		}
		return t, nil
	}
	if f.table == nil {
		return nil, postgres.ErrNotFound
	}
	return f.table, nil
}

func (f *fakeMeta) IncomingRelationships(context.Context, string, string) ([]postgres.Referrer, error) {
	return f.incoming, nil
}

func (f *fakeMeta) Search(_ context.Context, _, query string, limit int) ([]postgres.SearchHit, error) {
	f.searchQuery, f.searchLimit = query, limit
	return f.hits, f.errList
}

func (f *fakeMeta) ListDiagnostics(_ context.Context, _, severity string) ([]model.Diagnostic, error) {
	f.diagsSeverity = severity
	return f.diagnostics, f.errList
}

func (f *fakeMeta) ListSourceTables(context.Context, string) ([]model.SourceTable, error) {
	return f.sources, f.errList
}

func (f *fakeMeta) Ping(context.Context) error { return f.errPing }

func (f *fakeMeta) CreateConversation(_ context.Context, snapshotID, title string) (*model.Conversation, error) {
	f.createdFor, f.createdTitle = snapshotID, title
	if f.errConversation != nil {
		return nil, f.errConversation
	}
	if f.conversation != nil {
		return f.conversation, nil
	}
	return &model.Conversation{ID: "conv-1", SnapshotID: snapshotID, Title: title}, nil
}

func (f *fakeMeta) ListConversations(_ context.Context, snapshotID string) ([]model.Conversation, error) {
	f.listedFor = snapshotID
	return f.conversations, f.errConversation
}

func (f *fakeMeta) GetConversation(_ context.Context, id string) (*model.Conversation, error) {
	f.convID = id
	if f.errConversation != nil {
		return nil, f.errConversation
	}
	if f.conversation == nil {
		return nil, postgres.ErrNotFound
	}
	return f.conversation, nil
}

func (f *fakeMeta) DeleteConversation(_ context.Context, id string) error {
	f.convID = id
	return f.errConversation
}

func (f *fakeMeta) AppendMessage(_ context.Context, conversationID string, m model.Message) (*model.Message, error) {
	f.appendedTo, f.appendedMsg = conversationID, m
	if f.errAppend != nil {
		return nil, f.errAppend
	}
	if f.appended != nil {
		return f.appended, nil
	}
	stored := m
	stored.Ordinal = len(f.messages)
	return &stored, nil
}

func (f *fakeMeta) ListMessages(_ context.Context, conversationID string) ([]model.Message, error) {
	f.convID = conversationID
	return f.messages, f.errConversation
}
