package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/technobecet/kaizoku-go/internal/types"
)

// DownloadQueueItem holds the schema definition for a download queue entry.
// This replaces River for download jobs, providing strict FIFO ordering
// and per-provider concurrency control via a custom dispatcher.
type DownloadQueueItem struct {
	ent.Schema
}

func (DownloadQueueItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("group_key").Comment("Provider name for per-provider concurrency limiting"),
		field.Int("status").Default(types.DLStatusWaiting).Comment("0=waiting, 1=running, 2=completed, 3=failed"),
		field.Int("priority").Default(0).Comment("Lower = higher priority (chapter number used as priority)"),
		field.Time("scheduled_at").Default(time.Now),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("started_at").Optional().Nillable(),
		field.Time("completed_at").Optional().Nillable(),
		field.JSON("args", types.DownloadChapterArgs{}),
	}
}

func (DownloadQueueItem) Edges() []ent.Edge {
	return nil
}

func (DownloadQueueItem) Indexes() []ent.Index {
	return []ent.Index{
		// Primary dispatch query: waiting items eligible for execution
		index.Fields("status", "scheduled_at"),
		// Per-provider running count
		index.Fields("status", "group_key"),
	}
}
