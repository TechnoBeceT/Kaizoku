package job

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/ent/downloadqueueitem"
	"github.com/technobecet/kaizoku-go/internal/ent/seriesprovider"
	"github.com/technobecet/kaizoku-go/internal/types"
)

// DownloadDispatcher replaces River for download jobs.
// It polls the download_queue_items table and dispatches downloads with
// strict FIFO ordering and per-provider concurrency control.
type DownloadDispatcher struct {
	db       *ent.Client
	deps     *Deps
	maxTotal int // max concurrent downloads globally (config fallback)
	maxGroup int // max concurrent downloads per provider (config fallback)

	mu      sync.Mutex
	running map[string]int // group_key -> count of running downloads
	total   int            // total running count
	wg      sync.WaitGroup
}

// NewDownloadDispatcher creates a new download dispatcher.
func NewDownloadDispatcher(db *ent.Client, deps *Deps, maxTotal, maxGroup int) *DownloadDispatcher {
	if maxTotal <= 0 {
		maxTotal = 10
	}
	if maxGroup <= 0 {
		maxGroup = 3
	}
	return &DownloadDispatcher{
		db:       db,
		deps:     deps,
		maxTotal: maxTotal,
		maxGroup: maxGroup,
		running:  make(map[string]int),
	}
}

// getLimits returns the current maxTotal and maxGroup, preferring DB settings over config defaults.
func (d *DownloadDispatcher) getLimits(ctx context.Context) (maxTotal, maxGroup int) {
	maxTotal = d.maxTotal
	maxGroup = d.maxGroup
	if d.deps != nil && d.deps.Settings != nil {
		if s, err := d.deps.Settings.Get(ctx); err == nil && s != nil {
			if s.NumberOfSimultaneousDownloads > 0 {
				maxTotal = s.NumberOfSimultaneousDownloads
			}
			if s.NumberOfSimultaneousDownloadsPerProvider > 0 {
				maxGroup = s.NumberOfSimultaneousDownloadsPerProvider
			}
		}
	}
	return
}

// Run starts the dispatch loop. Blocks until ctx is cancelled.
func (d *DownloadDispatcher) Run(ctx context.Context) {
	// Reset any "running" items from a previous crash back to "waiting"
	d.resetStaleRunning(ctx)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("download dispatcher shutting down, waiting for running downloads...")
			d.wg.Wait()
			log.Info().Msg("download dispatcher stopped")
			return
		case <-ticker.C:
			d.dispatch(ctx)
		}
	}
}

// Stop waits for all running downloads to complete.
func (d *DownloadDispatcher) Stop() {
	d.wg.Wait()
}

// Enqueue adds a download job to the queue.
func (d *DownloadDispatcher) Enqueue(ctx context.Context, args types.DownloadChapterArgs, scheduledAt time.Time) error {
	priority := 0
	if args.ChapterNumber != nil {
		// Use chapter number * 100 as priority (lower = higher priority).
		// Multiplied to handle decimal chapters like 1.5.
		priority = int(*args.ChapterNumber * 100)
	}

	_, err := d.db.DownloadQueueItem.Create().
		SetGroupKey(args.ProviderName).
		SetStatus(types.DLStatusWaiting).
		SetPriority(priority).
		SetScheduledAt(scheduledAt).
		SetArgs(args).
		Save(ctx)
	if err != nil {
		return err
	}
	return nil
}

// resetStaleRunning resets any items stuck in "running" state from a previous crash.
func (d *DownloadDispatcher) resetStaleRunning(ctx context.Context) {
	n, err := d.db.DownloadQueueItem.Update().
		Where(downloadqueueitem.StatusEQ(types.DLStatusRunning)).
		SetStatus(types.DLStatusWaiting).
		ClearStartedAt().
		Save(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to reset stale running downloads")
	} else if n > 0 {
		log.Info().Int("count", n).Msg("reset stale running downloads to waiting")
	}
}

