package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Series holds the schema definition for the Series entity.
type Series struct {
	ent.Schema
}

func (Series) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("title"),
		field.String("thumbnail_url").Default(""),
		field.String("artist").Default(""),
		field.String("author").Default(""),
		field.String("description").Default(""),
		field.JSON("genre", []string{}).Optional(),
		field.String("status").Default("UNKNOWN"),
		field.String("storage_path").Optional(),
		field.String("type").Optional().Nillable(),
		field.Int("chapter_count").Default(0),
		field.Bool("pause_downloads").Default(false),
	}
}

func (Series) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("providers", SeriesProvider.Type),
		edge.To("latest_series", LatestSeries.Type),
	}
}
