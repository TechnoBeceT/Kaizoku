package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/technobecet/kaizoku-go/internal/types"
)

// LatestSeries holds the schema definition for the LatestSeries entity.
type LatestSeries struct {
	ent.Schema
}

func (LatestSeries) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Unique().Immutable().Comment("SuwayomiId is the PK"),
		field.String("suwayomi_source_id"),
		field.String("provider"),
		field.String("language").Default(""),
		field.String("url").Optional().Nillable(),
		field.String("title").Default(""),
		field.String("thumbnail_url").Optional().Nillable(),
		field.String("artist").Optional().Nillable(),
		field.String("author").Optional().Nillable(),
		field.String("description").Optional().Nillable(),
		field.JSON("genre", []string{}).Optional(),
		field.Time("fetch_date"),
		field.Int64("chapter_count").Optional().Nillable(),
		field.Float("latest_chapter").Optional().Nillable(),
		field.String("latest_chapter_title").Default(""),
		field.String("status").Default("UNKNOWN"),
		field.Int("in_library").Default(0),
		field.UUID("series_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("chapters", []types.SuwayomiChapter{}).Optional(),
	}
}

func (LatestSeries) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("series", Series.Type).
			Ref("latest_series").
			Field("series_id").
			Unique(),
	}
}

func (LatestSeries) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("suwayomi_source_id"),
		index.Fields("fetch_date"),
		index.Fields("title"),
	}
}
