/** Types mirroring the Go API's JSON shapes. */

/**
 * The role a table plays. Open by design, mirroring model.TableKind on the
 * backend: the built-in roles are listed for the sake of autocomplete, and a
 * role read straight from a document is just as valid. See graph/roles.ts for
 * how one gets drawn.
 */
export type KnownTableKind =
  | 'fact'
  | 'factless'
  | 'dimension'
  | 'outrigger'
  | 'bridge'
  | 'junk'
  | 'degenerate'
  | 'hub'
  | 'link'
  | 'satellite'
  | 'pit'
  | 'entity'
  | 'associative'
  | 'lookup'
  | 'reference'
  | 'unknown'

export type TableKind = KnownTableKind | (string & {})
export type Resolution = 'local' | 'conformed' | 'unresolved' | 'narrative'
export type Severity = 'error' | 'warning' | 'info'

export interface Stats {
  domains: number
  tables: number
  columns: number
  relationships: number
  lineageEdges: number
  sourceTables: number
  conformed: number
  filesParsed: number
  filesSkipped: number
  diagnostics: number
}

export interface Snapshot {
  id: string
  name: string
  sourceLabel: string
  createdAt: string
  stats: Stats
}

export interface DomainLineage {
  proposedTable: string
  sourceModels: string[]
}

export interface Domain {
  id: string
  name: string
  title: string
  description: string
  mermaid?: string
  lineage?: DomainLineage[]
  docPath: string
  tableCount: number
}

export interface Column {
  name: string
  type: string
  description: string
  ordinal: number
  isPk: boolean
  isFk: boolean
}

export interface ColumnLineage {
  column: string
  sourceTable: string
  sourceColumn: string
  notes: string
  derived: boolean
}

export interface Relationship {
  id: string
  fromTableId: string
  toTableId?: string
  targetRef: string
  fromColumn: string
  toColumn: string
  joinKeyRaw: string
  cardinality: string
  resolution: Resolution
  candidates?: string[]
}

export interface TableDetail {
  id: string
  name: string
  domainId: string
  kind: TableKind
  kindRaw: string
  grain: string
  updateFrequency: string
  layer: string
  domainLabel: string
  description: string
  columns: Column[]
  columnLineage: ColumnLineage[]
  relationships: Relationship[]
  notes: string[]
  relationshipNote?: string
  docPath: string
  conformed: boolean
  conformedIn?: string[]
}

export interface Referrer {
  tableId: string
  name: string
  domainId: string
  fromColumn: string
  toColumn: string
  cardinality: string
}

export interface LineageEntry {
  id: string
  label: string
  dataset?: string
  domainId?: string
  columns: string[]
  columnCount: number
}

export interface TableResponse {
  table: TableDetail
  incoming: Referrer[]
  upstream: LineageEntry[]
  siblings: LineageEntry[]
}

export interface TableSummary {
  id: string
  name: string
  domainId: string
  kind: TableKind
  grain: string
  conformed: boolean
  columnCount: number
  description: string
}

export interface GraphNode {
  id: string
  label: string
  type: 'table' | 'source'
  domainId?: string
  kind?: TableKind
  grain?: string
  conformed?: boolean
  columnCount?: number
  dataset?: string
  refs?: number
  degree: number
}

export interface GraphLink {
  id: string
  source: string
  target: string
  type: 'joins' | 'derived_from'
  fromColumn?: string
  toColumn?: string
  cardinality?: string
  resolution?: Resolution
  crossDomain?: boolean
  columns?: string[]
  columnCount?: number
}

export interface GraphData {
  nodes: GraphNode[]
  links: GraphLink[]
}

export interface PathHop {
  from: string
  to: string
  fromColumn: string
  toColumn: string
  cardinality: string
}

export interface JoinPath {
  length: number
  tables: string[]
  hops: PathHop[]
}

export interface Diagnostic {
  severity: Severity
  code: string
  message: string
  domainId?: string
  tableId?: string
  docPath?: string
}

export interface SearchHit {
  tableId: string
  name: string
  domainId: string
  kind: TableKind
  grain: string
  rank: number
  matchedOn?: string[]
}

export interface SourceTable {
  id: string
  dataset: string
  name: string
  refs: number
}

export interface IngestFile {
  path: string
  content: string
}

export interface IngestResult {
  snapshot: Snapshot
  edges: number
  diagnostics: Diagnostic[]
}
