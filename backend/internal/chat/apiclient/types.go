package apiclient

import (
	"time"

	"urara-vision/backend/internal/model"
)

// Copied from internal/store and internal/api, which the chat service must not
// import. Tags match the originals.

// ChatFeature and Features mirror internal/api's featuresResponse.
type ChatFeature struct {
	Available bool `json:"available"`
	Enabled   bool `json:"enabled"`
}

type Features struct {
	Chat ChatFeature `json:"chat"`
}

// Context mirrors internal/api's contextResponse.
type Context struct {
	Snapshot    ContextSnapshot `json:"snapshot"`
	Domains     []ContextDomain `json:"domains"`
	Tables      []ContextTable  `json:"tables"`
	Diagnostics map[string]int  `json:"diagnostics"`
	Truncated   bool            `json:"truncated"`
}

type ContextSnapshot struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	CreatedAt time.Time         `json:"createdAt"`
	Project   model.ProjectMeta `json:"project"`
	Stats     model.Stats       `json:"stats"`
}

type ContextDomain struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	TableCount  int    `json:"tableCount"`
}

type ContextTable struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	DomainID    string          `json:"domainId"`
	Kind        model.TableKind `json:"kind"`
	Grain       string          `json:"grain"`
	ColumnCount int             `json:"columnCount"`
	Conformed   bool            `json:"conformed,omitempty"`
}

// TableSummary mirrors postgres.TableSummary.
type TableSummary struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	DomainID    string          `json:"domainId"`
	Kind        model.TableKind `json:"kind"`
	Grain       string          `json:"grain"`
	Conformed   bool            `json:"conformed"`
	ColumnCount int             `json:"columnCount"`
	Description string          `json:"description"`
}

// Referrer mirrors postgres.Referrer.
type Referrer struct {
	TableID     string `json:"tableId"`
	Name        string `json:"name"`
	DomainID    string `json:"domainId"`
	FromColumn  string `json:"fromColumn"`
	ToColumn    string `json:"toColumn"`
	Cardinality string `json:"cardinality"`
}

// SearchHit mirrors postgres.SearchHit.
type SearchHit struct {
	TableID   string          `json:"tableId"`
	Name      string          `json:"name"`
	DomainID  string          `json:"domainId"`
	Kind      model.TableKind `json:"kind"`
	Grain     string          `json:"grain"`
	Rank      float64         `json:"rank"`
	MatchedOn []string        `json:"matchedOn,omitempty"`
}

// TableDetail is the /table envelope, a map in internal/api's tableDetail.
type TableDetail struct {
	Table    model.Table    `json:"table"`
	Incoming []Referrer     `json:"incoming"`
	Upstream []LineageEntry `json:"upstream"`
	Siblings []LineageEntry `json:"siblings"`
}

// TablesDetail is the /tables/detail envelope.
type TablesDetail struct {
	Tables  []TableDetail `json:"tables"`
	Missing []string      `json:"missing"`
}

// Node, Link, Graph, PathHop, JoinPath and LineageEntry mirror internal/store/neo4j.
type Node struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	DomainID    string `json:"domainId,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Grain       string `json:"grain,omitempty"`
	Conformed   bool   `json:"conformed,omitempty"`
	ColumnCount int    `json:"columnCount,omitempty"`
	Dataset     string `json:"dataset,omitempty"`
	Refs        int    `json:"refs,omitempty"`
	Degree      int    `json:"degree"`
}

type Link struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Target      string   `json:"target"`
	Type        string   `json:"type"`
	FromColumn  string   `json:"fromColumn,omitempty"`
	ToColumn    string   `json:"toColumn,omitempty"`
	Cardinality string   `json:"cardinality,omitempty"`
	Resolution  string   `json:"resolution,omitempty"`
	CrossDomain bool     `json:"crossDomain,omitempty"`
	Columns     []string `json:"columns,omitempty"`
	ColumnCount int      `json:"columnCount,omitempty"`
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Links []Link `json:"links"`
}

type PathHop struct {
	From        string `json:"from"`
	To          string `json:"to"`
	FromColumn  string `json:"fromColumn"`
	ToColumn    string `json:"toColumn"`
	Cardinality string `json:"cardinality"`
}

type JoinPath struct {
	Length int       `json:"length"`
	Tables []string  `json:"tables"`
	Hops   []PathHop `json:"hops"`
}

type LineageEntry struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Dataset     string   `json:"dataset,omitempty"`
	DomainID    string   `json:"domainId,omitempty"`
	Columns     []string `json:"columns"`
	ColumnCount int      `json:"columnCount"`
}
