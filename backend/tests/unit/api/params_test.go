// Query and path parameter handling: what is required, what is defaulted, and
// what an absent or malformed value must not silently become.
package api_test

import (
	"net/http"
	"testing"
)

// TestMissingRequiredParamsAre400: each of these endpoints needs an identifier
// that cannot be defaulted.
func TestMissingRequiredParamsAre400(t *testing.T) {
	h := newServer(t, &fakeMeta{}, &fakeGraphs{})
	cases := map[string]string{
		"table without id":         "/api/v1/snapshots/s1/table",
		"neighborhood without id":  "/api/v1/snapshots/s1/neighborhood",
		"paths without from or to": "/api/v1/snapshots/s1/paths",
		"paths without to":         "/api/v1/snapshots/s1/paths?from=a/b",
		"lineage without id":       "/api/v1/snapshots/s1/lineage",
	}
	for name, target := range cases {
		rec := do(t, h, http.MethodGet, target, nil, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
		if msg, _ := decode(t, rec.Body.Bytes())["error"].(string); msg == "" {
			t.Errorf("%s: 400 carried no explanation", name)
		}
	}
}

// TestGraphFiltersAreParsed: the filter sidebar sends CSV lists and boolean
// flags, and each has to arrive as the store option it maps to.
func TestGraphFiltersAreParsed(t *testing.T) {
	graphs := &fakeGraphs{}
	h := newServer(t, &fakeMeta{}, graphs)

	rec := do(t, h, http.MethodGet,
		"/api/v1/snapshots/s1/graph?domain=domain_one,+domain_two+,&kind=fact&sources=true&crossDomainOnly=true", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	opt := graphs.graphOpt
	if len(opt.Domains) != 2 || opt.Domains[0] != "domain_one" || opt.Domains[1] != "domain_two" {
		t.Errorf("Domains = %v, want [domain_one domain_two] with the blank entry dropped", opt.Domains)
	}
	if len(opt.Kinds) != 1 || opt.Kinds[0] != "fact" {
		t.Errorf("Kinds = %v", opt.Kinds)
	}
	if !opt.IncludeSources || !opt.CrossDomainOnly {
		t.Errorf("flags = sources %v, crossDomainOnly %v; want both true", opt.IncludeSources, opt.CrossDomainOnly)
	}
}

// TestEmptyFiltersMeanNoFilter: an absent parameter must not become a filter
// on the empty string, which would return nothing at all.
func TestEmptyFiltersMeanNoFilter(t *testing.T) {
	graphs := &fakeGraphs{}
	h := newServer(t, &fakeMeta{}, graphs)

	do(t, h, http.MethodGet, "/api/v1/snapshots/s1/graph?domain=&kind=", nil, "")
	if graphs.graphOpt.Domains != nil || graphs.graphOpt.Kinds != nil {
		t.Errorf("empty parameters became filters: %+v", graphs.graphOpt)
	}
	if graphs.graphOpt.IncludeSources || graphs.graphOpt.CrossDomainOnly {
		t.Error("absent boolean parameters defaulted to true")
	}
}

// TestNumericParamsFallBackToDefaults: a malformed depth or limit must not
// become zero, which would return an empty result for a valid-looking request.
func TestNumericParamsFallBackToDefaults(t *testing.T) {
	graphs := &fakeGraphs{}
	meta := &fakeMeta{}
	h := newServer(t, meta, graphs)

	do(t, h, http.MethodGet, "/api/v1/snapshots/s1/neighborhood?table=a/b&depth=deep", nil, "")
	if graphs.nbrDepth != 1 {
		t.Errorf("depth = %d, want the default 1", graphs.nbrDepth)
	}

	do(t, h, http.MethodGet, "/api/v1/snapshots/s1/paths?from=a/b&to=c/d&maxDepth=x&limit=y", nil, "")
	if graphs.pathDepth != 4 || graphs.pathLimit != 10 {
		t.Errorf("paths = depth %d, limit %d; want the defaults 4 and 10", graphs.pathDepth, graphs.pathLimit)
	}

	do(t, h, http.MethodGet, "/api/v1/snapshots/s1/search?q=user&limit=all", nil, "")
	if meta.searchLimit != 50 {
		t.Errorf("search limit = %d, want the default 50", meta.searchLimit)
	}
	if meta.searchQuery != "user" {
		t.Errorf("search query = %q", meta.searchQuery)
	}
}

// TestNeighborhoodPassesDepthAndSources through to the store.
func TestNeighborhoodPassesDepthAndSources(t *testing.T) {
	graphs := &fakeGraphs{}
	h := newServer(t, &fakeMeta{}, graphs)

	do(t, h, http.MethodGet, "/api/v1/snapshots/s1/neighborhood?table=a/b&depth=3&sources=true", nil, "")
	if graphs.nbrDepth != 3 || !graphs.nbrSources {
		t.Errorf("depth = %d, sources = %v; want 3 and true", graphs.nbrDepth, graphs.nbrSources)
	}
}

// TestLineageDirection: anything other than "downstream" means upstream, so a
// missing or misspelled direction still answers the common question.
func TestLineageDirection(t *testing.T) {
	cases := map[string]string{
		"":                      "upstream",
		"&direction=upstream":   "upstream",
		"&direction=sideways":   "upstream",
		"&direction=downstream": "downstream",
	}
	for suffix, want := range cases {
		graphs := &fakeGraphs{}
		h := newServer(t, &fakeMeta{}, graphs)
		rec := do(t, h, http.MethodGet, "/api/v1/snapshots/s1/lineage?id=a/b"+suffix, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%q: status = %d", suffix, rec.Code)
		}
		if got, _ := decode(t, rec.Body.Bytes())["direction"].(string); got != want {
			t.Errorf("%q: direction = %q, want %q", suffix, got, want)
		}
		if want == "downstream" && graphs.downID != "a/b" {
			t.Errorf("%q: downstream not queried", suffix)
		}
		if want == "upstream" && graphs.upstreamID != "a/b" {
			t.Errorf("%q: upstream not queried", suffix)
		}
	}
}

// TestDiagnosticsSeverityIsPassedThrough so the panel can filter server-side.
func TestDiagnosticsSeverityIsPassedThrough(t *testing.T) {
	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})
	do(t, h, http.MethodGet, "/api/v1/snapshots/s1/diagnostics?severity=error", nil, "")
	if meta.diagsSeverity != "error" {
		t.Errorf("severity = %q", meta.diagsSeverity)
	}
}

// TestListTablesDomainFilterIsPassedThrough.
func TestListTablesDomainFilterIsPassedThrough(t *testing.T) {
	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})
	do(t, h, http.MethodGet, "/api/v1/snapshots/s1/tables?domain=domain_one", nil, "")
	if meta.tablesDomain != "domain_one" {
		t.Errorf("domain = %q", meta.tablesDomain)
	}
}
