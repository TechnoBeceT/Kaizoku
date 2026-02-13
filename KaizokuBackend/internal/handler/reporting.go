package handler

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/ent/sourceevent"
	"github.com/technobecet/kaizoku-go/internal/types"
)

// ReportingHandler provides endpoints for querying and aggregating source events.
type ReportingHandler struct {
	db *ent.Client
}

// parsePeriod converts a human-readable period string into a time.Duration.
// Supported formats: "24h", "7d", "30d". Default: 24h.
func parsePeriod(s string) time.Duration {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 24 * time.Hour
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	if strings.HasSuffix(s, "h") {
		hours, err := strconv.Atoi(strings.TrimSuffix(s, "h"))
		if err == nil && hours > 0 {
			return time.Duration(hours) * time.Hour
		}
	}
	return 24 * time.Hour
}

// sourceAgg is an internal accumulator for per-source aggregation.
type sourceAgg struct {
	sourceID         string
	sourceName       string
	language         string
	totalDuration    int64
	maxDuration      int64
	eventCount       int
	successCount     int
	failureCount     int
	partialCount     int
	lastEventAt      time.Time
	lastErrorAt      time.Time
	lastErrorMessage *string
	hasError         bool
	breakdown        map[string]*types.EventTypeBreakdown
}

// eventToDTO converts an Ent SourceEvent entity to a SourceEventDTO.
func eventToDTO(e *ent.SourceEvent) types.SourceEventDTO {
	meta := e.Metadata
	if meta == nil {
		meta = make(map[string]string)
	}
	return types.SourceEventDTO{
		ID:            e.ID.String(),
		SourceID:      e.SourceID,
		SourceName:    e.SourceName,
		Language:      e.Language,
		EventType:     e.EventType,
		Status:        e.Status,
		DurationMs:    e.DurationMs,
		ErrorMessage:  e.ErrorMessage,
		ErrorCategory: e.ErrorCategory,
		ItemsCount:    e.ItemsCount,
		Metadata:      meta,
		CreatedAt:     e.CreatedAt.Format(time.RFC3339),
	}
}

// accumulateEvent updates a sourceAgg with data from one event.
func accumulateEvent(agg *sourceAgg, e *ent.SourceEvent) {
	agg.eventCount++
	agg.totalDuration += e.DurationMs
	if e.DurationMs > agg.maxDuration {
		agg.maxDuration = e.DurationMs
	}
	switch e.Status {
	case "success":
		agg.successCount++
	case "failed":
		agg.failureCount++
	case "partial":
		agg.partialCount++
	}
	if e.CreatedAt.After(agg.lastEventAt) {
		agg.lastEventAt = e.CreatedAt
	}
	if e.Status == "failed" && e.CreatedAt.After(agg.lastErrorAt) {
		agg.lastErrorAt = e.CreatedAt
		agg.lastErrorMessage = e.ErrorMessage
		agg.hasError = true
	}
}

// newSourceAgg creates a new sourceAgg from an event.
func newSourceAgg(e *ent.SourceEvent) *sourceAgg {
	return &sourceAgg{
		sourceID:   e.SourceID,
		sourceName: e.SourceName,
		language:   e.Language,
		breakdown:  make(map[string]*types.EventTypeBreakdown),
	}
}

// toSummary converts a sourceAgg to a SourceStatsSummary.
func toSummary(agg *sourceAgg) types.SourceStatsSummary {
	var fr, avgDur float64
	if agg.eventCount > 0 {
		fr = math.Round(float64(agg.failureCount)/float64(agg.eventCount)*10000) / 100
		avgDur = math.Round(float64(agg.totalDuration)/float64(agg.eventCount)*100) / 100
	}
	return types.SourceStatsSummary{
		SourceID:      agg.sourceID,
		SourceName:    agg.sourceName,
		Language:      agg.language,
		AvgDurationMs: avgDur,
		EventCount:    agg.eventCount,
		FailureCount:  agg.failureCount,
		FailureRate:   fr,
	}
}

