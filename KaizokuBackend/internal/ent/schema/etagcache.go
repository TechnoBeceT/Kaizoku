package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EtagCache holds the schema definition for the EtagCache entity.
type EtagCache struct {
	ent.Schema
}

func (EtagCache) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable().Comment("Cache key is the PK"),
		field.String("etag"),
		field.Time("last_updated").Default(time.Now),
	}
}

func (EtagCache) Edges() []ent.Edge {
	return nil
}

func (EtagCache) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("last_updated"),
	}
}
