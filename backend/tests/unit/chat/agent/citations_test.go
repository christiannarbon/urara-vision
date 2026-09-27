// Ported from chat/tests/unit/test_citations.py.
package agent_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"urara-vision/backend/internal/chat/agent"
)

const (
	factOrders   = "ordering/fact_orders"
	dimCustomers = "customer_identity/dim_customers"
)

func toolResult(ids ...string) map[string]any {
	items := make([]any, len(ids))
	for i, id := range ids {
		_, name, _ := strings.Cut(id, "/")
		items[i] = map[string]any{"id": id, "name": name}
	}
	return map[string]any{"items": items, "truncated": false, "total": len(ids)}
}

func results(r ...any) []any { return r }

type citationCase struct {
	name      string
	results   []any
	answer    string
	want      []string
	unordered bool
}

func citationCases() []citationCase {
	var thirty []string
	var thirtyNames []string
	for i := range 30 {
		thirty = append(thirty, fmt.Sprintf("d%d/table_%d", i, i))
		thirtyNames = append(thirtyNames, fmt.Sprintf("table_%d", i))
	}

	self := map[string]any{"id": factOrders}
	self["self"] = self
	self["siblings"] = []any{self}

	a := map[string]any{"id": factOrders}
	b := map[string]any{"id": dimCustomers, "other": a}
	a["other"] = b

	deep := map[string]any{"id": factOrders}
	for range 200 {
		deep = map[string]any{"nested": deep}
	}

	cases := []citationCase{
		// The two rules that matter.
		{name: "full ID", results: results(toolResult(factOrders)), answer: "See " + factOrders + " for that.", want: []string{factOrders}},
		{name: "bare name", results: results(toolResult(factOrders)), answer: "fact_orders holds one row per order.", want: []string{factOrders}},
		{name: "retrieved but never mentioned", results: results(toolResult(factOrders, dimCustomers)), answer: "fact_orders holds one row per order.", want: []string{factOrders}},
		{name: "mentioned but never retrieved", results: results(toolResult(factOrders)), answer: "See " + factOrders + " and also invented/dim_nonsense for the rest.", want: []string{factOrders}},
		{name: "nothing retrieved", answer: "fact_orders and dim_customers explain it", want: []string{}},

		// Ordering.
		{name: "first mention, not tool order", results: results(toolResult(factOrders, dimCustomers)), answer: "dim_customers is joined by fact_orders.", want: []string{dimCustomers, factOrders}},
		{name: "earlier spelling wins", results: results(toolResult(factOrders, dimCustomers)), answer: "fact_orders joins dim_customers; see " + factOrders + ".", want: []string{factOrders, dimCustomers}},
		{name: "duplicates collapse", results: results(toolResult(factOrders)), answer: "fact_orders, fact_orders again, and ordering/fact_orders once more.", want: []string{factOrders}},
		{name: "retrieved twice, cited once", results: results(toolResult(factOrders), toolResult(factOrders)), answer: "fact_orders", want: []string{factOrders}},
		{name: "cap at twenty, first mentioned", results: results(toolResult(thirty...)), answer: strings.Join(thirtyNames, " "), want: thirty[:agent.MaxCitations]},

		// Where IDs are found.
		{name: "join path tables array", results: results(map[string]any{"items": []any{map[string]any{
			"length": 1, "tables": []any{factOrders, dimCustomers},
			"hops": []any{map[string]any{"from": factOrders, "to": dimCustomers, "fromColumn": "c"}},
		}}}), answer: "fact_orders joins dim_customers", want: []string{factOrders, dimCustomers}, unordered: true},
		{name: "graph node IDs and link endpoints", results: results(map[string]any{
			"items": []any{map[string]any{"id": factOrders, "label": "fact_orders"}},
			"links": []any{map[string]any{"source": factOrders, "target": dimCustomers, "type": "joins"}},
		}), answer: "fact_orders reaches dim_customers", want: []string{factOrders, dimCustomers}, unordered: true},
		{name: "tableId key", results: results(map[string]any{"items": []any{map[string]any{"tableId": factOrders, "rank": 0.9}}}), answer: "fact_orders", want: []string{factOrders}},
		{name: "three levels deep", results: results(map[string]any{"items": []any{map[string]any{"table": map[string]any{
			"id": factOrders, "columns": []any{map[string]any{"name": "order_id"}},
		}}}}), answer: "fact_orders", want: []string{factOrders}},
		{name: "prose fields not harvested", results: results(map[string]any{"items": []any{map[string]any{
			"id": factOrders, "description": "Similar in shape to " + dimCustomers + ".",
			"grain": "One row per order in customer_identity/dim_customers terms.",
		}}}), answer: "fact_orders and dim_customers", want: []string{factOrders}},
		{name: "source model IDs are not tables", results: results(map[string]any{"items": []any{map[string]any{"id": "jaffle_shop.stg_orders", "dataset": "jaffle_shop"}}}), answer: "built from jaffle_shop.stg_orders", want: []string{}},
		{name: "bare string in a tables list", results: results(map[string]any{"tables": []any{factOrders}}), answer: "fact_orders", want: []string{factOrders}},
		{name: "missing IDs ignored", results: results(map[string]any{"items": []any{}, "missing": []any{"nope/not_a_table"}, "truncated": false, "total": 0}), answer: "nope/not_a_table does not exist", want: []string{}},

		// Word boundaries.
		{name: "not inside a longer name", results: results(toolResult("shared_kernel/dim_date")), answer: "dim_dates is a different table", want: []string{}},
		{name: "underscore is part of a word", results: results(toolResult("d/date")), answer: "dim_date holds days", want: []string{}},
		{name: "a name after a slash", results: results(toolResult("shared_kernel/dim_date")), answer: "use customer_identity/dim_date instead", want: []string{"shared_kernel/dim_date"}},
		{name: "punctuation around the name", results: results(toolResult(factOrders)), answer: "(fact_orders) fact_orders, 'fact_orders' -fact_orders.", want: []string{factOrders}},

		// Case insensitivity.
		{name: "upper-case answer", results: results(toolResult(factOrders)), answer: "FACT_ORDERS", want: []string{factOrders}},
		{name: "upper-case ID", results: results(toolResult("Ordering/Fact_Orders")), answer: "fact_orders", want: []string{"Ordering/Fact_Orders"}},
		{name: "full ID in either case", results: results(toolResult(factOrders)), answer: "ORDERING/FACT_ORDERS", want: []string{factOrders}},

		// Ambiguity.
		{name: "namesakes both cited", results: results(toolResult("shared_kernel/dim_date", "customer_identity/dim_date")), answer: "join on dim_date", want: []string{"shared_kernel/dim_date", "customer_identity/dim_date"}},
		{name: "a full ID selects only that one", results: results(toolResult("shared_kernel/dim_date", "customer_identity/dim_date")), answer: "join on shared_kernel/dim_date", want: []string{"shared_kernel/dim_date"}},

		// Plain-word names.
		{name: "prose does not cite a plain word", results: results(toolResult("catalog/film", "catalog/film_actor")), answer: "`catalog/film_actor` is one row per film-and-actor pair.", want: []string{"catalog/film_actor"}},
		{name: "a code span cites it", results: results(toolResult("catalog/film")), answer: "join to `film`", want: []string{"catalog/film"}},
		{name: "a column in a code span cites it", results: results(toolResult("catalog/film")), answer: "join on `film.language_id`", want: []string{"catalog/film"}},
		{name: "a longer word in a code span does not", results: results(toolResult("catalog/film")), answer: "see `filmography`", want: []string{}},

		// Degenerate input.
		{name: "empty results", answer: "fact_orders", want: []string{}},
		{name: "empty answer", results: results(toolResult(factOrders)), answer: "", want: []string{}},
		{name: "both empty", answer: "", want: []string{}},
		{name: "self-referential terminates", results: results(self), answer: "fact_orders", want: []string{factOrders}},
		{name: "mutual references terminate", results: results(a), answer: "fact_orders and dim_customers", want: []string{factOrders, dimCustomers}, unordered: true},
		{name: "very deep terminates", results: results(deep), answer: "fact_orders", want: []string{}},
		{name: "non-string values ignored", results: results(map[string]any{"items": []any{map[string]any{"id": nil, "tableId": 42, "source": []any{"not", "a", "string"}}}}), answer: "anything", want: []string{}},
		{name: "tool error string contributes nothing", results: results("No table with id 'nope/missing' in this model. Call search_model to find it."), answer: "nope/missing was not found", want: []string{}},

		// Added for the Go port.
		{name: "not next to an underscore or digit", results: results(toolResult(factOrders)), answer: "_fact_orders fact_orders2 9fact_orders ordering/fact_orders_x", want: []string{}},
		{name: "film in prose is not cited, `film` is", results: results(toolResult("catalog/film")), answer: "a film, then `film`", want: []string{"catalog/film"}},
		{name: "a full ID does not cite its namesake", results: results(toolResult("shared_kernel/dim_date", "customer_identity/dim_date")), answer: "see customer_identity/dim_date, which is not shared_kernel/dim_date", want: []string{"customer_identity/dim_date", "shared_kernel/dim_date"}},
		{name: "a full ID's bare name is not a second mention", results: results(toolResult("b/dim_date", "a/x")), answer: "a/dim_date is not b's; see b/dim_date", want: []string{"b/dim_date"}},
		{name: "namesakes tie in discovery order", results: results(toolResult("z/dim_date"), toolResult("a/dim_date")), answer: "dim_date", want: []string{"z/dim_date", "a/dim_date"}},
		{name: "non-ASCII before a name is a boundary", results: results(toolResult(factOrders)), answer: "注文はfact_ordersにある", want: []string{factOrders}},
		{name: "first spelling of a case duplicate is kept", results: results(toolResult("Ordering/Fact_Orders", factOrders)), answer: "fact_orders", want: []string{"Ordering/Fact_Orders"}},
	}

	// Each invalid ID shape on its own.
	for _, v := range []string{"no-slash", "too/many/slashes", "/leading", "trailing/", "-bad/start"} {
		cases = append(cases, citationCase{name: "not an ID: " + v, results: results(map[string]any{"items": []any{map[string]any{"id": v}}}), answer: v, want: []string{}})
	}
	// test_mention_shapes.
	for _, s := range []struct {
		answer string
		want   []string
	}{
		{"fact_orders", []string{factOrders}},
		{"ordering/fact_orders", []string{factOrders}},
		{"The fact_orders table.", []string{factOrders}},
		{"fact_orderss", []string{}},
		{"xfact_orders", []string{}},
		{"fact_order", []string{}},
		{"", []string{}},
	} {
		cases = append(cases, citationCase{name: "shape " + s.answer, results: results(toolResult(factOrders)), answer: s.answer, want: s.want})
	}
	return cases
}

func TestCitation(t *testing.T) {
	for _, c := range citationCases() {
		t.Run(c.name, func(t *testing.T) {
			got := agent.ExtractCitations(c.results, c.answer)
			if got == nil {
				t.Fatal("nil, want a slice (JSON must be [])")
			}
			if c.unordered {
				got, c.want = slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(c.want))
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Found at depth 12, not at 13.
func TestCitationDepthCap(t *testing.T) {
	for depth, want := range map[int]int{12: 1, 13: 0} {
		var node any = map[string]any{"id": factOrders}
		for range depth {
			node = map[string]any{"nested": node}
		}
		if got := agent.ExtractCitations([]any{node}, "fact_orders"); len(got) != want {
			t.Errorf("depth %d: %q", depth, got)
		}
	}
}

// The lineage tool reports the table it was asked about so an answer can cite it (18.1).
func TestCitationOfALineageResult(t *testing.T) {
	result := map[string]any{"items": []any{map[string]any{"id": "src.model"}}, "direction": "upstream", "tableId": "d/t"}
	if got := agent.ExtractCitations([]any{result}, "`d/t` is built from src.model"); !reflect.DeepEqual(got, []string{"d/t"}) {
		t.Errorf("got %q", got)
	}
}
