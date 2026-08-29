// The shapes the API hands to the frontend. They are query results rather than
// stored records, which is why they live here and not in the model package.
package neo4j

// Node is a graph node as returned to the frontend.
type Node struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"` // table | source
	DomainID    string `json:"domainId,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Grain       string `json:"grain,omitempty"`
	Conformed   bool   `json:"conformed,omitempty"`
	ColumnCount int    `json:"columnCount,omitempty"`
	Dataset     string `json:"dataset,omitempty"`
	Refs        int    `json:"refs,omitempty"`
	// Degree is the number of joins touching this node, used to size nodes.
	Degree int `json:"degree"`
}

// Link is a graph edge as returned to the frontend.
type Link struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Target      string   `json:"target"`
	Type        string   `json:"type"` // joins | derived_from
	FromColumn  string   `json:"fromColumn,omitempty"`
	ToColumn    string   `json:"toColumn,omitempty"`
	Cardinality string   `json:"cardinality,omitempty"`
	Resolution  string   `json:"resolution,omitempty"`
	CrossDomain bool     `json:"crossDomain,omitempty"`
	Columns     []string `json:"columns,omitempty"`
	ColumnCount int      `json:"columnCount,omitempty"`
}

// Graph is a node-link projection.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Links []Link `json:"links"`
}

// GraphOptions filters what a graph query returns.
type GraphOptions struct {
	// Domains restricts to tables in these domains; empty means all.
	Domains []string
	// Kinds restricts to these table kinds; empty means all.
	Kinds []string
	// IncludeSources adds upstream source tables and DERIVED_FROM edges.
	IncludeSources bool
	// CrossDomainOnly keeps only joins that span two domains.
	CrossDomainOnly bool
}

// PathHop is one step of a join path.
type PathHop struct {
	From        string `json:"from"`
	To          string `json:"to"`
	FromColumn  string `json:"fromColumn"`
	ToColumn    string `json:"toColumn"`
	Cardinality string `json:"cardinality"`
}

// JoinPath is one way to get from table A to table B.
type JoinPath struct {
	Length int       `json:"length"`
	Tables []string  `json:"tables"`
	Hops   []PathHop `json:"hops"`
}

// LineageEntry is one upstream source feeding a table, or one downstream table
// fed by a source.
type LineageEntry struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Dataset     string   `json:"dataset,omitempty"`
	DomainID    string   `json:"domainId,omitempty"`
	Columns     []string `json:"columns"`
	ColumnCount int      `json:"columnCount"`
}
