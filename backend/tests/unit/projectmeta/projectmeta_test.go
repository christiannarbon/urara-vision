// The manifest a documentation directory must carry.
//
// Everything here is about refusal: what the parser accepts, what it rejects,
// and whether the rejection says enough to fix the file. A manifest is written
// by hand, usually once, so an unhelpful error is a real cost.
package projectmeta_test

import (
	"strings"
	"testing"

	"urara-vision/backend/internal/projectmeta"
	"urara-vision/backend/tests/fixtures"
)

func TestParseReadsACompleteManifest(t *testing.T) {
	meta, err := projectmeta.Parse(fixtures.ProjectMetaTOML)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := fixtures.ProjectMeta()
	if meta.Project != want.Project {
		t.Errorf("project = %+v, want %+v", meta.Project, want.Project)
	}
	if got := strings.Join(meta.Internationalization.Supported, ","); got != "EN,JP" {
		t.Errorf("supported = %q, want \"EN,JP\"", got)
	}
	if meta.Internationalization.Primary != "EN" || meta.Internationalization.Type != "inline" {
		t.Errorf("internationalization = %+v", meta.Internationalization)
	}
}

// TestParseNormalises: a manifest written in either case describes the same
// languages, so the tags are upper-cased and the surrounding space is dropped
// before anything compares them.
func TestParseNormalises(t *testing.T) {
	meta, err := projectmeta.Parse(`
[project]
name = "  spaced-project  "
version = " 1.2.3 "

[internationalization]
primary = "en"
supported = ["en", " ja "]
type = "INLINE"
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if meta.Project.Name != "spaced-project" || meta.Project.Version != "1.2.3" {
		t.Errorf("project = %+v, want the values trimmed", meta.Project)
	}
	if got := strings.Join(meta.Internationalization.Supported, ","); got != "EN,JA" {
		t.Errorf("supported = %q, want \"EN,JA\"", got)
	}
	if meta.Internationalization.Primary != "EN" {
		t.Errorf("primary = %q, want \"EN\"", meta.Internationalization.Primary)
	}
	if meta.Internationalization.Type != "inline" {
		t.Errorf("type = %q, want \"inline\"", meta.Internationalization.Type)
	}
}

// TestParseRejects covers one broken manifest per rule. The assertion is on the
// text as well as the failure, because the message is the whole remedy: nothing
// else tells the author which line to edit.
func TestParseRejects(t *testing.T) {
	cases := []struct {
		name, toml, want string
	}{
		{
			name: "not toml at all",
			toml: "this is not = = toml",
			want: "not valid",
		},
		{
			name: "empty file",
			toml: "",
			want: "project.name is required",
		},
		{
			name: "no version",
			toml: "[project]\nname = \"p\"\n[internationalization]\nprimary = \"EN\"\nsupported = [\"EN\"]\ntype = \"inline\"\n",
			want: "project.version is required",
		},
		{
			name: "no languages",
			toml: "[project]\nname = \"p\"\nversion = \"1\"\n[internationalization]\nprimary = \"EN\"\nsupported = []\ntype = \"inline\"\n",
			want: "must list at least one language",
		},
		{
			name: "primary outside supported",
			toml: "[project]\nname = \"p\"\nversion = \"1\"\n[internationalization]\nprimary = \"FR\"\nsupported = [\"EN\"]\ntype = \"inline\"\n",
			want: "supported does not list",
		},
		{
			name: "duplicate language",
			toml: "[project]\nname = \"p\"\nversion = \"1\"\n[internationalization]\nprimary = \"EN\"\nsupported = [\"EN\", \"en\"]\ntype = \"inline\"\n",
			want: "twice",
		},
		{
			name: "not a language tag",
			toml: "[project]\nname = \"p\"\nversion = \"1\"\n[internationalization]\nprimary = \"EN\"\nsupported = [\"EN\", \"English\"]\ntype = \"inline\"\n",
			want: "not a language tag",
		},
		{
			name: "unknown translation type",
			toml: "[project]\nname = \"p\"\nversion = \"1\"\n[internationalization]\nprimary = \"EN\"\nsupported = [\"EN\"]\ntype = \"sidecar\"\n",
			want: "only inline is supported",
		},
		{
			name: "missing translation type",
			toml: "[project]\nname = \"p\"\nversion = \"1\"\n[internationalization]\nprimary = \"EN\"\nsupported = [\"EN\"]\n",
			want: "internationalization.type is required",
		},
		{
			// A typo silently dropped is metadata the project believes it has.
			name: "misspelled key",
			toml: "[project]\nname = \"p\"\nversion = \"1\"\ndescriptoin = \"typo\"\n[internationalization]\nprimary = \"EN\"\nsupported = [\"EN\"]\ntype = \"inline\"\n",
			want: `unknown key "project.descriptoin"`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := projectmeta.Parse(c.toml)
			if err == nil {
				t.Fatal("manifest was accepted")
			}
			if !projectmeta.IsInvalid(err) {
				t.Errorf("error = %T, want *projectmeta.Invalid", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to mention %q", err, c.want)
			}
			if !strings.Contains(err.Error(), projectmeta.FileName) {
				t.Errorf("error = %q, want it to name the file it is about", err)
			}
		})
	}
}

// TestParseReportsEveryProblemAtOnce, so a manifest written from scratch is
// fixed in one pass rather than one mistake per run.
func TestParseReportsEveryProblemAtOnce(t *testing.T) {
	_, err := projectmeta.Parse("[project]\ndescription = \"nothing else\"\n")
	if err == nil {
		t.Fatal("an all-but-empty manifest was accepted")
	}
	for _, want := range []string{
		"project.name is required",
		"project.version is required",
		"internationalization.primary is required",
		"internationalization.supported must list at least one language",
		"internationalization.type is required",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

// TestDescriptionIsOptional: it is the one field a project can reasonably not
// have decided on yet.
func TestDescriptionIsOptional(t *testing.T) {
	meta, err := projectmeta.Parse(
		"[project]\nname = \"p\"\nversion = \"1\"\n[internationalization]\nprimary = \"EN\"\nsupported = [\"EN\"]\ntype = \"inline\"\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if meta.Project.Description != "" {
		t.Errorf("description = %q, want empty", meta.Project.Description)
	}
}