// dispatch is the core polling function called every 500ms.
func (d *DownloadDispatcher) dispatch(ctx context.Context) {
	maxTotal, maxGroup := d.getLimits(ctx)

	d.mu.Lock()
	available := maxTotal - d.total
	if available <= 0 {
		d.mu.Unlock()
		return
	}
	// Snapshot running counts
	runningSnapshot := make(map[string]int, len(d.running))
	for k, v := range d.running {
		runningSnapshot[k] = v
	}
	d.mu.Unlock()

	// Get distinct group keys with eligible items to prevent source starvation.
	// A simple global Limit query would starve sources with higher chapter numbers.
	groupKeys, err := d.db.DownloadQueueItem.Query().
		Where(
			downloadqueueitem.StatusEQ(types.DLStatusWaiting),
			downloadqueueitem.ScheduledAtLTE(time.Now()),
		).
		Unique(true).
		Select(downloadqueueitem.FieldGroupKey).
		Strings(ctx)
	if err != nil || len(groupKeys) == 0 {
		return
	}

	// Fetch top items per group, respecting per-group running limits
	grouped := make(map[string][]*ent.DownloadQueueItem)
	var groupOrder []string
	for _, gk := range groupKeys {
		slotsLeft := maxGroup - runningSnapshot[gk]
		if slotsLeft <= 0 {
			continue
		}

		items, err := d.db.DownloadQueueItem.Query().
			Where(
				downloadqueueitem.StatusEQ(types.DLStatusWaiting),
				downloadqueueitem.ScheduledAtLTE(time.Now()),
				downloadqueueitem.GroupKeyEQ(gk),
			).
			Order(
				ent.Asc(downloadqueueitem.FieldPriority),
				ent.Asc(downloadqueueitem.FieldScheduledAt),
				ent.Asc(downloadqueueitem.FieldID),
			).
			Limit(slotsLeft).
			All(ctx)
		if err != nil || len(items) == 0 {
			continue
		}

		grouped[gk] = items
		groupOrder = append(groupOrder, gk)
	}

	if len(groupOrder) == 0 {
		return
	}

	// Fair-share round-robin: take 1 from each group in turn,
	// respecting per-group limits, until we fill available slots.
	var toStart []*ent.DownloadQueueItem
	started := 0
	for started < available {
		pickedAny := false
		for _, groupKey := range groupOrder {
			if started >= available {
				break
			}
			jobs := grouped[groupKey]
			if len(jobs) == 0 {
				continue
			}

			currentRunning := runningSnapshot[groupKey]
			if currentRunning >= maxGroup {
				continue
			}

			// Take the first (highest priority) job from this group
			item := jobs[0]
			grouped[groupKey] = jobs[1:]

			toStart = append(toStart, item)
			runningSnapshot[groupKey]++
			started++
			pickedAny = true
		}
		if !pickedAny {
			break // No group has eligible items
		}
	}

	// Start the selected downloads
	for _, item := range toStart {
		d.startDownload(ctx, item)
	}
}

// startDownload marks an item as running and launches a goroutine for the download.
func (d *DownloadDispatcher) startDownload(ctx context.Context, item *ent.DownloadQueueItem) {
	now := time.Now()

	// Mark as running in DB
	_, err := d.db.DownloadQueueItem.UpdateOneID(item.ID).
		SetStatus(types.DLStatusRunning).
		SetStartedAt(now).
		Save(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to mark download as running")
		return
	}

	d.mu.Lock()
	d.running[item.GroupKey]++
	d.total++
	d.mu.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			d.running[item.GroupKey]--
			if d.running[item.GroupKey] <= 0 {
				delete(d.running, item.GroupKey)
			}
			d.total--
			d.mu.Unlock()
		}()

		// Use a fresh context (not the dispatch ticker context) so downloads
		// can complete even during graceful shutdown.
		dlCtx := context.Background()
		d.executeDownload(dlCtx, item.ID, item.Args)
	}()
}

