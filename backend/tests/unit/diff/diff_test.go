// Comparing two hand-built models.
package diff_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"urara-vision/backend/internal/diff"
	"urara-vision/backend/internal/model"
)

func base() *model.Model {
	return &model.Model{
		Domains: []model.Domain{
			{ID: "ordering", Title: "Ordering", Description: "Orders."},
			{ID: "customer", Title: "Customer", Description: "Customers."},
		},
		Tables: []model.Table{
			{
				ID: "ordering/fact_orders", DomainID: "ordering", Kind: model.KindFact, Grain: "one row per order",
				Columns: []model.Column{
					{Name: "order_id", Type: "int", Ordinal: 1, IsPK: true},
					{Name: "customer_id", Type: "int", Ordinal: 2, IsFK: true},
					{Name: "amount", Type: "int", Ordinal: 3},
				},
				ColumnLineage: []model.ColumnLineage{
					{Column: "amount", SourceTable: "raw.orders", SourceColumn: "amount"},
				},
				Relationships: []model.Relationship{
					{ID: "r1", FromTableID: "ordering/fact_orders", ToTableID: "customer/dim_customer",
						TargetRef: "dim_customer", FromColumn: "customer_id", ToColumn: "customer_id",
						Cardinality: "many-to-one", Resolution: model.ResolvedLocal},
				},
			},
			{
				ID: "customer/dim_customer", DomainID: "customer", Kind: "dimension",
				Columns: []model.Column{{Name: "customer_id", Type: "int", Ordinal: 1, IsPK: true}},
			},
		},
	}
}

