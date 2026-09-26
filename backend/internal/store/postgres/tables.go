// Domain and table reads, from the lightweight summaries the lists and graph
// need up to the full detail one pane shows.
package postgres

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
)

// ListDomains returns the domains of a snapshot.
func (s *Store) ListDomains(ctx context.Context, sid string) ([]model.Domain, error) {
	return listDomains(ctx, s.pool, sid)
}

// querier is satisfied by both the pool and a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func listDomains(ctx context.Context, q querier, sid string) ([]model.Domain, error) {
	rows, err := q.Query(ctx,
		`SELECT id, name, title, description, mermaid, lineage, doc_path, table_count
		 FROM domains WHERE snapshot_id = $1 ORDER BY id`, sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Domain{}
	for rows.Next() {
		var d model.Domain
		var lineage []byte
		if err := rows.Scan(&d.ID, &d.Name, &d.Title, &d.Description, &d.Mermaid,
			&lineage, &d.DocPath, &d.TableCount); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(lineage, &d.Lineage); err != nil {
			return nil, err
		}
		d.SnapshotID = sid
		out = append(out, d)
	}
	return out, rows.Err()
}

// TableSummary is the lightweight table shape used by list and graph responses.
type TableSummary struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	DomainID    string          `json:"domainId"`
	Kind        model.TableKind `json:"kind"`
	Grain       string          `json:"grain"`
	Conformed   bool            `json:"conformed"`
	ColumnCount int             `json:"columnCount"`
	Description string          `json:"description"`
}

// ListTables returns table summaries, optionally filtered to one domain.
func (s *Store) ListTables(ctx context.Context, sid, domainID string) ([]TableSummary, error) {
	q := `SELECT t.id, t.name, t.domain_id, t.kind, t.grain, t.conformed, t.description,
	             (SELECT count(*) FROM columns c WHERE c.snapshot_id = t.snapshot_id AND c.table_id = t.id)
	      FROM tables t WHERE t.snapshot_id = $1`
	args := []any{sid}
	if domainID != "" {
		q += ` AND t.domain_id = $2`
		args = append(args, domainID)
	}
	q += ` ORDER BY t.domain_id, t.name`

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TableSummary{}
	for rows.Next() {
		var t TableSummary
		if err := rows.Scan(&t.ID, &t.Name, &t.DomainID, &t.Kind, &t.Grain,
			&t.Conformed, &t.Description, &t.ColumnCount); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTable returns one table with its columns, lineage and relationships.
func (s *Store) GetTable(ctx context.Context, sid, tableID string) (*model.Table, error) {
	var ts tableScan
	err := s.pool.QueryRow(ctx,
		`SELECT `+tableCols+` FROM tables WHERE snapshot_id = $1 AND id = $2`, sid, tableID).
		Scan(ts.dest()...)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t, err := ts.table(sid)
	if err != nil {
		return nil, err
	}

	colRows, err := s.pool.Query(ctx,
		`SELECT `+columnCols+` FROM columns WHERE snapshot_id = $1 AND table_id = $2 ORDER BY ordinal`, sid, tableID)
	if err != nil {
		return nil, err
	}
	defer colRows.Close()
	t.Columns = []model.Column{}
	for colRows.Next() {
		var c model.Column
		if err := colRows.Scan(columnDest(&c)...); err != nil {
			return nil, err
		}
		t.Columns = append(t.Columns, c)
	}
	if err := colRows.Err(); err != nil {
		return nil, err
	}

	linRows, err := s.pool.Query(ctx,
		`SELECT `+lineageCols+` FROM column_lineage WHERE snapshot_id = $1 AND table_id = $2 ORDER BY ordinal`, sid, tableID)
	if err != nil {
		return nil, err
	}
	defer linRows.Close()
	t.ColumnLineage = []model.ColumnLineage{}
	for linRows.Next() {
		var l model.ColumnLineage
		if err := linRows.Scan(lineageDest(&l)...); err != nil {
			return nil, err
		}
		t.ColumnLineage = append(t.ColumnLineage, l)
	}
	if err := linRows.Err(); err != nil {
		return nil, err
	}

	relRows, err := s.pool.Query(ctx,
		`SELECT `+relationshipCols+` FROM relationships WHERE snapshot_id = $1 AND from_table_id = $2 ORDER BY id`, sid, tableID)
	if err != nil {
		return nil, err
	}
	defer relRows.Close()
	t.Relationships = []model.Relationship{}
	for relRows.Next() {
		var rs relationshipScan
		if err := relRows.Scan(rs.dest()...); err != nil {
			return nil, err
		}
		r, err := rs.relationship()
		if err != nil {
			return nil, err
		}
		t.Relationships = append(t.Relationships, r)
	}
	return &t, relRows.Err()
}

// Shared by GetTable and LoadModel so the two cannot decode rows differently.
const (
	tableCols = `id, name, domain_id, kind, kind_raw, grain, update_frequency, layer,
		domain_label, description, notes, relationship_note, doc_path, conformed, conformed_in`
	columnCols       = `name, type, description, ordinal, is_pk, is_fk`
	lineageCols      = `column_name, source_table, source_column, notes, derived`
	relationshipCols = `id, from_table_id, to_table_id, target_ref, from_column, to_column,
		join_key_raw, cardinality, resolution, candidates`
)

type tableScan struct {
	t                  model.Table
	notes, conformedIn []byte
}

func (ts *tableScan) dest() []any {
	t := &ts.t
	return []any{&t.ID, &t.Name, &t.DomainID, &t.Kind, &t.KindRaw, &t.Grain, &t.UpdateFrequency,
		&t.Layer, &t.DomainLabel, &t.Description, &ts.notes, &t.RelationshipNote,
		&t.DocPath, &t.Conformed, &ts.conformedIn}
}

func (ts *tableScan) table(sid string) (model.Table, error) {
	t := ts.t
	t.SnapshotID = sid
	if err := json.Unmarshal(ts.notes, &t.Notes); err != nil {
		return t, err
	}
	if err := json.Unmarshal(ts.conformedIn, &t.ConformedIn); err != nil {
		return t, err
	}
	return t, nil
}

func columnDest(c *model.Column) []any {
	return []any{&c.Name, &c.Type, &c.Description, &c.Ordinal, &c.IsPK, &c.IsFK}
}

func lineageDest(l *model.ColumnLineage) []any {
	return []any{&l.Column, &l.SourceTable, &l.SourceColumn, &l.Notes, &l.Derived}
}

type relationshipScan struct {
	r     model.Relationship
	cands []byte
}

func (rs *relationshipScan) dest() []any {
	r := &rs.r
	return []any{&r.ID, &r.FromTableID, &r.ToTableID, &r.TargetRef, &r.FromColumn,
		&r.ToColumn, &r.JoinKeyRaw, &r.Cardinality, &r.Resolution, &rs.cands}
}

func (rs *relationshipScan) relationship() (model.Relationship, error) {
	r := rs.r
	err := json.Unmarshal(rs.cands, &r.Candidates)
	return r, err
}
