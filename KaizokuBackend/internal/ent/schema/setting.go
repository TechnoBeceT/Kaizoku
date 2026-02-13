package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Setting holds the schema definition for the Setting entity.
type Setting struct {
	ent.Schema
}

func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable().Comment("Setting name is the PK"),
		field.String("value"),
	}
}

func (Setting) Edges() []ent.Edge {
	return nil
}