// clone deep-copies through JSON so edits to one side never leak into the other.
func clone(t *testing.T, m *model.Model) *model.Model {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out model.Model
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

func table(m *model.Model, id string) *model.Table {
	for i := range m.Tables {
		if m.Tables[i].ID == id {
			return &m.Tables[i]
		}
	}
	return nil
}

func TestIdenticalModelsProduceEmptyDiff(t *testing.T) {
	a := base()
	r := diff.Compare(a, clone(t, a))
	if r.Summary != (diff.Summary{}) {
		t.Errorf("summary = %+v, want zero", r.Summary)
	}
	if r.Domains == nil || r.Tables == nil || r.Relationships == nil || r.Lineage == nil {
		t.Fatal("a result slice is nil")
	}
	if len(r.Domains)+len(r.Tables)+len(r.Relationships)+len(r.Lineage) != 0 {
		t.Errorf("expected no entries, got %+v", r)
	}
}

func TestTableAddedAndRemoved(t *testing.T) {
	a := base()
	b := clone(t, a)
	b.Tables = b.Tables[:1] // drop dim_customer
	b.Tables = append(b.Tables, model.Table{
		ID: "ordering/dim_date", DomainID: "ordering",
		Columns: []model.Column{{Name: "date_key", Type: "date"}},
	})

	r := diff.Compare(a, b)
	if r.Summary.Tables != (diff.Counts{Added: 1, Removed: 1}) {
		t.Errorf("tables = %+v", r.Summary.Tables)
	}
	if r.Summary.Columns != (diff.Counts{}) {
		t.Errorf("columns of added/removed tables counted: %+v", r.Summary.Columns)
	}
	want := map[string]diff.Change{"customer/dim_customer": diff.Removed, "ordering/dim_date": diff.Added}
	for _, td := range r.Tables {
		if want[td.ID] != td.Change {
			t.Errorf("%s: change %q", td.ID, td.Change)
		}
		if td.Columns == nil || len(td.Columns) != 0 {
			t.Errorf("%s: columns = %#v, want []", td.ID, td.Columns)
		}
	}
}

func TestColumnTypeChanged(t *testing.T) {
	a := base()
	b := clone(t, a)
	table(b, "ordering/fact_orders").Columns[2].Type = "numeric"

	r := diff.Compare(a, b)
	if len(r.Tables) != 1 {
		t.Fatalf("tables = %+v", r.Tables)
	}
	td := r.Tables[0]
	if td.Change != diff.Changed || td.Fields == nil || len(td.Fields) != 0 {
		t.Errorf("table = %+v, want changed with empty fields", td)
	}
	want := []diff.ColumnDiff{{Name: "amount", Change: diff.Changed,
		Fields: []diff.FieldChange{{Field: "type", From: "int", To: "numeric"}}}}
	if !reflect.DeepEqual(td.Columns, want) {
		t.Errorf("columns = %+v", td.Columns)
	}
	if r.Summary.Columns != (diff.Counts{Changed: 1}) {
		t.Errorf("column counts = %+v", r.Summary.Columns)
	}
}

func TestColumnInsertedShiftsOrdinalsOnly(t *testing.T) {
	a := base()
	b := clone(t, a)
	ft := table(b, "ordering/fact_orders")
	ft.Columns = []model.Column{
		{Name: "order_id", Type: "int", Ordinal: 1, IsPK: true},
		{Name: "order_date", Type: "date", Ordinal: 2},
		{Name: "customer_id", Type: "int", Ordinal: 3, IsFK: true},
		{Name: "amount", Type: "int", Ordinal: 4},
	}

	r := diff.Compare(a, b)
	if len(r.Tables) != 1 || len(r.Tables[0].Columns) != 1 {
		t.Fatalf("tables = %+v", r.Tables)
	}
	if c := r.Tables[0].Columns[0]; c.Name != "order_date" || c.Change != diff.Added {
		t.Errorf("column = %+v", c)
	}
}

func TestTableFieldAndColumnInOneDiff(t *testing.T) {
	a := base()
	b := clone(t, a)
	ft := table(b, "ordering/fact_orders")
	ft.Grain = "one row per order line"
	ft.Columns[2].Description = "Total."

	r := diff.Compare(a, b)
	if len(r.Tables) != 1 {
		t.Fatalf("tables = %+v", r.Tables)
	}
	td := r.Tables[0]
	if len(td.Fields) != 1 || td.Fields[0].Field != "grain" {
		t.Errorf("fields = %+v", td.Fields)
	}
	if len(td.Columns) != 1 || td.Columns[0].Fields[0].Field != "description" {
		t.Errorf("columns = %+v", td.Columns)
	}
}

func TestRelationshipIDIgnored(t *testing.T) {
	a := base()
	b := clone(t, a)
	table(b, "ordering/fact_orders").Relationships[0].ID = "r99"

	if r := diff.Compare(a, b); len(r.Relationships) != 0 {
		t.Errorf("relationships = %+v", r.Relationships)
	}
}

func TestRelationshipCardinalityChanged(t *testing.T) {
	a := base()
	b := clone(t, a)
	table(b, "ordering/fact_orders").Relationships[0].Cardinality = "one-to-one"

	r := diff.Compare(a, b)
	if len(r.Relationships) != 1 {
		t.Fatalf("relationships = %+v", r.Relationships)
	}
	rd := r.Relationships[0]
	want := []diff.FieldChange{{Field: "cardinality", From: "many-to-one", To: "one-to-one"}}
	if rd.Change != diff.Changed || !reflect.DeepEqual(rd.Fields, want) {
		t.Errorf("relationship = %+v", rd)
	}
}

func TestUnresolvedRelationshipMatchedByTargetRef(t *testing.T) {
	unresolved := model.Relationship{FromTableID: "ordering/fact_orders", TargetRef: "dim_store",
		FromColumn: "store_id", ToColumn: "store_id", Resolution: model.ResolvedUnresolved}
	a := base()
	table(a, "ordering/fact_orders").Relationships = append(table(a, "ordering/fact_orders").Relationships, unresolved)
	b := clone(t, a)
	table(b, "ordering/fact_orders").Relationships[1].Cardinality = "many-to-one"

	r := diff.Compare(a, b)
	if len(r.Relationships) != 1 {
		t.Fatalf("relationships = %+v", r.Relationships)
	}
	if rd := r.Relationships[0]; rd.Change != diff.Changed || rd.TargetRef != "dim_store" || rd.ToTableID != "" {
		t.Errorf("relationship = %+v", rd)
	}
}

func TestDuplicateJoinRemovedOnce(t *testing.T) {
	a := base()
	ft := table(a, "ordering/fact_orders")
	dup := ft.Relationships[0]
	dup.ID = "r2"
	ft.Relationships = append(ft.Relationships, dup)
	b := base()

	r := diff.Compare(a, b)
	if len(r.Relationships) != 1 || r.Relationships[0].Change != diff.Removed {
		t.Errorf("relationships = %+v", r.Relationships)
	}
	if r.Summary.Relationships != (diff.Counts{Removed: 1}) {
		t.Errorf("counts = %+v", r.Summary.Relationships)
	}
}

func TestLineageSecondSourceAdded(t *testing.T) {
	a := base()
	b := clone(t, a)
	ft := table(b, "ordering/fact_orders")
	ft.ColumnLineage = append(ft.ColumnLineage,
		model.ColumnLineage{Column: "amount", SourceTable: "raw.refunds", SourceColumn: "amount"})

	r := diff.Compare(a, b)
	if len(r.Lineage) != 1 {
		t.Fatalf("lineage = %+v", r.Lineage)
	}
	if l := r.Lineage[0]; l.Change != diff.Added || l.SourceTable != "raw.refunds" || l.TableID != "ordering/fact_orders" {
		t.Errorf("lineage = %+v", l)
	}
}

func TestDomainDescriptionChanged(t *testing.T) {
	a := base()
	b := clone(t, a)
	b.Domains[0].Description = "Orders and returns."

	r := diff.Compare(a, b)
	want := []diff.DomainDiff{{ID: "ordering", Change: diff.Changed,
		Fields: []diff.FieldChange{{Field: "description", From: "Orders.", To: "Orders and returns."}}}}
	if !reflect.DeepEqual(r.Domains, want) {
		t.Errorf("domains = %+v", r.Domains)
	}
}

// busy makes a diff with an entry in every slice, and several per slice.
func busy(t *testing.T) (*model.Model, *model.Model) {
	t.Helper()
	a := base()
	b := clone(t, a)
	b.Domains[0].Title = "Orders"
	b.Domains[1].Description = "People."
	b.Domains = append(b.Domains, model.Domain{ID: "finance"})
	ft := table(b, "ordering/fact_orders")
	ft.Grain = "line"
	ft.Columns[0].Description = "Key."
	ft.Columns[2].Type = "numeric"
	ft.Columns = append(ft.Columns, model.Column{Name: "tax", Type: "numeric"})
	ft.ColumnLineage = append(ft.ColumnLineage,
		model.ColumnLineage{Column: "tax", SourceTable: "raw.orders", SourceColumn: "tax"},
		model.ColumnLineage{Column: "amount", SourceTable: "raw.refunds", SourceColumn: "amount"})
	ft.Relationships = append(ft.Relationships,
		model.Relationship{FromTableID: ft.ID, TargetRef: "dim_store", FromColumn: "store_id", ToColumn: "store_id"},
		model.Relationship{FromTableID: ft.ID, ToTableID: "finance/dim_date", FromColumn: "date", ToColumn: "date"})
	table(b, "customer/dim_customer").Description = "Who."
	b.Tables = append(b.Tables, model.Table{ID: "finance/dim_date", DomainID: "finance"})
	return a, b
}

func TestOutputIsSortedAndStable(t *testing.T) {
	a, b := busy(t)
	first, err := json.Marshal(diff.Compare(a, b))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		got, _ := json.Marshal(diff.Compare(a, b))
		if !bytes.Equal(first, got) {
			t.Fatalf("run %d differs:\n%s\n%s", i, first, got)
		}
	}

	r := diff.Compare(a, b)
	cols := r.Tables[len(r.Tables)-1].Columns // ordering/fact_orders
	ids := func(n int, at func(int) string) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = at(i)
		}
		return out
	}
	for name, s := range map[string][]string{
		"domains": ids(len(r.Domains), func(i int) string { return r.Domains[i].ID }),
		"tables":  ids(len(r.Tables), func(i int) string { return r.Tables[i].ID }),
		"columns": ids(len(cols), func(i int) string { return cols[i].Name }),
	} {
		if !sort.StringsAreSorted(s) {
			t.Errorf("%s not sorted: %v", name, s)
		}
	}
}

