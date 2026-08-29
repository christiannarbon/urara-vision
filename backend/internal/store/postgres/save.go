// Writing a parsed model. One ingest is one transaction, so a snapshot is
// either wholly present or wholly absent.
package postgres

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
)

// SaveSnapshot writes an entire parsed model in one transaction, replacing any
// snapshot that already carries the same ID.
func (s *Store) SaveSnapshot(ctx context.Context, m *model.Model) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sid := m.Snapshot.ID
	if _, err := tx.Exec(ctx, `DELETE FROM snapshots WHERE id = $1`, sid); err != nil {
		return fmt.Errorf("clear snapshot: %w", err)
	}

	stats, err := json.Marshal(m.Snapshot.Stats)
	if err != nil {
		return err
	}
	created := m.Snapshot.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO snapshots (id, name, source_label, created_at, stats) VALUES ($1,$2,$3,$4,$5)`,
		sid, m.Snapshot.Name, m.Snapshot.SourceLabel, created, stats); err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	// Domains.
	domainRows := make([][]any, 0, len(m.Domains))
	for _, d := range m.Domains {
		lineage, err := json.Marshal(orEmptyLineage(d.Lineage))
		if err != nil {
			return err
		}
		domainRows = append(domainRows, []any{
			sid, d.ID, d.Name, d.Title, d.Description, d.Mermaid, lineage, d.DocPath, d.TableCount,
		})
	}
	if err := copyRows(ctx, tx, "domains",
		[]string{"snapshot_id", "id", "name", "title", "description", "mermaid", "lineage", "doc_path", "table_count"},
		domainRows); err != nil {
		return err
	}

	// Tables, plus their columns, lineage and relationships.
	tableRows := make([][]any, 0, len(m.Tables))
	var colRows, linRows, relRows [][]any
	for _, t := range m.Tables {
		notes, err := json.Marshal(orEmptyStrings(t.Notes))
		if err != nil {
			return err
		}
		conformedIn, err := json.Marshal(orEmptyStrings(t.ConformedIn))
		if err != nil {
			return err
		}
		tableRows = append(tableRows, []any{
			sid, t.ID, t.Name, t.DomainID, string(t.Kind), t.KindRaw, t.Grain,
			t.UpdateFrequency, t.Layer, t.DomainLabel, t.Description, notes,
			t.RelationshipNote, t.DocPath, t.Conformed, conformedIn,
		})
		for i, c := range t.Columns {
			colRows = append(colRows, []any{sid, t.ID, i, c.Name, c.Type, c.Description, c.IsPK, c.IsFK})
		}
		for i, l := range t.ColumnLineage {
			linRows = append(linRows, []any{sid, t.ID, i, l.Column, l.SourceTable, l.SourceColumn, l.Notes, l.Derived})
		}
		for _, r := range t.Relationships {
			cands, err := json.Marshal(orEmptyStrings(r.Candidates))
			if err != nil {
				return err
			}
			relRows = append(relRows, []any{
				sid, r.ID, r.FromTableID, r.ToTableID, r.TargetRef, r.FromColumn,
				r.ToColumn, r.JoinKeyRaw, r.Cardinality, string(r.Resolution), cands,
			})
		}
	}
	if err := copyRows(ctx, tx, "tables",
		[]string{"snapshot_id", "id", "name", "domain_id", "kind", "kind_raw", "grain",
			"update_frequency", "layer", "domain_label", "description", "notes",
			"relationship_note", "doc_path", "conformed", "conformed_in"},
		tableRows); err != nil {
		return err
	}
	if err := copyRows(ctx, tx, "columns",
		[]string{"snapshot_id", "table_id", "ordinal", "name", "type", "description", "is_pk", "is_fk"},
		colRows); err != nil {
		return err
	}
	if err := copyRows(ctx, tx, "column_lineage",
		[]string{"snapshot_id", "table_id", "ordinal", "column_name", "source_table", "source_column", "notes", "derived"},
		linRows); err != nil {
		return err
	}
	if err := copyRows(ctx, tx, "relationships",
		[]string{"snapshot_id", "id", "from_table_id", "to_table_id", "target_ref", "from_column",
			"to_column", "join_key_raw", "cardinality", "resolution", "candidates"},
		relRows); err != nil {
		return err
	}

	srcRows := make([][]any, 0, len(m.SourceTables))
	for _, st := range m.SourceTables {
		srcRows = append(srcRows, []any{sid, st.ID, st.Dataset, st.Name, st.Refs})
	}
	if err := copyRows(ctx, tx, "source_tables",
		[]string{"snapshot_id", "id", "dataset", "name", "refs"}, srcRows); err != nil {
		return err
	}

	diagRows := make([][]any, 0, len(m.Diagnostics))
	for i, d := range m.Diagnostics {
		diagRows = append(diagRows, []any{sid, i, d.Severity, d.Code, d.Message, d.DomainID, d.TableID, d.DocPath})
	}
	if err := copyRows(ctx, tx, "diagnostics",
		[]string{"snapshot_id", "ordinal", "severity", "code", "message", "domain_id", "table_id", "doc_path"},
		diagRows); err != nil {
		return err
	}

	// Build the full-text index from the table's own prose plus its column
	// names and descriptions, so a search for a column finds its table.
	if _, err := tx.Exec(ctx, `
		UPDATE tables t SET search =
			setweight(to_tsvector('english', coalesce(t.name, '')), 'A') ||
			setweight(to_tsvector('english', coalesce(t.domain_id, '')), 'B') ||
			setweight(to_tsvector('english', coalesce(t.description, '')), 'C') ||
			setweight(to_tsvector('english', coalesce(t.grain, '')), 'C') ||
			setweight(to_tsvector('english', coalesce((
				SELECT string_agg(c.name || ' ' || c.description, ' ')
				FROM columns c
				WHERE c.snapshot_id = t.snapshot_id AND c.table_id = t.id
			), '')), 'D')
		WHERE t.snapshot_id = $1`, sid); err != nil {
		return fmt.Errorf("build search index: %w", err)
	}

	return tx.Commit(ctx)
}

// copyRows bulk-loads rows, tolerating an empty set.
func copyRows(ctx context.Context, tx pgx.Tx, table string, cols []string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{table}, cols, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("copy into %s: %w", table, err)
	}
	return nil
}

func orEmptyStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func orEmptyLineage(v []model.DomainLineage) []model.DomainLineage {
	if v == nil {
		return []model.DomainLineage{}
	}
	return v
}