// executeDownload runs the actual download logic for a queue item.
func (d *DownloadDispatcher) executeDownload(ctx context.Context, itemID uuid.UUID, args types.DownloadChapterArgs) {
	chapStr := "?"
	if args.ChapterNumber != nil {
		chapStr = formatChapterNumber(*args.ChapterNumber)
	}

	// Validate state before downloading — series may have been paused, provider disabled/deleted.
	if err := d.validateDownloadState(ctx, args); err != nil {
		log.Info().Err(err).
			Str("title", args.Title).
			Str("provider", args.ProviderName).
			Str("chapter", chapStr).
			Msg("download skipped (state changed)")

		// Mark as failed so it doesn't retry automatically
		d.db.DownloadQueueItem.UpdateOneID(itemID).
			SetStatus(types.DLStatusFailed).
			SetCompletedAt(time.Now()).
			Save(ctx)
		return
	}

	cbzFilename, err := d.deps.performDownload(ctx, args, chapStr, itemID.String())
	if err != nil {
		log.Warn().Err(err).
			Str("title", args.Title).
			Str("provider", args.ProviderName).
			Str("chapter", chapStr).
			Msg("chapter download failed")

		// Mark as failed initially
		d.db.DownloadQueueItem.UpdateOneID(itemID).
			SetStatus(types.DLStatusFailed).
			SetCompletedAt(time.Now()).
			Save(ctx)

		// Set the original item ID so cascade handlers can clean up
		args.OriginalItemID = itemID

		var retryScheduled bool
		if args.IsReplacement {
			retryScheduled = d.deps.handleReplacementFailure(ctx, args)
		} else {
			retryScheduled = d.deps.cascadeOnFailure(ctx, args)
		}

		// If a retry/fallback was enqueued, remove the old failed item
		// so it doesn't show in Error Downloads prematurely
		if retryScheduled {
			d.db.DownloadQueueItem.DeleteOneID(itemID).Exec(ctx)
		}
		return
	}

	// Mark as completed
	d.db.DownloadQueueItem.UpdateOneID(itemID).
		SetStatus(types.DLStatusCompleted).
		SetCompletedAt(time.Now()).
		Save(ctx)

	log.Info().Str("file", cbzFilename).Str("title", args.Title).Msg("chapter download complete")

	if args.IsReplacement {
		d.deps.handleReplacementSuccess(ctx, args)
	} else {
		d.deps.handleDownloadSuccess(ctx, args, cbzFilename)
	}
}

// formatChapterNumber formats a float64 chapter number as a string.
func formatChapterNumber(n float64) string {
	if n == float64(int(n)) {
		return fmtNum(n)
	}
	return fmtNum(n)
}

// GetMetrics returns current download queue counts.
func (d *DownloadDispatcher) GetMetrics(ctx context.Context) types.DownloadsMetrics {
	metrics := types.DownloadsMetrics{}

	running, _ := d.db.DownloadQueueItem.Query().
		Where(downloadqueueitem.StatusEQ(types.DLStatusRunning)).
		Count(ctx)
	metrics.Downloads = running

	queued, _ := d.db.DownloadQueueItem.Query().
		Where(downloadqueueitem.StatusEQ(types.DLStatusWaiting)).
		Count(ctx)
	metrics.Queued = queued

	failed, _ := d.db.DownloadQueueItem.Query().
		Where(downloadqueueitem.StatusEQ(types.DLStatusFailed)).
		Count(ctx)
	metrics.Failed = failed

	return metrics
}

// GetDownloads returns download items filtered by status with pagination.
func (d *DownloadDispatcher) GetDownloads(ctx context.Context, status *int, limit int, offset int, keyword string) types.DownloadInfoList {
	// First get total count for the filter
	countQuery := d.db.DownloadQueueItem.Query()
	if status != nil {
		countQuery = countQuery.Where(downloadqueueitem.StatusEQ(*status))
	}
	totalCount, _ := countQuery.Count(ctx)

	// Then get paginated results
	query := d.db.DownloadQueueItem.Query()

	if status != nil {
		query = query.Where(downloadqueueitem.StatusEQ(*status))
	}

	// Order depends on status filter:
	// Completed/Failed → most recent first (completed_at desc)
	// Running/Waiting/mixed → chapter order (priority asc, scheduled_at asc)
	if status != nil && (*status == types.DLStatusCompleted || *status == types.DLStatusFailed) {
		query = query.Order(
			ent.Desc(downloadqueueitem.FieldCompletedAt),
			ent.Asc(downloadqueueitem.FieldPriority),
		)
	} else {
		query = query.Order(
			ent.Asc(downloadqueueitem.FieldStatus),
			ent.Asc(downloadqueueitem.FieldPriority),
			ent.Asc(downloadqueueitem.FieldScheduledAt),
		)
	}

	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	items, err := query.All(ctx)
	if err != nil {
		return types.DownloadInfoList{Downloads: []types.DownloadInfo{}}
	}

	downloads := make([]types.DownloadInfo, 0, len(items))
	for _, item := range items {
		info := queueItemToDownloadInfo(item)
		if keyword != "" && !matchesKeyword(info, keyword) {
			continue
		}
		downloads = append(downloads, info)
	}

	return types.DownloadInfoList{
		TotalCount: totalCount,
		Downloads:  downloads,
	}
}

