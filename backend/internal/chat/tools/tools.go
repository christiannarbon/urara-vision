// Package tools is the fixed set of tools an agent may call, bound to one snapshot.
package tools

import (
	"context"
	"encoding/json"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/model"
)

type Spec struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Run         func(ctx context.Context, args json.RawMessage) (any, error)
}

// Backend is the part of apiclient.Client the tools call.
type Backend interface {
	Domains(ctx context.Context, sid string) ([]model.Domain, error)
	Tables(ctx context.Context, sid, domain string) ([]apiclient.TableSummary, error)
	TablesDetail(ctx context.Context, sid string, ids []string) (apiclient.TablesDetail, error)
	Search(ctx context.Context, sid, query string, limit int) ([]apiclient.SearchHit, error)
	Neighbourhood(ctx context.Context, sid, tableID string, depth int, sources bool) (apiclient.Graph, error)
	JoinPaths(ctx context.Context, sid, from, to string, maxDepth, limit int) ([]apiclient.JoinPath, error)
	Lineage(ctx context.Context, sid, tableID, direction string) ([]apiclient.LineageEntry, error)
	Diagnostics(ctx context.Context, sid, severity string) ([]model.Diagnostic, error)
	Sources(ctx context.Context, sid string) ([]model.SourceTable, error)
}

// joinPathLimit is the Python client's default.
const joinPathLimit = 10

var names = []string{
	"list_domains",
	"list_tables",
	"get_tables",
	"search_model",
	"get_neighbourhood",
	"find_join_paths",
	"get_lineage",
	"list_diagnostics",
	"list_source_models",
}

// Names lists the tools in the order Build returns them.
func Names() []string { return append([]string(nil), names...) }

// decode fills dst over its defaults; empty args mean none given.
func decode(args json.RawMessage, dst any) error {
	if len(args) == 0 {
		return nil
	}
	return json.Unmarshal(args, dst)
}

