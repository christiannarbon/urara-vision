// Reads of what an ingest observed: the diagnostics it raised and the upstream
// models it found.
package postgres

import (
	"context"
	_ "embed"

	"urara-vision/backend/internal/model"
)

// ListDiagnostics returns a snapshot's diagnostics, optionally by severity.
func (s *Store) ListDiagnostics(ctx context.Context, sid, severity string) ([]model.Diagnostic, error) {
	q := `SELECT severity, code, message, domain_id, table_id, doc_path
	      FROM diagnostics WHERE snapshot_id = $1`
	args := []any{sid}
	if severity != "" {
		q += ` AND severity = $2`
		args = append(args, severity)
	}
	q += ` ORDER BY ordinal`

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Diagnostic{}
	for rows.Next() {
		var d model.Diagnostic
		if err := rows.Scan(&d.Severity, &d.Code, &d.Message, &d.DomainID, &d.TableID, &d.DocPath); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListSourceTables returns the upstream models referenced by column lineage.
func (s *Store) ListSourceTables(ctx context.Context, sid string) ([]model.SourceTable, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, dataset, name, refs FROM source_tables WHERE snapshot_id = $1 ORDER BY refs DESC, id`, sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SourceTable{}
	for rows.Next() {
		var st model.SourceTable
		if err := rows.Scan(&st.ID, &st.Dataset, &st.Name, &st.Refs); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
