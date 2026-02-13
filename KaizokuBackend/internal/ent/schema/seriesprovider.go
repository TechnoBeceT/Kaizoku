package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/technobecet/kaizoku-go/internal/types"
)

// SeriesProvider holds the schema definition for the SeriesProvider entity.
type SeriesProvider struct {
	ent.Schema
}

func (SeriesProvider) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("series_id", uuid.UUID{}),
		field.Int("suwayomi_id").Default(0),
		field.String("provider"),
		field.String("scanlator").Default(""),
		field.String("url").Optional().Nillable(),
		field.String("title").Default(""),
		field.String("language").Default(""),
		field.String("thumbnail_url").Optional().Nillable(),
		field.String("artist").Optional().Nillable(),
		field.String("author").Optional().Nillable(),
		field.String("description").Optional().Nillable(),
		field.JSON("genre", []string{}).Optional(),
		field.Time("fetch_date").Optional().Nillable(),
		field.Int64("chapter_count").Optional().Nillable(),
		field.Float("continue_after_chapter").Optional().Nillable(),
		field.Bool("is_title").Default(false),
		field.Bool("is_cover").Default(false),
		field.Bool("is_unknown").Default(false),
		field.Int("importance").Default(0),
		field.Bool("is_disabled").Default(false),
		field.Bool("is_uninstalled").Default(false),
		field.String("status").Default("UNKNOWN"),
		field.JSON("chapters", []types.Chapter{}).Optional(),
	}
}

func (SeriesProvider) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("series", Series.Type).
			Ref("providers").
			Field("series_id").
			Unique().
			Required(),
	}
}

func (SeriesProvider) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("series_id"),
		index.Fields("suwayomi_id"),
		index.Fields("title", "language"),
		index.Fields("provider", "language", "scanlator"),
	}
}