// Build binds every tool to snapshot sid, which the model can never choose.
func Build(b Backend, sid string) []Spec {
	return []Spec{
		{
			Name: "list_domains",
			Description: "List the subject areas the model is divided into, with a short " +
				"description and table count for each. Use this first when the reader " +
				"asks what the model covers, or when you need a domain ID to narrow a " +
				"later call. Returns one entry per domain.",
			Schema: json.RawMessage(noArgsSchema),
			Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
				domains, err := b.Domains(ctx, sid)
				if err != nil {
					return nil, err
				}
				items := make([]any, len(domains))
				for i, d := range domains {
					items[i] = map[string]any{
						"id": d.ID, "title": d.Title, "description": d.Description, "tableCount": d.TableCount,
					}
				}
				return Capped(items), nil
			},
		},
		{
			Name: "list_tables",
			Description: "List tables, optionally within one domain, with their kind, grain and " +
				"column count. Use this to find out what exists before reading anything " +
				"in full, or to answer questions about how many tables of a kind there " +
				"are. Returns summaries, not documents: call get_tables for the detail. " +
				"The optional 'domain' argument is a domain ID such as 'ordering'.",
			Schema: json.RawMessage(listTablesSchema),
			Run: func(ctx context.Context, args json.RawMessage) (any, error) {
				var a struct {
					Domain string `json:"domain"`
				}
				if err := decode(args, &a); err != nil {
					return nil, err
				}
				tables, err := b.Tables(ctx, sid, a.Domain)
				if err != nil {
					return nil, err
				}
				items := make([]any, len(tables))
				for i, t := range tables {
					items[i] = Prune(map[string]any{
						"id": t.ID, "name": t.Name, "domainId": t.DomainID, "kind": string(t.Kind),
						"grain": t.Grain, "columnCount": t.ColumnCount, "conformed": t.Conformed,
					})
				}
				return Capped(items), nil
			},
		},
		{
			Name: "get_tables",
			Description: "Read up to eight table documents in full: columns with types and " +
				"descriptions, declared relationships, column-level lineage and documented " +
				"caveats. " +
				"This is the tool that answers questions about what a table contains or " +
				"means. Ask for every table you need in one call rather than one at a " +
				"time. IDs are full 'domain/table' identifiers, e.g. " +
				"'ordering/fact_orders'; call search_model or list_tables first if you " +
				"only have a name. IDs that do not exist come back in 'missing' rather " +
				"than failing the call.",
			Schema: json.RawMessage(getTablesSchema),
			Run: func(ctx context.Context, args json.RawMessage) (any, error) {
				var a struct {
					IDs []string `json:"ids"`
				}
				if err := decode(args, &a); err != nil {
					return nil, err
				}
				batch, err := b.TablesDetail(ctx, sid, a.IDs)
				if err != nil {
					return nil, err
				}
				return shrinkBatch(batch)
			},
		},
		{
			Name: "search_model",
			Description: "Full-text search over table names, grains, and column names and " +
				"descriptions. Use this when the reader names something in their own " +
				"words rather than by ID -- 'where do we store refunds' -- and to turn " +
				"a name into the 'domain/table' ID the other tools need. Returns ranked " +
				"hits with the field that matched, not the documents themselves.",
			Schema: json.RawMessage(searchSchema),
			Run: func(ctx context.Context, args json.RawMessage) (any, error) {
				a := struct {
					Query string `json:"query"`
					Limit int    `json:"limit"`
				}{Limit: 20}
				if err := decode(args, &a); err != nil {
					return nil, err
				}
				hits, err := b.Search(ctx, sid, a.Query, a.Limit)
				if err != nil {
					return nil, err
				}
				items := make([]any, len(hits))
				for i, h := range hits {
					items[i] = Prune(map[string]any{
						"tableId": h.TableID, "name": h.Name, "domainId": h.DomainID, "kind": string(h.Kind),
						"grain": h.Grain, "matchedOn": strs(h.MatchedOn),
					})
				}
				return Capped(items), nil
			},
		},
		{
			Name: "get_neighbourhood",
			Description: "Show what a table joins to, out to a given number of hops. Use this " +
				"when the reader asks what surrounds a table, what it connects to, or " +
				"what else they would need to answer a question from it. Returns the " +
				"nearby tables and the joins between them, with the columns joined on. " +
				"'table_id' is a full 'domain/table' ID; 'depth' is 1 to 3, and 1 is " +
				"usually enough.",
			Schema: json.RawMessage(neighbourhoodSchema),
			Run: func(ctx context.Context, args json.RawMessage) (any, error) {
				a := struct {
					TableID string `json:"table_id"`
					Depth   int    `json:"depth"`
				}{Depth: 1}
				if err := decode(args, &a); err != nil {
					return nil, err
				}
				g, err := b.Neighbourhood(ctx, sid, a.TableID, a.Depth, false)
				if err != nil {
					return nil, err
				}
				return shrinkGraph(g), nil
			},
		},
		{
			Name: "find_join_paths",
			Description: "Find how to get from one table to another by following declared " +
				"joins. Use this when the reader asks how two tables relate or what the " +
				"join path between them is. Returns each path as an ordered list of " +
				"hops with the columns joined at each step. Both arguments are full " +
				"table IDs in 'domain/table' form; call search_model first if you only " +
				"have a name.",
			Schema: json.RawMessage(joinPathsSchema),
			Run: func(ctx context.Context, args json.RawMessage) (any, error) {
				a := struct {
					FromTable string `json:"from_table"`
					ToTable   string `json:"to_table"`
					MaxDepth  int    `json:"max_depth"`
				}{MaxDepth: 4}
				if err := decode(args, &a); err != nil {
					return nil, err
				}
				paths, err := b.JoinPaths(ctx, sid, a.FromTable, a.ToTable, a.MaxDepth, joinPathLimit)
				if err != nil {
					return nil, err
				}
				items := make([]any, len(paths))
				for i, p := range paths {
					items[i] = shrinkPath(p)
				}
				return Capped(items), nil
			},
		},
		{
			Name: "get_lineage",
			Description: "Trace where a table's data comes from, or what is built on it. Use " +
				"'upstream' to find the source models feeding a table, and 'downstream' " +
				"on a source model to find every table built from it -- the tool to " +
				"reach for when the reader asks what breaks if something upstream " +
				"changes. Returns the entries and the columns each contributes. " +
				"'table_id' is a full 'domain/table' ID, or a source model ID such as " +
				"'warehouse.stg_orders' when going downstream.",
			Schema: json.RawMessage(lineageSchema),
			Run: func(ctx context.Context, args json.RawMessage) (any, error) {
				a := struct {
					TableID   string `json:"table_id"`
					Direction string `json:"direction"`
				}{Direction: "upstream"}
				if err := decode(args, &a); err != nil {
					return nil, err
				}
				entries, err := b.Lineage(ctx, sid, a.TableID, a.Direction)
				if err != nil {
					return nil, err
				}
				items := make([]any, len(entries))
				for i, e := range entries {
					items[i] = Prune(map[string]any{
						"id": e.ID, "label": e.Label, "dataset": e.Dataset, "domainId": e.DomainID, "columns": strs(e.Columns),
					})
				}
				result := Capped(items)
				result["direction"] = a.Direction
				// Lets an answer about this table cite it; entries are its sources.
				result["tableId"] = a.TableID
				return result, nil
			},
		},
		{
			Name: "list_diagnostics",
			Description: "List the problems found in the documentation itself: unresolved " +
				"references, missing grains, ambiguous joins. Use this when the reader " +
				"asks about the quality or completeness of the documentation, or when a " +
				"table looks wrong and you want to know whether it is already known to " +
				"be. Returns each with its severity, code, message and where it points. " +
				"Filter with 'severity' as 'error', 'warning' or 'info'.",
			Schema: json.RawMessage(diagnosticsSchema),
			Run: func(ctx context.Context, args json.RawMessage) (any, error) {
				var a struct {
					Severity string `json:"severity"`
				}
				if err := decode(args, &a); err != nil {
					return nil, err
				}
				diags, err := b.Diagnostics(ctx, sid, a.Severity)
				if err != nil {
					return nil, err
				}
				items := make([]any, len(diags))
				for i, d := range diags {
					items[i] = Prune(map[string]any{
						"severity": d.Severity, "code": d.Code, "message": d.Message,
						"domainId": d.DomainID, "tableId": d.TableID,
					})
				}
				return Capped(items), nil
			},
		},
		{
			Name: "list_source_models",
			Description: "List the upstream models the documented tables are built from, with " +
				"how many tables reference each. Use this to answer what the model is " +
				"sourced from, or to find the most depended-upon inputs. Returns source " +
				"IDs such as 'warehouse.stg_orders'; pass one to get_lineage with " +
				"direction 'downstream' to see what is built on it.",
			Schema: json.RawMessage(noArgsSchema),
			Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
				sources, err := b.Sources(ctx, sid)
				if err != nil {
					return nil, err
				}
				items := make([]any, len(sources))
				for i, s := range sources {
					items[i] = map[string]any{"id": s.ID, "dataset": s.Dataset, "name": s.Name, "refs": s.Refs}
				}
				return Capped(items), nil
			},
		},
	}
}
