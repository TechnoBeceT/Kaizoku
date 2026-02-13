package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/technobecet/kaizoku-go/internal/types"
)

// ImportEntry holds the schema definition for the ImportEntry entity.
// Named ImportEntry to avoid Go keyword conflict with "import".
type ImportEntry struct {
	ent.Schema
}

func (ImportEntry) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable().Comment("Path is the PK"),
		field.String("title"),
		field.Int("status").Default(int(types.ImportStatusImport)),
		field.Int("action").Default(int(types.ImportActionAdd)),
		field.JSON("info", &types.KaizokuInfo{}).Optional(),
		field.JSON("series", []map[string]interface{}{}).Optional(),
		field.Float("continue_after_chapter").Optional().Nillable(),
	}
}

func (ImportEntry) Edges() []ent.Edge {
	return nil
}

func (ImportEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "action"),
	}
}
