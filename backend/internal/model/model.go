// Package model holds the domain types shared by the parser, graph builder,
// stores and API. Everything here is plain data: no persistence concerns.
package model

import "time"

// Resolution records how a declared relationship target was matched to a table.
type Resolution string

const (
	// ResolvedLocal means the target exists inside the same domain directory.
	ResolvedLocal Resolution = "local"
	// ResolvedConformed means the target only exists in other domains and was
	// matched to a conformed instance elsewhere.
	ResolvedConformed Resolution = "conformed"
	// ResolvedUnresolved means no table document matched the reference.
	ResolvedUnresolved Resolution = "unresolved"
	// ResolvedNarrative means the cell held prose ("Various Fact Tables")
	// rather than a table identifier.
	ResolvedNarrative Resolution = "narrative"
)

// Severity levels for ingest diagnostics.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Snapshot is one ingest of a documentation directory.
type Snapshot struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	SourceLabel string      `json:"sourceLabel"`
	CreatedAt   time.Time   `json:"createdAt"`
	Stats       Stats       `json:"stats"`
	Project     ProjectMeta `json:"project"`
}

// ProjectMeta is what a documentation directory declares about itself, read
// from the projectmeta.toml it must carry. Unlike everything else here it is
// not derived from the documents: it is stated, and an ingest is refused
// without it.
type ProjectMeta struct {
	Project              Project              `json:"project"`
	Internationalization Internationalization `json:"internationalization"`
}

// Project identifies the documented project.
type Project struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// Internationalization is the languages the documentation is written in, and
// how the translations are held. Language tags are upper-cased as they are
// read, so "en" and "EN" are one language.
type Internationalization struct {
	Primary   string   `json:"primary"`
	Supported []string `json:"supported"`
	Type      string   `json:"type"`
}

// Stats summarises what an ingest produced.
type Stats struct {
	Domains       int `json:"domains"`
	Tables        int `json:"tables"`
	Columns       int `json:"columns"`
	Relationships int `json:"relationships"`
	LineageEdges  int `json:"lineageEdges"`
	SourceTables  int `json:"sourceTables"`
	Conformed     int `json:"conformed"`
	FilesParsed   int `json:"filesParsed"`
	FilesSkipped  int `json:"filesSkipped"`
	Diagnostics   int `json:"diagnostics"`
	// Translated is how many prose fields carry a language beyond the
	// project's primary one.
	Translated int `json:"translated"`
}

// Domain is one subject area, parsed from a top-level index markdown file.
type Domain struct {
	ID          string          `json:"id"`
	SnapshotID  string          `json:"snapshotId"`
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Mermaid     string          `json:"mermaid,omitempty"`
	Lineage     []DomainLineage `json:"lineage,omitempty"`
	DocPath     string          `json:"docPath"`
	TableCount  int             `json:"tableCount"`
}

// DomainLineage is one row of a domain index's "Lineage" table: a proposed
// table and the source models feeding it.
type DomainLineage struct {
	ProposedTable string   `json:"proposedTable"`
	SourceModels  []string `json:"sourceModels"`
}

// Table is a single table document, whatever role it plays. See roles.go.
type Table struct {
	ID               string          `json:"id"`
	SnapshotID       string          `json:"snapshotId"`
	Name             string          `json:"name"`
	DomainID         string          `json:"domainId"`
	Kind             TableKind       `json:"kind"`
	KindRaw          string          `json:"kindRaw"`
	Grain            string          `json:"grain"`
	UpdateFrequency  string          `json:"updateFrequency"`
	Layer            string          `json:"layer"`
	DomainLabel      string          `json:"domainLabel"`
	Description      string          `json:"description"`
	Columns          []Column        `json:"columns"`
	ColumnLineage    []ColumnLineage `json:"columnLineage"`
	Relationships    []Relationship  `json:"relationships"`
	Notes            []string        `json:"notes"`
	RelationshipNote string          `json:"relationshipNote,omitempty"`
	DocPath          string          `json:"docPath"`
	Conformed        bool            `json:"conformed"`
	ConformedIn      []string        `json:"conformedIn,omitempty"`
}

// Column is one row of a table document's "Columns" table.
type Column struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Ordinal     int    `json:"ordinal"`
	IsPK        bool   `json:"isPk"`
	IsFK        bool   `json:"isFk"`
}

// ColumnLineage is one row of a "Column-Level Lineage" table.
type ColumnLineage struct {
	Column       string `json:"column"`
	SourceTable  string `json:"sourceTable"`
	SourceColumn string `json:"sourceColumn"`
	Notes        string `json:"notes"`
	Derived      bool   `json:"derived"`
}

// Relationship is one edge between two tables, after resolution.
type Relationship struct {
	ID          string     `json:"id"`
	FromTableID string     `json:"fromTableId"`
	ToTableID   string     `json:"toTableId,omitempty"`
	TargetRef   string     `json:"targetRef"`
	FromColumn  string     `json:"fromColumn"`
	ToColumn    string     `json:"toColumn"`
	JoinKeyRaw  string     `json:"joinKeyRaw"`
	Cardinality string     `json:"cardinality"`
	Resolution  Resolution `json:"resolution"`
	Candidates  []string   `json:"candidates,omitempty"`
}

// Diagnostic is a problem or notable observation found during ingest.
type Diagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	DomainID string `json:"domainId,omitempty"`
	TableID  string `json:"tableId,omitempty"`
	DocPath  string `json:"docPath,omitempty"`
}

// SourceTable is an upstream model referenced by column lineage, e.g.
// "warehouse.upstream_model".
type SourceTable struct {
	ID      string `json:"id"`
	Dataset string `json:"dataset"`
	Name    string `json:"name"`
	Refs    int    `json:"refs"`
}

// Model is the complete parsed and resolved result of one ingest.
type Model struct {
	Snapshot     Snapshot      `json:"snapshot"`
	Domains      []Domain      `json:"domains"`
	Tables       []Table       `json:"tables"`
	SourceTables []SourceTable `json:"sourceTables"`
	Diagnostics  []Diagnostic  `json:"diagnostics"`
}
