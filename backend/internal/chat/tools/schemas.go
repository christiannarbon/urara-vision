package tools

// Python's pydantic schemas without the title keys. Nullable optionals are
// plain optional properties: an omitted key already means "none".

const noArgsSchema = `{"type": "object", "properties": {}, "additionalProperties": false}`

const listTablesSchema = `{
	"type": "object",
	"properties": {
		"domain": {
			"type": "string",
			"description": "Optional domain ID to restrict the list to, e.g. 'ordering'. Omit to list every table in the model."
		}
	},
	"additionalProperties": false
}`

const getTablesSchema = `{
	"type": "object",
	"properties": {
		"ids": {
			"type": "array",
			"items": {"type": "string"},
			"minItems": 1,
			"maxItems": 8,
			"description": "Between one and eight full table IDs in 'domain/table' form, e.g. ['ordering/fact_orders']. Use search_model or list_tables first if you only have a name."
		}
	},
	"required": ["ids"],
	"additionalProperties": false
}`

const searchSchema = `{
	"type": "object",
	"properties": {
		"query": {
			"type": "string",
			"description": "Words to search for in table and column names and prose."
		},
		"limit": {
			"type": "integer",
			"minimum": 1,
			"maximum": 50,
			"default": 20,
			"description": "Maximum hits to return, 1 to 50."
		}
	},
	"required": ["query"],
	"additionalProperties": false
}`

const neighbourhoodSchema = `{
	"type": "object",
	"properties": {
		"table_id": {
			"type": "string",
			"description": "Full table ID in 'domain/table' form."
		},
		"depth": {
			"type": "integer",
			"minimum": 1,
			"maximum": 3,
			"default": 1,
			"description": "How many joins to follow out, 1 to 3."
		}
	},
	"required": ["table_id"],
	"additionalProperties": false
}`

const joinPathsSchema = `{
	"type": "object",
	"properties": {
		"from_table": {
			"type": "string",
			"description": "Full table ID to start from, in 'domain/table' form."
		},
		"to_table": {
			"type": "string",
			"description": "Full table ID to reach, in 'domain/table' form."
		},
		"max_depth": {
			"type": "integer",
			"minimum": 1,
			"maximum": 6,
			"default": 4,
			"description": "Longest path to consider, 1 to 6."
		}
	},
	"required": ["from_table", "to_table"],
	"additionalProperties": false
}`

const lineageSchema = `{
	"type": "object",
	"properties": {
		"table_id": {
			"type": "string",
			"description": "Full table ID in 'domain/table' form."
		},
		"direction": {
			"type": "string",
			"enum": ["upstream", "downstream"],
			"default": "upstream",
			"description": "'upstream' for the sources feeding this table, 'downstream' for what is fed by it."
		}
	},
	"required": ["table_id"],
	"additionalProperties": false
}`

const diagnosticsSchema = `{
	"type": "object",
	"properties": {
		"severity": {
			"type": "string",
			"enum": ["error", "warning", "info"],
			"description": "Restrict to one severity. Omit for all of them."
		}
	},
	"additionalProperties": false
}`
