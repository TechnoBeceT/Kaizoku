package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// SourceEvent logs individual source interactions for performance reporting.
// Each row represents one operation (download, chapter fetch, latest fetch, search, etc.)
// with timing, status, and error details.
type SourceEvent struct {
	ent.Schema
}

func (SourceEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("source_id").Comment("Suwayomi source ID or provider identifier"),
		field.String("source_name").Comment("Human-readable source display name"),
		field.String("language").Comment("Source language code (e.g. en, ko)"),
		field.String("event_type").Comment("get_latest, get_chapters, download, search, get_popular"),
		field.String("status").Comment("success, failed, partial"),
		field.Int64("duration_ms").Comment("Operation wall-clock time in milliseconds"),
		field.String("error_message").Optional().Nillable().Comment("Error details on failure"),
		field.String("error_category").Optional().Nillable().Comment("network, timeout, rate_limit, server_error, not_found, parse, cancelled, unknown"),
		field.Int("items_count").Optional().Nillable().Comment("Number of items processed (pages, chapters, manga)"),
		field.JSON("metadata", map[string]string{}).Optional().Comment("Extra context (series title, chapter number, etc.)"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (SourceEvent) Edges() []ent.Edge {
	return nil
}

func (SourceEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_id", "created_at"),
		index.Fields("event_type", "created_at"),
		index.Fields("status", "created_at"),
		index.Fields("created_at"),
	}
}
