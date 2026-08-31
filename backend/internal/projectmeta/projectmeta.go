// Package projectmeta reads the manifest a documentation directory has to
// carry before any of it is parsed.
//
// The parser reads whatever documents it is given and describes what it found.
// This file is the opposite: it is the one thing a directory must state about
// itself, so an ingest knows what project it is looking at and which languages
// the documentation is written in. A directory without it is not parsed at all.
//
// Everything wrong with a manifest is reported at once. Someone writing this
// file for the first time should learn about all four mistakes in one run
// rather than one per attempt.
package projectmeta

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"urara-vision/backend/internal/model"
)

// FileName is the manifest's name, at the root of the selected directory.
const FileName = "projectmeta.toml"

// The translation strategies internationalization.type accepts. "inline" means
// every language lives in the documents themselves. It is the only strategy the
// parser implements today, and an unrecognised value is refused rather than
// ignored: a manifest that declares a strategy nothing acts on is worse than
// one that does not compile.
const TypeInline = "inline"

// Types is every accepted internationalization.type, in the order an error
// message lists them.
var Types = []string{TypeInline}

// ErrMissing is returned when there is no manifest to read at all.
var ErrMissing = fmt.Errorf("%s is required at the root of the documentation directory", FileName)

// languageTag is deliberately loose: a manifest names the languages its author
// documents in, and this only checks the shape of a tag rather than asserting
// it exists. "EN", "JP", "pt-BR" and "zh-Hans" all pass.
var languageTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// Invalid carries every problem found in one manifest.
type Invalid struct {
	Problems []string
}

func (e *Invalid) Error() string {
	return fmt.Sprintf("%s is not valid: %s", FileName, strings.Join(e.Problems, "; "))
}

// IsInvalid reports whether err came from a manifest that was read but refused.
func IsInvalid(err error) bool {
	var inv *Invalid
	return errors.As(err, &inv)
}

// Parse reads a manifest and validates it. The returned metadata is normalised:
// values are trimmed, and language tags are upper-cased so "en" and "EN" are
// the same language wherever they are compared.
func Parse(content string) (model.ProjectMeta, error) {
	var raw struct {
		Project struct {
			Name        string `toml:"name"`
			Version     string `toml:"version"`
			Description string `toml:"description"`
		} `toml:"project"`
		I18n struct {
			Primary   string   `toml:"primary"`
			Supported []string `toml:"supported"`
			Type      string   `toml:"type"`
		} `toml:"internationalization"`
	}

	md, err := toml.Decode(content, &raw)
	if err != nil {
		return model.ProjectMeta{}, &Invalid{Problems: []string{err.Error()}}
	}

	var problems []string

	// An unrecognised key is a typo far more often than it is a plan, and a
	// silently dropped `descriptoin` is metadata the project thinks it has.
	for _, key := range md.Undecoded() {
		problems = append(problems, fmt.Sprintf("unknown key %q", key.String()))
	}

	meta := model.ProjectMeta{
		Project: model.Project{
			Name:        strings.TrimSpace(raw.Project.Name),
			Version:     strings.TrimSpace(raw.Project.Version),
			Description: strings.TrimSpace(raw.Project.Description),
		},
		Internationalization: model.Internationalization{
			Primary:   normaliseTag(raw.I18n.Primary),
			Supported: []string{},
			Type:      strings.ToLower(strings.TrimSpace(raw.I18n.Type)),
		},
	}

	if meta.Project.Name == "" {
		problems = append(problems, "project.name is required")
	}
	if meta.Project.Version == "" {
		problems = append(problems, "project.version is required")
	}

	seen := map[string]bool{}
	for _, s := range raw.I18n.Supported {
		tag := normaliseTag(s)
		if tag == "" {
			problems = append(problems, "internationalization.supported contains an empty entry")
			continue
		}
		if !languageTag.MatchString(tag) {
			problems = append(problems, fmt.Sprintf("internationalization.supported contains %q, which is not a language tag", tag))
			continue
		}
		if seen[tag] {
			problems = append(problems, fmt.Sprintf("internationalization.supported lists %q twice", tag))
			continue
		}
		seen[tag] = true
		meta.Internationalization.Supported = append(meta.Internationalization.Supported, tag)
	}
	if len(meta.Internationalization.Supported) == 0 && !hasProblemAbout(problems, "supported") {
		problems = append(problems, "internationalization.supported must list at least one language")
	}

	switch {
	case meta.Internationalization.Primary == "":
		problems = append(problems, "internationalization.primary is required")
	case !languageTag.MatchString(meta.Internationalization.Primary):
		problems = append(problems, fmt.Sprintf("internationalization.primary is %q, which is not a language tag",
			meta.Internationalization.Primary))
	case !seen[meta.Internationalization.Primary]:
		problems = append(problems, fmt.Sprintf("internationalization.primary is %q, which internationalization.supported does not list",
			meta.Internationalization.Primary))
	}

	switch {
	case meta.Internationalization.Type == "":
		problems = append(problems, "internationalization.type is required")
	case !isKnownType(meta.Internationalization.Type):
		problems = append(problems, fmt.Sprintf("internationalization.type is %q, but only %s is supported",
			meta.Internationalization.Type, strings.Join(Types, ", ")))
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return model.ProjectMeta{}, &Invalid{Problems: problems}
	}
	return meta, nil
}

// normaliseTag trims a language tag and upper-cases it, so a manifest written
// in either case compares the same everywhere.
func normaliseTag(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

// hasProblemAbout keeps an empty `supported` from being reported twice: once
// for the entries that were refused, and again for the list ending up empty.
func hasProblemAbout(problems []string, field string) bool {
	for _, p := range problems {
		if strings.Contains(p, field) {
			return true
		}
	}
	return false
}

func isKnownType(v string) bool {
	for _, t := range Types {
		if t == v {
			return true
		}
	}
	return false
}
