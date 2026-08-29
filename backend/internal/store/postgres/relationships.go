// Reads that run against the grain of the stored relationships: who points at
// this table, and who reads this upstream model.
package postgres

import (
	"context"
	_ "embed"
)

// Referrer is a table that points at the table being inspected.
type Referrer struct {
	TableID     string `json:"tableId"`
	Name        string `json:"name"`
	DomainID    string `json:"domainId"`
	FromColumn  string `json:"fromColumn"`
	ToColumn    string `json:"toColumn"`
	Cardinality string `json:"cardinality"`
}

// IncomingRelationships lists tables declaring a relationship into tableID.
func (s *Store) IncomingRelationships(ctx context.Context, sid, tableID string) ([]Referrer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.from_table_id, t.name, t.domain_id, r.from_column, r.to_column, r.cardinality
		FROM relationships r
		JOIN tables t ON t.snapshot_id = r.snapshot_id AND t.id = r.from_table_id
		WHERE r.snapshot_id = $1 AND r.to_table_id = $2
		ORDER BY t.domain_id, t.name`, sid, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Referrer{}
	for rows.Next() {
		var r Referrer
		if err := rows.Scan(&r.TableID, &r.Name, &r.DomainID, &r.FromColumn, &r.ToColumn, &r.Cardinality); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DownstreamUse reports which tables consume a given source table.
func (s *Store) DownstreamUse(ctx context.Context, sid, sourceTable string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT table_id FROM column_lineage
		 WHERE snapshot_id = $1 AND source_table = $2 ORDER BY table_id`, sid, sourceTable)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
