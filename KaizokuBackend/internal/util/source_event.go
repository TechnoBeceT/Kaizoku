package util

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/ent"
)

// EventOption configures optional fields on a source event.
type EventOption func(c *ent.SourceEventCreate)

// WithError sets the error message and auto-categorizes it.
func WithError(err error) EventOption {
	return func(c *ent.SourceEventCreate) {
		if err == nil {
			return
		}
		msg := err.Error()
		c.SetErrorMessage(msg)
		c.SetErrorCategory(CategorizeError(err))
	}
}

// WithErrorCategory sets an explicit error category (overrides auto-categorization).
func WithErrorCategory(category string) EventOption {
	return func(c *ent.SourceEventCreate) {
		c.SetErrorCategory(category)
	}
}

// WithItemsCount sets the number of items processed.
func WithItemsCount(count int) EventOption {
	return func(c *ent.SourceEventCreate) {
		c.SetItemsCount(count)
	}
}

// WithMetadata sets extra context for the event.
func WithMetadata(meta map[string]string) EventOption {
	return func(c *ent.SourceEventCreate) {
		c.SetMetadata(meta)
	}
}

// LogSourceEvent records a source interaction event asynchronously.
// It fires-and-forgets to avoid blocking the calling worker or handler.
func LogSourceEvent(
	db *ent.Client,
	sourceID, sourceName, language string,
	eventType, status string,
	durationMs int64,
	opts ...EventOption,
) {
	go func() {
		create := db.SourceEvent.Create().
			SetSourceID(sourceID).
			SetSourceName(sourceName).
			SetLanguage(language).
			SetEventType(eventType).
			SetStatus(status).
			SetDurationMs(durationMs)

		for _, opt := range opts {
			opt(create)
		}

		if err := create.Exec(context.Background()); err != nil {
			log.Warn().Err(err).
				Str("sourceId", sourceID).
				Str("eventType", eventType).
				Msg("failed to log source event")
		}
	}()
}