// GetSeriesDownloads returns downloads for a specific series.
func (d *DownloadDispatcher) GetSeriesDownloads(ctx context.Context, seriesID string) []types.DownloadInfo {
	items, err := d.db.DownloadQueueItem.Query().
		Order(
			ent.Asc(downloadqueueitem.FieldStatus),
			ent.Asc(downloadqueueitem.FieldPriority),
		).
		All(ctx)
	if err != nil {
		return []types.DownloadInfo{}
	}

	var downloads []types.DownloadInfo
	for _, item := range items {
		if item.Args.SeriesID.String() == seriesID {
			downloads = append(downloads, queueItemToDownloadInfo(item))
		}
	}
	if downloads == nil {
		downloads = []types.DownloadInfo{}
	}
	return downloads
}

// RetryDownload re-queues a failed download.
func (d *DownloadDispatcher) RetryDownload(ctx context.Context, id uuid.UUID) error {
	_, err := d.db.DownloadQueueItem.UpdateOneID(id).
		SetStatus(types.DLStatusWaiting).
		SetScheduledAt(time.Now()).
		ClearStartedAt().
		ClearCompletedAt().
		Save(ctx)
	return err
}

// DeleteDownload removes a download from the queue.
func (d *DownloadDispatcher) DeleteDownload(ctx context.Context, id uuid.UUID) error {
	return d.db.DownloadQueueItem.DeleteOneID(id).Exec(ctx)
}

// CancelSeriesDownloads deletes all waiting downloads for a given series.
func (d *DownloadDispatcher) CancelSeriesDownloads(ctx context.Context, seriesID uuid.UUID) (int, error) {
	items, err := d.db.DownloadQueueItem.Query().
		Where(downloadqueueitem.StatusEQ(types.DLStatusWaiting)).
		All(ctx)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, item := range items {
		if item.Args.SeriesID == seriesID {
			if err := d.db.DownloadQueueItem.DeleteOneID(item.ID).Exec(ctx); err == nil {
				deleted++
			}
		}
	}
	return deleted, nil
}

// queueItemToDownloadInfo converts an Ent entity to the API DTO.
func queueItemToDownloadInfo(item *ent.DownloadQueueItem) types.DownloadInfo {
	args := item.Args
	scheduledAt := item.ScheduledAt.UTC().Format("2006-01-02T15:04:05Z")

	status := types.QueueStatusWaiting
	switch item.Status {
	case types.DLStatusRunning:
		status = types.QueueStatusRunning
	case types.DLStatusCompleted:
		status = types.QueueStatusCompleted
	case types.DLStatusFailed:
		status = types.QueueStatusFailed
	}

	info := types.DownloadInfo{
		ID:               item.ID.String(),
		Title:            args.Title,
		Provider:         args.ProviderName,
		Language:         args.Language,
		Chapter:          args.ChapterNumber,
		Status:           status,
		ScheduledDateUTC: scheduledAt,
		Retries:          args.CascadeRetries,
	}

	if args.Scanlator != "" {
		info.Scanlator = &args.Scanlator
	}
	if args.ChapterName != "" {
		info.ChapterTitle = &args.ChapterName
	}
	if args.ThumbnailURL != "" {
		info.ThumbnailURL = &args.ThumbnailURL
	}
	if args.URL != "" {
		info.URL = &args.URL
	}
	if item.CompletedAt != nil {
		t := item.CompletedAt.UTC().Format("2006-01-02T15:04:05Z")
		info.DownloadDateUTC = &t
	}

	return info
}

