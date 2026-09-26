package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
)

// LoadModel reads a snapshot's domains and every table with its columns,
// lineage and relationships, in a fixed number of queries.
func (s *Store) LoadModel(ctx context.Context, sid string) (*model.Model, error) {
	// One consistent view, so a concurrent delete cannot leave a half-read model.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sn, err := scanSnapshot(tx.QueryRow(ctx, `SELECT `+snapshotColumns+` `+snapshotsFrom+` WHERE s.id = $1`, sid))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	domains, err := listDomains(ctx, tx, sid)
	if err != nil {
		return nil, fmt.Errorf("load domains: %w", err)
	}

	tables := []model.Table{}
	err = eachRow(ctx, tx, `SELECT `+tableCols+` FROM tables WHERE snapshot_id = $1 ORDER BY id`, sid,
		func(rows pgx.Rows) error {
			var ts tableScan
			if err := rows.Scan(ts.dest()...); err != nil {
				return err
			}
			t, err := ts.table(sid)
			if err != nil {
				return err
			}
			t.Columns = []model.Column{}
			t.ColumnLineage = []model.ColumnLineage{}
			t.Relationships = []model.Relationship{}
			tables = append(tables, t)
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("load tables: %w", err)
	}
	// Built after the slice stops growing, so the pointers stay valid.
	byID := make(map[string]*model.Table, len(tables))
	for i := range tables {
		byID[tables[i].ID] = &tables[i]
	}

	err = eachRow(ctx, tx,
		`SELECT table_id, `+columnCols+` FROM columns WHERE snapshot_id = $1 ORDER BY table_id, ordinal`, sid,
		func(rows pgx.Rows) error {
			var tid string
			var c model.Column
			if err := rows.Scan(append([]any{&tid}, columnDest(&c)...)...); err != nil {
				return err
			}
			if t := byID[tid]; t != nil {
				t.Columns = append(t.Columns, c)
			}
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("load columns: %w", err)
	}

	err = eachRow(ctx, tx,
		`SELECT table_id, `+lineageCols+` FROM column_lineage WHERE snapshot_id = $1 ORDER BY table_id, ordinal`, sid,
		func(rows pgx.Rows) error {
			var tid string
			var l model.ColumnLineage
			if err := rows.Scan(append([]any{&tid}, lineageDest(&l)...)...); err != nil {
				return err
			}
			if t := byID[tid]; t != nil {
				t.ColumnLineage = append(t.ColumnLineage, l)
			}
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("load lineage: %w", err)
	}

	err = eachRow(ctx, tx,
		`SELECT `+relationshipCols+` FROM relationships WHERE snapshot_id = $1 ORDER BY from_table_id, id`, sid,
		func(rows pgx.Rows) error {
			var rs relationshipScan
			if err := rows.Scan(rs.dest()...); err != nil {
				return err
			}
			r, err := rs.relationship()
			if err != nil {
				return err
			}
			if t := byID[r.FromTableID]; t != nil {
				t.Relationships = append(t.Relationships, r)
			}
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("load relationships: %w", err)
	}

	return &model.Model{
		Snapshot:     sn,
		Domains:      domains,
		Tables:       tables,
		SourceTables: []model.SourceTable{},
		Diagnostics:  []model.Diagnostic{},
	}, nil
}

func eachRow(ctx context.Context, q querier, sql, sid string, fn func(pgx.Rows) error) error {
	rows, err := q.Query(ctx, sql, sid)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