// GetOverview returns a dashboard summary of source events.
// GET /api/reporting/overview?period=24h
func (h *ReportingHandler) GetOverview(c echo.Context) error {
	ctx := c.Request().Context()
	period := parsePeriod(c.QueryParam("period"))
	cutoff := time.Now().Add(-period)

	events, err := h.db.SourceEvent.Query().
		Where(sourceevent.CreatedAtGTE(cutoff)).
		All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("reporting: failed to query events for overview")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to query events"})
	}

	// Aggregate globally and per-source.
	totalEvents := len(events)
	var successCount int
	var totalDuration int64
	sources := make(map[string]*sourceAgg)

	for _, e := range events {
		totalDuration += e.DurationMs
		if e.Status == "success" {
			successCount++
		}

		agg, ok := sources[e.SourceID]
		if !ok {
			agg = newSourceAgg(e)
			sources[e.SourceID] = agg
		}
		accumulateEvent(agg, e)
	}

	var successRate float64
	var avgDuration float64
	if totalEvents > 0 {
		successRate = math.Round(float64(successCount)/float64(totalEvents)*10000) / 100
		avgDuration = math.Round(float64(totalDuration)/float64(totalEvents)*100) / 100
	}

	// Build source summary list.
	summaries := make([]types.SourceStatsSummary, 0, len(sources))
	for _, agg := range sources {
		summaries = append(summaries, toSummary(agg))
	}

	// Slowest: top 10 by avg duration, minimum 5 events.
	slowest := make([]types.SourceStatsSummary, 0)
	for _, s := range summaries {
		if s.EventCount >= 5 {
			slowest = append(slowest, s)
		}
	}
	sort.Slice(slowest, func(i, j int) bool {
		return slowest[i].AvgDurationMs > slowest[j].AvgDurationMs
	})
	if len(slowest) > 10 {
		slowest = slowest[:10]
	}

	// Most failing: top 10 by failure count.
	failing := make([]types.SourceStatsSummary, 0, len(summaries))
	for _, s := range summaries {
		if s.FailureCount > 0 {
			failing = append(failing, s)
		}
	}
	sort.Slice(failing, func(i, j int) bool {
		return failing[i].FailureCount > failing[j].FailureCount
	})
	if len(failing) > 10 {
		failing = failing[:10]
	}

	// Recent errors: last 20 failed events.
	failedEvents, err := h.db.SourceEvent.Query().
		Where(
			sourceevent.CreatedAtGTE(cutoff),
			sourceevent.StatusEQ("failed"),
		).
		Order(ent.Desc(sourceevent.FieldCreatedAt)).
		Limit(20).
		All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("reporting: failed to query recent errors")
		failedEvents = nil
	}

	recentErrors := make([]types.SourceEventDTO, 0, len(failedEvents))
	for _, e := range failedEvents {
		recentErrors = append(recentErrors, eventToDTO(e))
	}

	return c.JSON(http.StatusOK, types.ReportingOverview{
		TotalEvents:    totalEvents,
		SuccessRate:    successRate,
		AvgDurationMs:  avgDuration,
		ActiveSources:  len(sources),
		SlowestSources: slowest,
		FailingSources: failing,
		RecentErrors:   recentErrors,
	})
}

// GetSources returns per-source aggregated stats.
// GET /api/reporting/sources?period=24h&sort=failures
func (h *ReportingHandler) GetSources(c echo.Context) error {
	ctx := c.Request().Context()
	period := parsePeriod(c.QueryParam("period"))
	sortBy := c.QueryParam("sort")
	cutoff := time.Now().Add(-period)

	events, err := h.db.SourceEvent.Query().
		Where(sourceevent.CreatedAtGTE(cutoff)).
		All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("reporting: failed to query events for sources")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to query events"})
	}

	sources := make(map[string]*sourceAgg)
	for _, e := range events {
		agg, ok := sources[e.SourceID]
		if !ok {
			agg = newSourceAgg(e)
			sources[e.SourceID] = agg
		}
		accumulateEvent(agg, e)

		// Event type breakdown.
		bd, ok := agg.breakdown[e.EventType]
		if !ok {
			bd = &types.EventTypeBreakdown{}
			agg.breakdown[e.EventType] = bd
		}
		bd.Total++
		if e.Status == "success" {
			bd.Success++
		} else if e.Status == "failed" {
			bd.Failed++
		}
	}

	result := make([]types.SourceStats, 0, len(sources))
	for _, agg := range sources {
		var sr float64
		var avgDur float64
		if agg.eventCount > 0 {
			sr = math.Round(float64(agg.successCount)/float64(agg.eventCount)*10000) / 100
			avgDur = math.Round(float64(agg.totalDuration)/float64(agg.eventCount)*100) / 100
		}

		var lastEventAt *string
		if !agg.lastEventAt.IsZero() {
			s := agg.lastEventAt.Format(time.RFC3339)
			lastEventAt = &s
		}
		var lastErrorAt *string
		if agg.hasError {
			s := agg.lastErrorAt.Format(time.RFC3339)
			lastErrorAt = &s
		}

		breakdown := make(map[string]types.EventTypeBreakdown, len(agg.breakdown))
		for k, v := range agg.breakdown {
			breakdown[k] = *v
		}

		result = append(result, types.SourceStats{
			SourceID:         agg.sourceID,
			SourceName:       agg.sourceName,
			Language:         agg.language,
			TotalEvents:      agg.eventCount,
			SuccessCount:     agg.successCount,
			FailureCount:     agg.failureCount,
			PartialCount:     agg.partialCount,
			SuccessRate:      sr,
			AvgDurationMs:    avgDur,
			MaxDurationMs:    agg.maxDuration,
			LastEventAt:      lastEventAt,
			LastErrorAt:      lastErrorAt,
			LastErrorMessage: agg.lastErrorMessage,
			Breakdown:        breakdown,
		})
	}

	// Sort results.
	switch sortBy {
	case "failures":
		sort.Slice(result, func(i, j int) bool {
			return result[i].FailureCount > result[j].FailureCount
		})
	case "duration":
		sort.Slice(result, func(i, j int) bool {
			return result[i].AvgDurationMs > result[j].AvgDurationMs
		})
	case "events":
		sort.Slice(result, func(i, j int) bool {
			return result[i].TotalEvents > result[j].TotalEvents
		})
	default:
		// Default: sort by source name ascending.
		sort.Slice(result, func(i, j int) bool {
			return result[i].SourceName < result[j].SourceName
		})
	}

	return c.JSON(http.StatusOK, result)
}