func matchesKeyword(info types.DownloadInfo, keyword string) bool {
	kw := strings.ToLower(keyword)
	return strings.Contains(strings.ToLower(info.Title), kw) ||
		strings.Contains(strings.ToLower(info.Provider), kw) ||
		strings.Contains(strings.ToLower(info.Language), kw) ||
		(info.ChapterTitle != nil && strings.Contains(strings.ToLower(*info.ChapterTitle), kw))
}

// validateDownloadState checks if the series/provider are still in a downloadable state.
// Returns an error if the download should be skipped.
func (d *DownloadDispatcher) validateDownloadState(ctx context.Context, args types.DownloadChapterArgs) error {
	// Check if series still exists and isn't paused
	s, err := d.db.Series.Get(ctx, args.SeriesID)
	if err != nil {
		return fmt.Errorf("series no longer exists")
	}
	if s.PauseDownloads {
		return fmt.Errorf("series downloads are paused")
	}

	// Check if provider still exists and isn't disabled/deleted
	sp, err := d.db.SeriesProvider.Get(ctx, args.ProviderID)
	if err != nil {
		return fmt.Errorf("provider no longer exists")
	}
	if sp.IsDisabled {
		return fmt.Errorf("provider is disabled")
	}
	if sp.IsUninstalled {
		return fmt.Errorf("provider is uninstalled")
	}
	if sp.IsUnknown {
		return fmt.Errorf("provider was converted to unknown (deleted)")
	}

	return nil
}

// CancelProviderDownloads deletes all waiting downloads for a given provider.
func (d *DownloadDispatcher) CancelProviderDownloads(ctx context.Context, providerID uuid.UUID) (int, error) {
	items, err := d.db.DownloadQueueItem.Query().
		Where(downloadqueueitem.StatusEQ(types.DLStatusWaiting)).
		All(ctx)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, item := range items {
		if item.Args.ProviderID == providerID {
			if err := d.db.DownloadQueueItem.DeleteOneID(item.ID).Exec(ctx); err == nil {
				deleted++
			}
		}
	}
	return deleted, nil
}

// CancelProviderDownloadsByName deletes all waiting downloads matching a provider name for a series.
func (d *DownloadDispatcher) CancelProviderDownloadsByName(ctx context.Context, seriesID uuid.UUID, providerName string) (int, error) {
	items, err := d.db.DownloadQueueItem.Query().
		Where(downloadqueueitem.StatusEQ(types.DLStatusWaiting)).
		All(ctx)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, item := range items {
		if item.Args.SeriesID == seriesID && item.Args.ProviderName == providerName {
			if err := d.db.DownloadQueueItem.DeleteOneID(item.ID).Exec(ctx); err == nil {
				deleted++
			}
		}
	}
	return deleted, nil
}

// PauseSeriesDownloads cancels all waiting downloads for a series (used when pause is toggled on).
func (d *DownloadDispatcher) PauseSeriesDownloads(ctx context.Context, seriesID uuid.UUID) (int, error) {
	return d.CancelSeriesDownloads(ctx, seriesID)
}

// CancelDisabledProviderDownloads cancels waiting downloads for all disabled providers of a series.
func (d *DownloadDispatcher) CancelDisabledProviderDownloads(ctx context.Context, seriesID uuid.UUID) (int, error) {
	// Get all disabled providers for this series
	disabledProviders, err := d.db.SeriesProvider.Query().
		Where(
			seriesprovider.SeriesIDEQ(seriesID),
			seriesprovider.IsDisabledEQ(true),
		).
		All(ctx)
	if err != nil {
		return 0, err
	}

	providerIDs := make(map[uuid.UUID]bool)
	for _, p := range disabledProviders {
		providerIDs[p.ID] = true
	}

	if len(providerIDs) == 0 {
		return 0, nil
	}

	items, err := d.db.DownloadQueueItem.Query().
		Where(downloadqueueitem.StatusEQ(types.DLStatusWaiting)).
		All(ctx)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, item := range items {
		if item.Args.SeriesID == seriesID && providerIDs[item.Args.ProviderID] {
			if err := d.db.DownloadQueueItem.DeleteOneID(item.ID).Exec(ctx); err == nil {
				deleted++
			}
		}
	}
	return deleted, nil
}