func TestJSONFieldNamesMatchBrief(t *testing.T) {
	a, b := busy(t)
	raw, err := json.Marshal(diff.Compare(a, b))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	keys := func(v any) []string {
		m := v.(map[string]any)
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	first := func(k string) any { return doc[k].([]any)[0] }
	counts := []string{"added", "changed", "removed"}

	checks := map[string]struct {
		got  []string
		want []string
	}{
		"top":          {keys(doc), []string{"domains", "lineage", "relationships", "summary", "tables"}},
		"summary":      {keys(doc["summary"]), []string{"columns", "domains", "lineage", "relationships", "tables"}},
		"counts":       {keys(doc["summary"].(map[string]any)["tables"]), counts},
		"domain":       {keys(first("domains")), []string{"change", "fields", "id"}},
		"field":        {keys(first("domains").(map[string]any)["fields"].([]any)[0]), []string{"field", "from", "to"}},
		"table":        {keys(first("tables")), []string{"change", "columns", "domainId", "fields", "id"}},
		"relationship": {keys(first("relationships")), []string{"change", "fields", "fromColumn", "fromTableId", "targetRef", "toColumn", "toTableId"}},
		"lineage":      {keys(first("lineage")), []string{"change", "column", "fields", "sourceColumn", "sourceTable", "tableId"}},
	}
	for name, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s keys = %v, want %v", name, c.got, c.want)
		}
	}

	var column map[string]any
	for _, td := range doc["tables"].([]any) {
		if cols := td.(map[string]any)["columns"].([]any); len(cols) > 0 {
			column = cols[0].(map[string]any)
		}
	}
	if got := keys(column); !reflect.DeepEqual(got, []string{"change", "fields", "name"}) {
		t.Errorf("column keys = %v", got)
	}
}