// GetSourceEvents returns paginated events for a specific source.
// GET /api/reporting/source/:sourceId/events?status=failed&eventType=download&limit=50&offset=0
func (h *ReportingHandler) GetSourceEvents(c echo.Context) error {
	ctx := c.Request().Context()
	sourceID := c.Param("sourceId")
	if sourceID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "sourceId required"})
	}

	statusFilter := c.QueryParam("status")
	eventTypeFilter := c.QueryParam("eventType")
	limitParam := c.QueryParam("limit")
	offsetParam := c.QueryParam("offset")

	limit := 50
	if limitParam != "" {
		if v, err := strconv.Atoi(limitParam); err == nil && v > 0 {
			limit = v
		}
	}

	offset := 0
	if offsetParam != "" {
		if v, err := strconv.Atoi(offsetParam); err == nil && v >= 0 {
			offset = v
		}
	}

	// Build query with filters. "__all__" means no source filter (global event log).
	query := h.db.SourceEvent.Query()
	if sourceID != "__all__" {
		query = query.Where(sourceevent.SourceIDEQ(sourceID))
	}

	if statusFilter != "" {
		query = query.Where(sourceevent.StatusEQ(statusFilter))
	}
	if eventTypeFilter != "" {
		query = query.Where(sourceevent.EventTypeEQ(eventTypeFilter))
	}

	// Get total count (same filters, no limit/offset).
	total, err := query.Clone().Count(ctx)
	if err != nil {
		log.Error().Err(err).Str("sourceId", sourceID).Msg("reporting: failed to count source events")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to count events"})
	}

	// Fetch paginated results.
	events, err := query.
		Order(ent.Desc(sourceevent.FieldCreatedAt)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		log.Error().Err(err).Str("sourceId", sourceID).Msg("reporting: failed to query source events")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to query events"})
	}

	dtos := make([]types.SourceEventDTO, 0, len(events))
	for _, e := range events {
		dtos = append(dtos, eventToDTO(e))
	}

	return c.JSON(http.StatusOK, types.SourceEventList{
		Total:  total,
		Events: dtos,
	})
}

// GetSourceTimeline returns time-bucketed aggregation for a specific source.
// GET /api/reporting/source/:sourceId/timeline?bucket=hour&period=24h
func (h *ReportingHandler) GetSourceTimeline(c echo.Context) error {
	ctx := c.Request().Context()
	sourceID := c.Param("sourceId")
	if sourceID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "sourceId required"})
	}

	bucket := c.QueryParam("bucket")
	if bucket == "" {
		bucket = "hour"
	}
	period := parsePeriod(c.QueryParam("period"))
	cutoff := time.Now().Add(-period)

	events, err := h.db.SourceEvent.Query().
		Where(
			sourceevent.SourceIDEQ(sourceID),
			sourceevent.CreatedAtGTE(cutoff),
		).
		Order(ent.Asc(sourceevent.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		log.Error().Err(err).Str("sourceId", sourceID).Msg("reporting: failed to query events for timeline")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to query events"})
	}

	// Bucket events by truncating timestamps.
	type bucketAgg struct {
		successCount  int
		failureCount  int
		totalDuration int64
		totalEvents   int
	}
	buckets := make(map[string]*bucketAgg)
	var bucketKeys []string

	for _, e := range events {
		var ts time.Time
		switch bucket {
		case "day":
			ts = time.Date(e.CreatedAt.Year(), e.CreatedAt.Month(), e.CreatedAt.Day(), 0, 0, 0, 0, e.CreatedAt.Location())
		default: // "hour"
			ts = e.CreatedAt.Truncate(time.Hour)
		}
		key := ts.Format(time.RFC3339)

		b, ok := buckets[key]
		if !ok {
			b = &bucketAgg{}
			buckets[key] = b
			bucketKeys = append(bucketKeys, key)
		}
		b.totalEvents++
		b.totalDuration += e.DurationMs
		switch e.Status {
		case "success":
			b.successCount++
		case "failed":
			b.failureCount++
		}
	}

	// Sort keys chronologically (already mostly in order from the DB query, but be safe).
	sort.Strings(bucketKeys)

	result := make([]types.TimelineBucket, 0, len(bucketKeys))
	for _, key := range bucketKeys {
		b := buckets[key]
		var avgDur float64
		if b.totalEvents > 0 {
			avgDur = math.Round(float64(b.totalDuration)/float64(b.totalEvents)*100) / 100
		}
		result = append(result, types.TimelineBucket{
			Timestamp:     key,
			SuccessCount:  b.successCount,
			FailureCount:  b.failureCount,
			AvgDurationMs: avgDur,
			TotalEvents:   b.totalEvents,
		})
	}

	return c.JSON(http.StatusOK, result)
}
