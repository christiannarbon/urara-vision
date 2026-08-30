// The shipped Fintech demo documentation set, parsed and resolved.
//
// docs/demo/fintech-bi-ddd models a retail bank's star schema in the
// conventions of flexanalytics/dbt-business-intelligence -- surrogate keys
// hashed from a natural key, an stg_* staging layer cited as lineage -- split
// across five bounded contexts and one that has not shipped. Like the Jaffle
// Shop set, every flaw in it is deliberate, so what it produces is pinned here
// rather than left to drift.
package demo_test

import (
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
)

const fintechDir = "../../../../docs/demo/fintech-bi-ddd"

func loadFintech(t *testing.T) *model.Model { return loadSet(t, fintechDir) }

func TestFintechStats(t *testing.T) {
	m := loadFintech(t)
	s := m.Snapshot.Stats
	checkStats(t, []stat{
		{"files parsed", s.FilesParsed, 16},
		{"files skipped", s.FilesSkipped, 0},
		{"domains", s.Domains, 6},
		{"tables", s.Tables, 10},
		{"columns", s.Columns, 108},
		{"conformed instances", s.Conformed, 3}, // dim_customer twice, and the kernel's declared dim_date
		{"declared relationships", s.Relationships, 21},
		{"normalised edges", len(graph.Edges(m)), 12},
		{"lineage edges", s.LineageEdges, 104},
		{"source tables", s.SourceTables, 10},
	})
}

// TestFintechDiagnostics pins the flaws this sample is built around.
func TestFintechDiagnostics(t *testing.T) {
	checkDiagnostics(t, loadFintech(t), map[string]int{
		"unresolved_reference":   1,  // dim_settlement_batch, owned by an unmodelled Treasury
		"cross_domain_reference": 10, // Payments and Lending own almost no dimensions
		"conformed_drift":        1,  // risk_compliance/dim_customer vs Customer Identity's
		"unmatched_join_key":     1,  // repayment_date = calendar_date
		"empty_domain":           1,  // treasury.md has no directory
		"isolated_fact":          1,  // fact_fraud_alerts
		"narrative_reference":    2,  // "Various Fact Tables", "Payments Context Facts"
		"undocumented_lineage":   2,  // prose in the Source Table column
	}, 1) // exactly one error, so `uraractl -strict` on this set exits non-zero
}

// TestFintechJoinKeyOrientation is this sample's headline case, and the same
// shape as Jaffle Shop's: the fact writes its dim_date join key
// dimension-column-first on a Many-to-one row, and the resolver must recover
// the true orientation from the column lists.
func TestFintechJoinKeyOrientation(t *testing.T) {
	got, ok := relationshipTo(loadFintech(t), "payments/fact_payment_transactions", "dim_date")
	if !ok {
		t.Fatal("fact_payment_transactions no longer declares a relationship to dim_date")
	}
	if got.JoinKeyRaw != "date_key = transaction_date_key" {
		t.Fatalf("the document no longer writes the key backwards (%q); that case is the point of the sample", got.JoinKeyRaw)
	}
	if got.FromColumn != "transaction_date_key" || got.ToColumn != "date_key" {
		t.Errorf("oriented key = %q = %q, want transaction_date_key = date_key", got.FromColumn, got.ToColumn)
	}
	if got.ToTableID != "shared_kernel/dim_date" {
		t.Errorf("target = %q, want shared_kernel/dim_date", got.ToTableID)
	}
}

// TestFintechOrientationOnOneToMany is the harder orientation case: dim_account
// declares its One-to-many join to dim_loan with the loan's column written
// first. The two sides have different names, so the written order is the only
// thing pointing the wrong way and the column lists are the only way back.
func TestFintechOrientationOnOneToMany(t *testing.T) {
	got, ok := relationshipTo(loadFintech(t), "customer_identity/dim_account", "dim_loan")
	if !ok {
		t.Fatal("dim_account no longer declares a relationship to dim_loan")
	}
	if got.JoinKeyRaw != "origination_account_key = account_key" {
		t.Fatalf("the document no longer writes the key loan-column-first (%q); that case is the point", got.JoinKeyRaw)
	}
	if got.FromColumn != "account_key" || got.ToColumn != "origination_account_key" {
		t.Errorf("oriented key = %q = %q, want account_key = origination_account_key", got.FromColumn, got.ToColumn)
	}
}

// TestFintechConformedAuthority checks that a borrowed dim_customer binds to
// Customer Identity's instance and not to Risk & Compliance's stale copy.
// Getting this backwards would answer every cross-context customer question
// from a table missing nine attributes.
func TestFintechConformedAuthority(t *testing.T) {
	m := loadFintech(t)
	for _, from := range []string{
		"payments/fact_payment_transactions",
		"lending/fact_loan_repayments",
		"lending/dim_loan",
	} {
		r, ok := relationshipTo(m, from, "dim_customer")
		if !ok {
			t.Errorf("%s no longer declares a relationship to dim_customer", from)
			continue
		}
		if r.ToTableID != "customer_identity/dim_customer" {
			t.Errorf("%s bound dim_customer to %q, want customer_identity/dim_customer", from, r.ToTableID)
		}
	}
}

// TestFintechTwoSidedDeclarationsCollapse checks the six joins this sample
// declares from both sides, which must each be a single edge.
func TestFintechTwoSidedDeclarationsCollapse(t *testing.T) {
	checkTwoSided(t, loadFintech(t), map[string][]string{
		"customer_identity/dim_account->customer_identity/dim_customer": {
			"customer_identity/dim_account", "customer_identity/dim_customer"},
		"lending/dim_loan->customer_identity/dim_account": {
			"customer_identity/dim_account", "lending/dim_loan"},
		"lending/fact_loan_repayments->lending/dim_loan": {
			"lending/dim_loan", "lending/fact_loan_repayments"},
		"payments/dim_card->customer_identity/dim_account": {
			"customer_identity/dim_account", "payments/dim_card"},
		"payments/fact_payment_transactions->payments/dim_card": {
			"payments/dim_card", "payments/fact_payment_transactions"},
		"payments/fact_payment_transactions->payments/dim_merchant": {
			"payments/dim_merchant", "payments/fact_payment_transactions"},
	})
}

// TestFintechSourceCanonicalisation checks the sample's inconsistent citation
// of the card network's transaction feed folds onto a single lineage node.
func TestFintechSourceCanonicalisation(t *testing.T) {
	m := loadFintech(t)
	for _, s := range m.SourceTables {
		if s.ID == "stg_transaction" {
			t.Errorf("bare stg_transaction survived as its own source node; it should fold onto card_network.stg_transaction")
		}
		if s.Dataset == "" {
			t.Errorf("source %q has no dataset; every source in this set is qualified", s.ID)
		}
	}
	readers := map[string]bool{}
	for _, tb := range m.Tables {
		for _, l := range tb.ColumnLineage {
			if l.SourceTable == "card_network.stg_transaction" {
				readers[tb.ID] = true
			}
		}
	}
	for _, want := range []string{
		"payments/fact_payment_transactions", // cited it qualified
		"customer_identity/dim_customer",     // cited it qualified
		"payments/dim_merchant",              // cited it bare
	} {
		if !readers[want] {
			t.Errorf("%s does not read card_network.stg_transaction; the mixed-citation case is the point", want)
		}
	}
}
