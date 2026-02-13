package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/ent/importentry"
	"github.com/technobecet/kaizoku-go/internal/ent/latestseries"
	"github.com/technobecet/kaizoku-go/internal/ent/seriesprovider"
	"github.com/technobecet/kaizoku-go/internal/ent/sourceevent"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
	"github.com/technobecet/kaizoku-go/internal/types"
	"github.com/technobecet/kaizoku-go/internal/util"
)

// ProgressBroadcaster is the interface for broadcasting progress updates.
type ProgressBroadcaster interface {
	BroadcastProgress(id string, jobType int, status int, percentage float64, message string, param interface{})
}

// Deps holds shared dependencies injected into job workers.
type Deps struct {
	DB            *ent.Client
	Suwayomi      *suwayomi.Client
	Progress      ProgressBroadcaster
	Config        *config.Config
	DownloadQueue *DownloadDispatcher // Custom download queue (replaces River for downloads)
	RiverClient   RiverInserter       // For enqueuing River jobs from shared functions
}

// RiverInserter is the interface for inserting River jobs (allows both handler and worker context).
type RiverInserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// ============================================================
// Download execution logic — called by DownloadDispatcher
// ============================================================

// performDownload fetches pages from Suwayomi, creates a CBZ, and updates the database.
// Returns the CBZ filename on success.
func (d *Deps) performDownload(ctx context.Context, args types.DownloadChapterArgs, chapStr string, itemID string) (string, error) {
	jobID := fmt.Sprintf("dl-%s", itemID)
	dlStart := time.Now()

	log.Info().
		Str("title", args.Title).
		Str("provider", args.ProviderName).
		Str("chapter", chapStr).
		Msg("downloading chapter")

	// Build DownloadCardInfo for progress broadcasting (frontend needs this to display active downloads).
	cardInfo := map[string]interface{}{
		"pageCount":     args.PageCount,
		"provider":      args.ProviderName,
		"language":      args.Language,
		"title":         args.Title,
		"chapterName":   args.ChapterName,
		"thumbnailUrl":  args.ThumbnailURL,
	}
	if args.Scanlator != "" {
		cardInfo["scanlator"] = args.Scanlator
	}
	if args.ChapterNumber != nil {
		cardInfo["chapterNumber"] = *args.ChapterNumber
	}

	d.Progress.BroadcastProgress(jobID, int(types.JobTypeDownload),
		int(types.ProgressStatusRunning), 1,
		fmt.Sprintf("Starting %s Ch.%s", args.Title, chapStr), cardInfo)

	// ALWAYS load chapter via Suwayomi first — this triggers Suwayomi to cache/load the
	// chapter's pages. Without this step, page requests may return 404 because Suwayomi
	// hasn't fetched the pages from the source yet. This matches the .NET behavior.
	pageCountHint := args.PageCount
	dlSourceID := strconv.Itoa(args.SuwayomiID)
	dlMeta := map[string]string{"title": args.Title, "chapter": chapStr, "origin": "background_job"}

	chInfo, err := d.Suwayomi.GetChapter(ctx, args.SuwayomiID, args.ChapterIndex)
	if err != nil {
		util.LogSourceEvent(d.DB, dlSourceID, args.ProviderName, args.Language,
			"download", "failed", time.Since(dlStart).Milliseconds(),
			util.WithError(err), util.WithMetadata(dlMeta))
		return "", fmt.Errorf("fetch chapter info (trigger load): %w", err)
	}
	if chInfo.PageCount > 0 {
		pageCountHint = chInfo.PageCount
	}
	if pageCountHint <= 0 {
		pageCountHint = 50 // Default like .NET — actual loop uses 404 to stop
	}

	// Load provider for max chapter padding and chapter list
	sp, err := d.DB.SeriesProvider.Get(ctx, args.ProviderID)
	if err != nil {
		return "", fmt.Errorf("load provider: %w", err)
	}

	var maxChapter *float64
	for _, ch := range sp.Chapters {
		if ch.Number != nil {
			if maxChapter == nil || *ch.Number > *maxChapter {
				n := *ch.Number
				maxChapter = &n
			}
		}
	}
	if args.ChapterNumber != nil {
		if maxChapter == nil || *args.ChapterNumber > *maxChapter {
			n := *args.ChapterNumber
			maxChapter = &n
		}
	}

	// Fetch pages using a while-style loop with 404 detection (matching .NET approach).
	// We do NOT trust pageCount metadata — Suwayomi may report 0 or -1 for unloaded chapters.
	var pages []util.PageData
	for i := 0; ; i++ {
		data, _, err := d.Suwayomi.GetPage(ctx, args.SuwayomiID, args.ChapterIndex, i)
		if errors.Is(err, suwayomi.ErrNotFound) {
			// 404 means no more pages — graceful exit like .NET
			break
		}
		if err != nil {
			log.Warn().Err(err).Int("page", i).Str("title", args.Title).Str("chapter", chapStr).Msg("failed to fetch page")
			break
		}

		if len(data) == 0 {
			log.Warn().Int("page", i).Str("title", args.Title).Str("chapter", chapStr).Msg("page returned empty data")
			break
		}

		ext := util.DetectImageExtension(data)
		if ext == ".bin" {
			log.Warn().Int("page", i).Str("title", args.Title).Str("chapter", chapStr).Msg("page is not a valid image")
			break
		}

		filename := util.GeneratePageFilename(
			args.ProviderName, args.Scanlator, args.Language, args.Title,
			args.ChapterNumber, args.ChapterName, maxChapter,
			i, pageCountHint, ext,
		)

		pages = append(pages, util.PageData{Filename: filename, Data: data})

		pct := float64(i+1) / float64(pageCountHint) * 90 // Reserve 10% for finalization
		if pct > 90 {
			pct = 90
		}
		d.Progress.BroadcastProgress(jobID, int(types.JobTypeDownload),
			int(types.ProgressStatusRunning), pct,
			fmt.Sprintf("Downloading %s Ch.%s (%d/%d)", args.Title, chapStr, i+1, pageCountHint), cardInfo)
	}

	if len(pages) == 0 {
		noPageErr := fmt.Errorf("no pages downloaded for %s Ch.%s", args.Title, chapStr)
		util.LogSourceEvent(d.DB, dlSourceID, args.ProviderName, args.Language,
			"download", "failed", time.Since(dlStart).Milliseconds(),
			util.WithError(noPageErr), util.WithMetadata(dlMeta))
		return "", noPageErr
	}

	// Load series for metadata
	series, err := d.DB.Series.Get(ctx, args.SeriesID)
	if err != nil {
		return "", fmt.Errorf("load series: %w", err)
	}

	// Generate ComicInfo.xml
	var uploadDate *time.Time
	if args.UploadDate > 0 {
		t := time.UnixMilli(args.UploadDate).UTC()
		uploadDate = &t
	}

	ci := util.NewComicInfo(util.ChapterMeta{
		Title:         sp.Title,
		SeriesTitle:   series.Title,
		ProviderTitle: sp.Title,
		ChapterNumber: args.ChapterNumber,
		ChapterName:   args.ChapterName,
		ChapterCount:  len(sp.Chapters),
		PageCount:     len(pages),
		Language:      args.Language,
		Provider:      args.ProviderName,
		Scanlator:     args.Scanlator,
		Author:        series.Author,
		Artist:        series.Artist,
		Genre:         series.Genre,
		Type:          derefStrDefault(series.Type, ""),
		URL:           args.URL,
		UploadDate:    uploadDate,
	})

	// Create CBZ
	cbzFilename := util.GenerateCBZFilename(
		args.ProviderName, args.Scanlator, args.Language, args.Title,
		args.ChapterNumber, args.ChapterName, maxChapter,
	)
	destPath := filepath.Join(d.Config.Storage.Folder, args.StoragePath, cbzFilename)

	if err := util.CreateCBZ(destPath, pages, &ci); err != nil {
		return "", fmt.Errorf("create CBZ: %w", err)
	}

	// Update provider's chapters in database
	now := time.Now().UTC()
	chapters := make([]types.Chapter, len(sp.Chapters))
	copy(chapters, sp.Chapters)

	pc := len(pages)
	found := false
	for i, ch := range chapters {
		if ch.Number != nil && args.ChapterNumber != nil && *ch.Number == *args.ChapterNumber {
			chapters[i].Filename = cbzFilename
			chapters[i].PageCount = &pc
			chapters[i].DownloadDate = &now
			chapters[i].ShouldDownload = false
			chapters[i].IsDeleted = false
			found = true
			break
		}
	}
	if !found {
		chapters = append(chapters, types.Chapter{
			Name:           args.ChapterName,
			Number:         args.ChapterNumber,
			URL:            args.URL,
			ProviderIndex:  args.ChapterIndex,
			DownloadDate:   &now,
			ShouldDownload: false,
			IsDeleted:      false,
			PageCount:      &pc,
			Filename:       cbzFilename,
		})
		if uploadDate != nil {
			chapters[len(chapters)-1].ProviderUploadDate = uploadDate
		}
	}

	// Update ContinueAfterChapter to max downloaded chapter
	var maxNum *float64
	for _, ch := range chapters {
		if ch.Number != nil && ch.Filename != "" && !ch.IsDeleted {
			if maxNum == nil || *ch.Number > *maxNum {
				n := *ch.Number
				maxNum = &n
			}
		}
	}

	update := d.DB.SeriesProvider.UpdateOneID(sp.ID).
		SetChapters(chapters).
		SetChapterCount(int64(len(chapters)))
	if maxNum != nil {
		update = update.SetContinueAfterChapter(*maxNum)
	}
	if _, err := update.Save(ctx); err != nil {
		return "", fmt.Errorf("update provider chapters: %w", err)
	}

	// Save kaizoku.json
	if err := saveSeriesKaizokuJSON(ctx, d.DB, args.SeriesID, d.Config.Storage.Folder); err != nil {
		log.Warn().Err(err).Msg("failed to save kaizoku.json")
	}

	d.Progress.BroadcastProgress(jobID, int(types.JobTypeDownload),
		int(types.ProgressStatusCompleted), 100,
		fmt.Sprintf("Downloaded %s Ch.%s", args.Title, chapStr), cardInfo)

	util.LogSourceEvent(d.DB, dlSourceID, args.ProviderName, args.Language,
		"download", "success", time.Since(dlStart).Milliseconds(),
		util.WithItemsCount(len(pages)), util.WithMetadata(dlMeta))

	return cbzFilename, nil
}

// cascadeOnFailure tries the next fallback provider or schedules a full cascade retry.
func (d *Deps) cascadeOnFailure(ctx context.Context, args types.DownloadChapterArgs) {
	// Try remaining fallback providers
	if len(args.FallbackProviders) > 0 {
		if err := d.enqueueNextFallback(ctx, args); err == nil {
			return // Successfully cascaded
		}
	}

	// All fallbacks exhausted — schedule full cascade retry
	d.scheduleFullCascadeRetry(ctx, args)
}

// enqueueNextFallback finds the next usable fallback provider and enqueues a download.
func (d *Deps) enqueueNextFallback(ctx context.Context, args types.DownloadChapterArgs) error {
	for i, next := range args.FallbackProviders {
		remaining := args.FallbackProviders[i+1:]

		fallbackSP, err := d.DB.SeriesProvider.Get(ctx, next.ProviderID)
		if err != nil || fallbackSP.IsDisabled || fallbackSP.IsUninstalled || fallbackSP.SuwayomiID == 0 {
			continue
		}

		chIdx, chURL, pc := findChapterInProvider(fallbackSP, args.ChapterNumber)
		if chIdx < 0 {
			continue
		}

		newArgs := types.DownloadChapterArgs{
			SeriesID:          args.SeriesID,
			ProviderID:        next.ProviderID,
			SuwayomiID:        next.SuwayomiID,
			ChapterIndex:      chIdx,
			ChapterNumber:     args.ChapterNumber,
			ChapterName:       args.ChapterName,
			ProviderName:      fallbackSP.Provider,
			Scanlator:         fallbackSP.Scanlator,
			Language:          fallbackSP.Language,
			Title:             args.Title,
			StoragePath:       args.StoragePath,
			ThumbnailURL:      args.ThumbnailURL,
			URL:               chURL,
			PageCount:         pc,
			UploadDate:        args.UploadDate,
			FallbackProviders: remaining,
			CascadeRetries:    args.CascadeRetries,
		}

		if err := d.DownloadQueue.Enqueue(ctx, newArgs, time.Now()); err != nil {
			log.Warn().Err(err).Str("provider", fallbackSP.Provider).Msg("failed to enqueue cascade fallback")
			continue
		}
		log.Info().
			Str("title", args.Title).
			Str("fallback", fallbackSP.Provider).
			Msg("cascading to fallback provider")
		return nil
	}

	return fmt.Errorf("no usable fallback found")
}

// scheduleFullCascadeRetry rebuilds the full provider list and schedules a retry after a delay.
func (d *Deps) scheduleFullCascadeRetry(ctx context.Context, args types.DownloadChapterArgs) {
	maxRetries := d.Config.Settings.ChapterFailRetries
	if args.CascadeRetries >= maxRetries {
		log.Warn().
			Str("title", args.Title).
			Int("retries", args.CascadeRetries).
			Msg("chapter download permanently failed after all cascade retries")
		return
	}

	allProviders, err := d.DB.SeriesProvider.Query().
		Where(
			seriesprovider.SeriesIDEQ(args.SeriesID),
			seriesprovider.IsDisabledEQ(false),
			seriesprovider.IsUninstalledEQ(false),
		).
		All(ctx)
	if err != nil || len(allProviders) == 0 {
		log.Warn().Err(err).Msg("no providers available for cascade retry")
		return
	}

	sort.Slice(allProviders, func(i, j int) bool {
		return allProviders[i].Importance < allProviders[j].Importance
	})

	var primary *ent.SeriesProvider
	var primaryChIdx int
	var primaryURL string
	var primaryPC int

	for _, sp := range allProviders {
		if sp.SuwayomiID == 0 {
			continue
		}
		idx, url, pc := findChapterInProvider(sp, args.ChapterNumber)
		if idx >= 0 {
			primary = sp
			primaryChIdx = idx
			primaryURL = url
			primaryPC = pc
			break
		}
	}

	if primary == nil {
		log.Warn().Str("title", args.Title).Msg("no provider has this chapter for cascade retry")
		return
	}

	var fallbacks []types.FallbackSource
	for _, sp := range allProviders {
		if sp.ID == primary.ID || sp.SuwayomiID == 0 {
			continue
		}
		fallbacks = append(fallbacks, types.FallbackSource{
			ProviderID: sp.ID,
			SuwayomiID: sp.SuwayomiID,
			Importance: sp.Importance,
		})
	}

	retryDelay, err := time.ParseDuration(d.Config.Settings.ChapterFailRetryTime)
	if err != nil {
		retryDelay = 30 * time.Minute
	}

	newArgs := types.DownloadChapterArgs{
		SeriesID:          args.SeriesID,
		ProviderID:        primary.ID,
		SuwayomiID:        primary.SuwayomiID,
		ChapterIndex:      primaryChIdx,
		ChapterNumber:     args.ChapterNumber,
		ChapterName:       args.ChapterName,
		ProviderName:      primary.Provider,
		Scanlator:         primary.Scanlator,
		Language:          primary.Language,
		Title:             args.Title,
		StoragePath:       args.StoragePath,
		ThumbnailURL:      args.ThumbnailURL,
		URL:               primaryURL,
		PageCount:         primaryPC,
		UploadDate:        args.UploadDate,
		FallbackProviders: fallbacks,
		CascadeRetries:    args.CascadeRetries + 1,
	}

	if err := d.DownloadQueue.Enqueue(ctx, newArgs, time.Now().Add(retryDelay)); err != nil {
		log.Warn().Err(err).Msg("failed to schedule cascade retry")
	} else {
		log.Info().
			Str("title", args.Title).
			Int("retry", args.CascadeRetries+1).
			Dur("delay", retryDelay).
			Msg("scheduled cascade retry")
	}
}

// handleDownloadSuccess is called after a successful non-replacement download.
func (d *Deps) handleDownloadSuccess(ctx context.Context, args types.DownloadChapterArgs, cbzFilename string) {
	sp, err := d.DB.SeriesProvider.Get(ctx, args.ProviderID)
	if err != nil {
		return
	}

	if sp.Importance == 0 {
		d.cleanupInferiorCopies(ctx, args)
		return
	}

	betterProviders, err := d.DB.SeriesProvider.Query().
		Where(
			seriesprovider.SeriesIDEQ(args.SeriesID),
			seriesprovider.IsDisabledEQ(false),
			seriesprovider.IsUninstalledEQ(false),
			seriesprovider.ImportanceLT(sp.Importance),
		).
		All(ctx)
	if err != nil || len(betterProviders) == 0 {
		return
	}

	sort.Slice(betterProviders, func(i, j int) bool {
		return betterProviders[i].Importance < betterProviders[j].Importance
	})

	for _, better := range betterProviders {
		if better.SuwayomiID == 0 {
			continue
		}
		chIdx, chURL, pc := findChapterInProvider(better, args.ChapterNumber)
		if chIdx < 0 {
			continue
		}

		retryDelay, _ := time.ParseDuration(d.Config.Settings.ChapterFailRetryTime)
		if retryDelay == 0 {
			retryDelay = 30 * time.Minute
		}

		repArgs := types.DownloadChapterArgs{
			SeriesID:            args.SeriesID,
			ProviderID:          better.ID,
			SuwayomiID:          better.SuwayomiID,
			ChapterIndex:        chIdx,
			ChapterNumber:       args.ChapterNumber,
			ChapterName:         args.ChapterName,
			ProviderName:        better.Provider,
			Scanlator:           better.Scanlator,
			Language:            better.Language,
			Title:               args.Title,
			StoragePath:         args.StoragePath,
			ThumbnailURL:        args.ThumbnailURL,
			URL:                 chURL,
			PageCount:           pc,
			UploadDate:          args.UploadDate,
			IsReplacement:       true,
			ReplacingProviderID: args.ProviderID,
			ReplacingFilename:   cbzFilename,
		}

		if err := d.DownloadQueue.Enqueue(ctx, repArgs, time.Now().Add(retryDelay)); err != nil {
			log.Warn().Err(err).Msg("failed to schedule replacement download")
		} else {
			log.Info().
				Str("title", args.Title).
				Str("from", better.Provider).
				Str("replacing", args.ProviderName).
				Msg("scheduled replacement download")
		}
		break
	}
}

// cleanupInferiorCopies deletes chapter copies from less-important providers.
func (d *Deps) cleanupInferiorCopies(ctx context.Context, args types.DownloadChapterArgs) {
	if args.ChapterNumber == nil {
		return
	}

	allProviders, err := d.DB.SeriesProvider.Query().
		Where(
			seriesprovider.SeriesIDEQ(args.SeriesID),
			seriesprovider.IDNEQ(args.ProviderID),
		).
		All(ctx)
	if err != nil {
		return
	}

	for _, other := range allProviders {
		chapters := make([]types.Chapter, len(other.Chapters))
		copy(chapters, other.Chapters)
		changed := false

		for i, ch := range chapters {
			if ch.Number == nil || *ch.Number != *args.ChapterNumber {
				continue
			}
			if ch.Filename == "" || ch.IsDeleted {
				continue
			}
			filePath := filepath.Join(d.Config.Storage.Folder, args.StoragePath, ch.Filename)
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				log.Warn().Err(err).Str("file", ch.Filename).Msg("failed to delete inferior copy")
				continue
			}
			chapters[i].IsDeleted = true
			chapters[i].Filename = ""
			changed = true
			log.Info().
				Str("file", ch.Filename).
				Str("provider", other.Provider).
				Msg("cleaned up inferior copy")
		}

		if changed {
			if _, err := d.DB.SeriesProvider.UpdateOneID(other.ID).
				SetChapters(chapters).Save(ctx); err != nil {
				log.Warn().Err(err).Str("provider", other.Provider).Msg("failed to update chapters after cleanup")
			}
		}
	}
}

// handleReplacementSuccess is called when a replacement download succeeds.
func (d *Deps) handleReplacementSuccess(ctx context.Context, args types.DownloadChapterArgs) {
	if args.ReplacingFilename == "" {
		return
	}

	oldPath := filepath.Join(d.Config.Storage.Folder, args.StoragePath, args.ReplacingFilename)
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Str("file", args.ReplacingFilename).Msg("failed to delete replaced file")
	}

	oldSP, err := d.DB.SeriesProvider.Get(ctx, args.ReplacingProviderID)
	if err != nil {
		return
	}

	chapters := make([]types.Chapter, len(oldSP.Chapters))
	copy(chapters, oldSP.Chapters)
	changed := false

	for i, ch := range chapters {
		if ch.Number != nil && args.ChapterNumber != nil && *ch.Number == *args.ChapterNumber {
			chapters[i].IsDeleted = true
			chapters[i].Filename = ""
			changed = true
			break
		}
	}

	if changed {
		if _, err := d.DB.SeriesProvider.UpdateOneID(oldSP.ID).
			SetChapters(chapters).Save(ctx); err != nil {
			log.Warn().Err(err).Str("provider", oldSP.Provider).Msg("failed to update chapters after replacement")
		}
	}

	log.Info().
		Str("title", args.Title).
		Str("replaced", args.ReplacingFilename).
		Str("from", args.ProviderName).
		Msg("replacement download complete")
}

// handleReplacementFailure retries the replacement or tries the next importance level.
func (d *Deps) handleReplacementFailure(ctx context.Context, args types.DownloadChapterArgs) {
	maxRetries := d.Config.Settings.ChapterFailRetries
	if args.ReplacementRetry < maxRetries {
		retryDelay, _ := time.ParseDuration(d.Config.Settings.ChapterFailRetryTime)
		if retryDelay == 0 {
			retryDelay = 30 * time.Minute
		}

		newArgs := args
		newArgs.ReplacementRetry++

		if err := d.DownloadQueue.Enqueue(ctx, newArgs, time.Now().Add(retryDelay)); err != nil {
			log.Warn().Err(err).Msg("failed to schedule replacement retry")
		}
		return
	}

	sp, err := d.DB.SeriesProvider.Get(ctx, args.ProviderID)
	if err != nil {
		return
	}

	replacingSP, err := d.DB.SeriesProvider.Get(ctx, args.ReplacingProviderID)
	if err != nil {
		return
	}

	nextProviders, err := d.DB.SeriesProvider.Query().
		Where(
			seriesprovider.SeriesIDEQ(args.SeriesID),
			seriesprovider.IsDisabledEQ(false),
			seriesprovider.IsUninstalledEQ(false),
			seriesprovider.ImportanceGT(sp.Importance),
			seriesprovider.ImportanceLT(replacingSP.Importance),
		).
		All(ctx)
	if err != nil || len(nextProviders) == 0 {
		log.Info().
			Str("title", args.Title).
			Msg("replacement exhausted all better providers, keeping current copy")
		return
	}

	sort.Slice(nextProviders, func(i, j int) bool {
		return nextProviders[i].Importance < nextProviders[j].Importance
	})

	for _, next := range nextProviders {
		if next.SuwayomiID == 0 {
			continue
		}
		chIdx, chURL, pc := findChapterInProvider(next, args.ChapterNumber)
		if chIdx < 0 {
			continue
		}

		retryDelay, _ := time.ParseDuration(d.Config.Settings.ChapterFailRetryTime)
		if retryDelay == 0 {
			retryDelay = 30 * time.Minute
		}

		newArgs := types.DownloadChapterArgs{
			SeriesID:            args.SeriesID,
			ProviderID:          next.ID,
			SuwayomiID:          next.SuwayomiID,
			ChapterIndex:        chIdx,
			ChapterNumber:       args.ChapterNumber,
			ChapterName:         args.ChapterName,
			ProviderName:        next.Provider,
			Scanlator:           next.Scanlator,
			Language:            next.Language,
			Title:               args.Title,
			StoragePath:         args.StoragePath,
			ThumbnailURL:        args.ThumbnailURL,
			URL:                 chURL,
			PageCount:           pc,
			UploadDate:          args.UploadDate,
			IsReplacement:       true,
			ReplacingProviderID: args.ReplacingProviderID,
			ReplacingFilename:   args.ReplacingFilename,
			ReplacementRetry:    0,
		}

		if err := d.DownloadQueue.Enqueue(ctx, newArgs, time.Now().Add(retryDelay)); err != nil {
			log.Warn().Err(err).Msg("failed to schedule next replacement provider")
			continue
		}
		log.Info().
			Str("title", args.Title).
			Str("nextProvider", next.Provider).
			Msg("replacement moving to next importance level")
		break
	}
}

// findChapterInProvider looks up a chapter by number in a provider's chapter list.
// Returns the provider index, URL, and page count, or (-1, "", 0) if not found.
func findChapterInProvider(sp *ent.SeriesProvider, chapterNumber *float64) (index int, url string, pageCount int) {
	if chapterNumber == nil {
		return -1, "", 0
	}
	for _, ch := range sp.Chapters {
		if ch.Number != nil && *ch.Number == *chapterNumber {
			pc := 0
			if ch.PageCount != nil {
				pc = *ch.PageCount
			}
			return ch.ProviderIndex, ch.URL, pc
		}
	}
	return -1, "", 0
}

// ============================================================
// GetChaptersWorker — fetches chapters for a provider, queues downloads
// ============================================================

type GetChaptersWorker struct {
	river.WorkerDefaults[GetChaptersArgs]
	Deps *Deps
}

func (w *GetChaptersWorker) Work(ctx context.Context, job *river.Job[GetChaptersArgs]) error {
	providerID := job.Args.ProviderID

	// Load the series provider
	sp, err := w.Deps.DB.SeriesProvider.Get(ctx, providerID)
	if err != nil {
		log.Warn().Err(err).Str("providerId", providerID.String()).Msg("provider not found, deleting job")
		return nil // Provider doesn't exist anymore
	}

	if sp.IsDisabled || sp.IsUninstalled {
		log.Info().Str("provider", sp.Provider).Msg("provider is disabled or uninstalled, skipping")
		return nil
	}

	if sp.SuwayomiID == 0 {
		log.Warn().Str("provider", sp.Provider).Msg("no suwayomi ID for provider")
		return nil
	}

	log.Info().
		Str("provider", sp.Provider).
		Int("suwayomiId", sp.SuwayomiID).
		Msg("fetching chapters")

	// Fetch chapters from Suwayomi
	chStart := time.Now()
	onlineChapters, err := w.Deps.Suwayomi.GetChapters(ctx, sp.SuwayomiID, true)
	chDuration := time.Since(chStart).Milliseconds()
	if err != nil {
		util.LogSourceEvent(w.Deps.DB, strconv.Itoa(sp.SuwayomiID), sp.Provider, sp.Language,
			"get_chapters", "failed", chDuration,
			util.WithError(err))
		return fmt.Errorf("fetch chapters from suwayomi: %w", err)
	}
	util.LogSourceEvent(w.Deps.DB, strconv.Itoa(sp.SuwayomiID), sp.Provider, sp.Language,
		"get_chapters", "success", chDuration,
		util.WithItemsCount(len(onlineChapters)))

	// Load series info
	series, err := w.Deps.DB.Series.Get(ctx, sp.SeriesID)
	if err != nil {
		return fmt.Errorf("load series: %w", err)
	}

	if series.PauseDownloads {
		log.Info().Str("title", series.Title).Msg("downloads paused for series, skipping")
		return nil
	}

	// Load ALL providers for the series (including disabled — needed for chapter dedup)
	allProviders, err := w.Deps.DB.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(sp.SeriesID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("load all providers for series: %w", err)
	}

	// Generate list of chapters to download with cross-provider dedup
	toDownload := generateDownloads(sp, allProviders, onlineChapters)
	if len(toDownload) == 0 {
		log.Info().Str("provider", sp.Provider).Msg("no new chapters to download")
		return nil
	}

	log.Info().
		Str("provider", sp.Provider).
		Int("count", len(toDownload)).
		Msg("queueing chapter downloads")

	// Sort chapters by number ascending so downloads start from chapter 1
	sort.Slice(toDownload, func(i, j int) bool {
		a, b := toDownload[i].ChapterNumber, toDownload[j].ChapterNumber
		if a == nil {
			return true
		}
		if b == nil {
			return false
		}
		return *a < *b
	})

	// Build fallback providers (other active providers, sorted by importance)
	var fallbacks []types.FallbackSource
	for _, other := range allProviders {
		if other.ID == sp.ID || other.IsDisabled || other.IsUninstalled || other.SuwayomiID == 0 {
			continue
		}
		fallbacks = append(fallbacks, types.FallbackSource{
			ProviderID: other.ID,
			SuwayomiID: other.SuwayomiID,
			Importance: other.Importance,
		})
	}
	sort.Slice(fallbacks, func(i, j int) bool {
		return fallbacks[i].Importance < fallbacks[j].Importance
	})

	// Enqueue download jobs via custom DownloadDispatcher.
	// All jobs get the same scheduled_at — the dispatcher uses priority (chapter number)
	// and per-provider concurrency limits to control ordering.
	baseTime := time.Now()
	for _, ch := range toDownload {
		args := types.DownloadChapterArgs{
			SeriesID:          sp.SeriesID,
			ProviderID:        sp.ID,
			SuwayomiID:        sp.SuwayomiID,
			ChapterIndex:      ch.Index,
			ChapterNumber:     ch.ChapterNumber,
			ChapterName:       ch.Name,
			ProviderName:      sp.Provider,
			Scanlator:         sp.Scanlator,
			Language:          sp.Language,
			Title:             series.Title,
			StoragePath:       series.StoragePath,
			ThumbnailURL:      series.ThumbnailURL,
			URL:               ch.URL,
			PageCount:         ch.PageCount,
			UploadDate:        ch.UploadDate,
			FallbackProviders: fallbacks,
		}
		if err := w.Deps.DownloadQueue.Enqueue(ctx, args, baseTime); err != nil {
			log.Warn().Err(err).Str("chapter", ch.Name).Msg("failed to enqueue download")
		}
	}

	// Update provider fetch date and chapter count
	now := time.Now().UTC()
	_, err = w.Deps.DB.SeriesProvider.UpdateOneID(sp.ID).
		SetFetchDate(now).
		SetChapterCount(int64(len(onlineChapters))).
		Save(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to update provider fetch date")
	}

	return nil
}

// generateDownloads compares online chapters with local data across all providers
// and returns chapters to download, respecting importance-based deduplication.
func generateDownloads(sp *ent.SeriesProvider, allProviders []*ent.SeriesProvider, online []suwayomi.SuwayomiChapter) []suwayomi.SuwayomiChapter {
	// Filter by scanlator if provider specifies one
	var filtered []suwayomi.SuwayomiChapter
	for _, ch := range online {
		scanlator := ""
		if ch.Scanlator != nil {
			scanlator = *ch.Scanlator
		}
		if sp.Scanlator == "" || sp.Scanlator == sp.Provider {
			// No specific scanlator filter - accept all
			filtered = append(filtered, ch)
		} else if strings.EqualFold(scanlator, sp.Scanlator) {
			filtered = append(filtered, ch)
		}
	}

	// Build set of already-downloaded chapter numbers for THIS provider
	thisDownloaded := make(map[float64]bool)
	for _, ch := range sp.Chapters {
		if ch.Number != nil && ch.Filename != "" && !ch.IsDeleted {
			thisDownloaded[*ch.Number] = true
		}
	}

	// Build cross-provider chapter availability map.
	// Track both DOWNLOADED and AVAILABLE chapters from other providers.
	// This prevents race conditions where multiple providers generate duplicate downloads.
	type chapterStatus struct {
		hasDisabledCopy      bool // A disabled/removed provider has this on disk
		bestActiveDownloaded int  // Lowest importance of active providers with this chapter downloaded (-1 = none)
		bestActiveAvailable  int  // Lowest importance of active providers with this chapter available (-1 = none)
	}
	otherChapters := make(map[float64]*chapterStatus)

	ensureStatus := func(num float64) *chapterStatus {
		if s, ok := otherChapters[num]; ok {
			return s
		}
		s := &chapterStatus{bestActiveDownloaded: -1, bestActiveAvailable: -1}
		otherChapters[num] = s
		return s
	}

	for _, other := range allProviders {
		if other.ID == sp.ID {
			continue
		}
		isActive := !other.IsDisabled && !other.IsUninstalled

		for _, ch := range other.Chapters {
			if ch.Number == nil {
				continue
			}
			num := *ch.Number
			status := ensureStatus(num)

			// Track downloaded chapters
			if ch.Filename != "" && !ch.IsDeleted {
				if !isActive {
					status.hasDisabledCopy = true
				} else {
					if status.bestActiveDownloaded == -1 || other.Importance < status.bestActiveDownloaded {
						status.bestActiveDownloaded = other.Importance
					}
				}
			}

			// Track available chapters (in chapter list, regardless of download status)
			if isActive {
				if status.bestActiveAvailable == -1 || other.Importance < status.bestActiveAvailable {
					status.bestActiveAvailable = other.Importance
				}
			}
		}
	}

	// Filter chapters based on importance rules
	continueAfter := 0.0
	if sp.ContinueAfterChapter != nil {
		continueAfter = *sp.ContinueAfterChapter
	}

	var result []suwayomi.SuwayomiChapter
	for _, ch := range filtered {
		if ch.ChapterNumber == nil {
			continue
		}
		num := *ch.ChapterNumber

		// Skip if this provider already has it
		if thisDownloaded[num] {
			continue
		}
		// Respect ContinueAfterChapter for imported series
		if continueAfter > 0 && num <= continueAfter {
			continue
		}
		// Check cross-provider availability
		if status, ok := otherChapters[num]; ok {
			// A more-important (or equal) active provider has it DOWNLOADED — skip
			if status.bestActiveDownloaded >= 0 && status.bestActiveDownloaded <= sp.Importance {
				continue
			}
			// A more-important active provider has this chapter AVAILABLE (even if not yet downloaded) — skip.
			// This prevents race conditions on initial import where all providers generate downloads simultaneously.
			if status.bestActiveAvailable >= 0 && status.bestActiveAvailable < sp.Importance {
				continue
			}
			// Only disabled/removed providers have it downloaded — skip (preserve existing file)
			if status.hasDisabledCopy && status.bestActiveDownloaded == -1 {
				continue
			}
			// A less-important active provider has it downloaded — include for replacement
		}

		result = append(result, ch)
	}

	return result
}

// ============================================================
// GetLatestWorker — fetches latest series from a source
// ============================================================

type GetLatestWorker struct {
	river.WorkerDefaults[GetLatestArgs]
	Deps *Deps
}

func (w *GetLatestWorker) Work(ctx context.Context, job *river.Job[GetLatestArgs]) error {
	sourceID := job.Args.SourceID
	log.Info().Str("sourceId", sourceID).Msg("fetching latest series")

	start := time.Now()

	// Resolve source name and language from Suwayomi
	sourceName := sourceID
	sourceLang := ""
	if sources, err := w.Deps.Suwayomi.GetSources(ctx); err == nil {
		for _, s := range sources {
			if s.ID == sourceID {
				sourceName = s.Name
				sourceLang = strings.ToLower(s.Lang)
				break
			}
		}
	}

	// Load known latest series for this source
	knownLatest, err := w.Deps.DB.LatestSeries.Query().
		Where(latestseries.SuwayomiSourceIDEQ(sourceID)).
		All(ctx)
	if err != nil {
		util.LogSourceEvent(w.Deps.DB, sourceID, sourceName, sourceLang,
			"get_latest", "failed", time.Since(start).Milliseconds(),
			util.WithError(err))
		return fmt.Errorf("query known latest: %w", err)
	}

	// Build lookup by suwayomi ID
	knownMap := make(map[int]*ent.LatestSeries)
	for _, ls := range knownLatest {
		knownMap[ls.ID] = ls
	}

	neverDone := len(knownMap) == 0
	upToDate := false
	itemsCount := 0

	for page := 1; !upToDate || neverDone; page++ {
		result, err := w.Deps.Suwayomi.GetLatestSeries(ctx, sourceID, page)
		if err != nil {
			log.Warn().Err(err).Int("page", page).Msg("failed to fetch latest page")
			util.LogSourceEvent(w.Deps.DB, sourceID, sourceName, sourceLang,
				"get_latest", "failed", time.Since(start).Milliseconds(),
				util.WithError(err), util.WithItemsCount(itemsCount))
			break
		}
		if result == nil || len(result.MangaList) == 0 {
			break
		}

		for _, manga := range result.MangaList {
			// Fetch full series data if not cached or stale
			known, exists := knownMap[manga.ID]
			needFullFetch := !exists || (exists && time.Since(known.FetchDate) > 7*24*time.Hour)

			var fullData *suwayomi.SuwayomiSeries
			if needFullFetch {
				fullData, err = w.Deps.Suwayomi.GetFullSeriesData(ctx, manga.ID, true)
				if err != nil {
					log.Warn().Err(err).Int("mangaId", manga.ID).Msg("failed to fetch full series data")
					continue
				}
			}

			// Fetch chapters
			chapters, err := w.Deps.Suwayomi.GetChapters(ctx, manga.ID, true)
			if err != nil {
				log.Warn().Err(err).Int("mangaId", manga.ID).Msg("failed to fetch chapters")
				continue
			}

			itemsCount++

			// Check if we're up to date (seen this before with same latest chapter)
			if exists && !neverDone && len(chapters) > 0 {
				latestOnline := latestChapter(chapters)
				if known.ChapterCount != nil && latestOnline != nil {
					if latestOnline.Index <= int(*known.ChapterCount) {
						upToDate = true
					}
				}
			}

			// Store/update LatestSeries record
			w.upsertLatestSeries(ctx, sourceID, sourceName, sourceLang, manga, fullData, chapters)
		}

		// First time through, do all pages
		if neverDone && page >= 5 {
			break // Safety limit for first run
		}
	}

	util.LogSourceEvent(w.Deps.DB, sourceID, sourceName, sourceLang,
		"get_latest", "success", time.Since(start).Milliseconds(),
		util.WithItemsCount(itemsCount))

	log.Info().Str("sourceId", sourceID).Msg("finished fetching latest")
	return nil
}

func (w *GetLatestWorker) upsertLatestSeries(
	ctx context.Context,
	sourceID, sourceName, sourceLang string,
	manga suwayomi.SuwayomiSeries,
	fullData *suwayomi.SuwayomiSeries,
	chapters []suwayomi.SuwayomiChapter,
) {
	now := time.Now().UTC()

	// Convert chapters to the types.SuwayomiChapter format for storage
	storedChapters := make([]types.SuwayomiChapter, len(chapters))
	for i, ch := range chapters {
		scanlator := ""
		if ch.Scanlator != nil {
			scanlator = *ch.Scanlator
		}
		storedChapters[i] = types.SuwayomiChapter{
			ID:            ch.ID,
			URL:           ch.URL,
			Name:          ch.Name,
			UploadDate:    ch.UploadDate,
			ChapterNumber: ch.ChapterNumber,
			Scanlator:     scanlator,
			MangaID:       ch.MangaID,
			Index:         ch.Index,
			PageCount:     ch.PageCount,
		}
	}

	// Calculate latest chapter info
	var latestNum *float64
	latestTitle := ""
	if latest := latestChapter(chapters); latest != nil {
		latestNum = latest.ChapterNumber
		latestTitle = latest.Name
	}

	// Use resolved source name/language, enrich from full data if available
	title := manga.Title
	provider := sourceName
	language := sourceLang
	artist := ""
	author := ""
	description := ""
	status := "UNKNOWN"
	var genre []string
	var thumbnailURL *string

	if fullData != nil {
		title = fullData.Title
		if fullData.Artist != nil {
			artist = *fullData.Artist
		}
		if fullData.Author != nil {
			author = *fullData.Author
		}
		if fullData.Description != nil {
			description = *fullData.Description
		}
		status = fullData.Status
		genre = fullData.Genre
		// Use proxy path instead of raw Suwayomi URL (not accessible from browser)
		proxyPath := fmt.Sprintf("serie/thumb/%d", manga.ID)
		thumbnailURL = &proxyPath
	}

	// Check InLibrary status
	inLibrary := int(types.InLibraryNotInLibrary)
	existingProviders, _ := w.Deps.DB.SeriesProvider.Query().
		Where(seriesprovider.SuwayomiIDEQ(manga.ID)).
		All(ctx)
	for _, ep := range existingProviders {
		if ep.IsDisabled || ep.IsUninstalled {
			inLibrary = int(types.InLibraryInLibraryDisabled)
		} else {
			inLibrary = int(types.InLibraryInLibrary)
			break
		}
	}

	// Upsert
	cc := int64(len(chapters))
	err := w.Deps.DB.LatestSeries.Create().
		SetID(manga.ID).
		SetSuwayomiSourceID(sourceID).
		SetProvider(provider).
		SetLanguage(language).
		SetTitle(title).
		SetNillableURL(&manga.URL).
		SetNillableThumbnailURL(thumbnailURL).
		SetNillableArtist(strPtr(artist)).
		SetNillableAuthor(strPtr(author)).
		SetNillableDescription(strPtr(description)).
		SetGenre(genre).
		SetFetchDate(now).
		SetNillableChapterCount(&cc).
		SetNillableLatestChapter(latestNum).
		SetLatestChapterTitle(latestTitle).
		SetStatus(status).
		SetInLibrary(inLibrary).
		SetChapters(storedChapters).
		OnConflictColumns("id").
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		log.Warn().Err(err).Int("mangaId", manga.ID).Msg("failed to upsert latest series")
	}
}

// ============================================================
// UpdateExtensionsWorker — refreshes extension cache and auto-updates
// ============================================================

type UpdateExtensionsWorker struct {
	river.WorkerDefaults[UpdateExtensionsArgs]
	Deps *Deps
}

func (w *UpdateExtensionsWorker) Work(ctx context.Context, job *river.Job[UpdateExtensionsArgs]) error {
	log.Info().Msg("checking for extension updates")

	// Step 1: Fetch all extensions
	extensions, err := w.Deps.Suwayomi.GetExtensions(ctx)
	if err != nil {
		return fmt.Errorf("fetch extensions: %w", err)
	}

	// Step 2: Auto-update extensions that have updates
	updated := false
	for _, ext := range extensions {
		if ext.HasUpdate {
			log.Info().Str("pkg", ext.PkgName).Msg("updating extension")
			if err := w.Deps.Suwayomi.UpdateExtension(ctx, ext.PkgName); err != nil {
				log.Warn().Err(err).Str("pkg", ext.PkgName).Msg("failed to update extension")
			} else {
				updated = true
			}
		}
	}

	// Re-fetch if any were updated
	if updated {
		extensions, err = w.Deps.Suwayomi.GetExtensions(ctx)
		if err != nil {
			return fmt.Errorf("re-fetch extensions: %w", err)
		}
	}

	// Step 3: Fetch all sources
	sources, err := w.Deps.Suwayomi.GetSources(ctx)
	if err != nil {
		return fmt.Errorf("fetch sources: %w", err)
	}

	// Step 4: Fetch existing providers from DB
	existingProviders, err := w.Deps.DB.ProviderStorage.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("query providers: %w", err)
	}

	// Step 5: Process each extension
	for _, ext := range extensions {
		w.processExtension(ctx, ext, sources, existingProviders)
	}

	log.Info().Int("count", len(extensions)).Msg("extension update complete")
	return nil
}

func (w *UpdateExtensionsWorker) processExtension(
	ctx context.Context,
	ext suwayomi.SuwayomiExtension,
	sources []suwayomi.SuwayomiSource,
	existing []*ent.ProviderStorage,
) {
	// Find existing provider with matching name+lang+versionCode
	var provider *ent.ProviderStorage
	for _, p := range existing {
		if p.Name == ext.Name && p.Lang == ext.Lang && p.VersionCode == ext.VersionCode {
			provider = p
			break
		}
	}

	if provider != nil {
		// Sync disabled state
		if provider.IsDisabled != !ext.Installed {
			_, err := w.Deps.DB.ProviderStorage.UpdateOneID(provider.ID).
				SetIsDisabled(!ext.Installed).
				Save(ctx)
			if err != nil {
				log.Warn().Err(err).Str("name", ext.Name).Msg("failed to sync provider disabled state")
			}
		}
		return
	}

	// Check for version upgrade (same name+lang, different version)
	// Preserve IsStorage from old entry
	preserveIsStorage := true
	for _, p := range existing {
		if p.Name == ext.Name && p.Lang == ext.Lang && p.VersionCode != ext.VersionCode {
			preserveIsStorage = p.IsStorage
			if err := w.Deps.DB.ProviderStorage.DeleteOneID(p.ID).Exec(ctx); err != nil {
				log.Warn().Err(err).Str("name", ext.Name).Msg("failed to delete old provider version")
			}
			break
		}
	}

	// Auto-install if explicitly enabled in DB and not yet installed.
	// Extensions with no DB entry are NOT auto-installed — they must be
	// installed manually or via the InstallExtensionsWorker.
	if !ext.Installed {
		foundInDB := false
		isDisabled := true
		for _, p := range existing {
			if p.Name == ext.Name && p.Lang == ext.Lang {
				foundInDB = true
				isDisabled = p.IsDisabled
				break
			}
		}
		if foundInDB && !isDisabled {
			log.Info().Str("pkg", ext.PkgName).Msg("auto-installing extension")
			if err := w.Deps.Suwayomi.InstallExtension(ctx, ext.PkgName); err != nil {
				log.Warn().Err(err).Str("pkg", ext.PkgName).Msg("failed to auto-install extension")
			}
		}
	}

	// Build source mappings — exact name match (like .NET)
	var mappings []types.ProviderMapping
	for _, src := range sources {
		matches := false
		if ext.Lang == "all" {
			matches = src.Name == ext.Name
		} else {
			matches = src.Name == ext.Name && src.Lang == ext.Lang
		}
		if !matches {
			continue
		}

		// Fetch preferences for this source
		prefs, err := w.Deps.Suwayomi.GetSourcePreferences(ctx, src.ID)
		if err != nil {
			log.Warn().Err(err).Str("sourceId", src.ID).Msg("failed to fetch source preferences")
			prefs = nil
		}

		srcCopy := types.SuwayomiSource{
			ID:             src.ID,
			Name:           src.Name,
			Lang:           src.Lang,
			IconURL:        src.IconURL,
			SupportsLatest: src.SupportsLatest,
			IsConfigurable: src.IsConfigurable,
			IsNsfw:         src.IsNsfw,
			DisplayName:    src.DisplayName,
		}

		// Convert suwayomi prefs to types prefs
		// Remove lang suffix for "all" lang extensions and set Source (like .NET's RemoveSuffixPreferences)
		var typePrefs []types.SuwayomiPreference
		for _, p := range prefs {
			key := p.Props.Key
			if ext.Lang == "all" {
				if idx := strings.LastIndex(key, "_"); idx > 0 {
					key = key[:idx]
				}
			}
			typePrefs = append(typePrefs, types.SuwayomiPreference{
				Type:   p.Type,
				Source: src.ID,
				Props: types.SuwayomiProp{
					Key:              key,
					Title:            p.Props.Title,
					Summary:          p.Props.Summary,
					DefaultValue:     p.Props.DefaultValue,
					Entries:          p.Props.Entries,
					EntryValues:      p.Props.EntryValues,
					DefaultValueType: p.Props.DefaultValueType,
					CurrentValue:     p.Props.CurrentValue,
					Visible:          p.Props.Visible,
					DialogTitle:      p.Props.DialogTitle,
					DialogMessage:    p.Props.DialogMessage,
					Text:             p.Props.Text,
				},
			})
		}

		mappings = append(mappings, types.ProviderMapping{
			Source:      &srcCopy,
			Preferences: typePrefs,
		})
	}

	// Create provider in DB
	err := w.Deps.DB.ProviderStorage.Create().
		SetApkName(ext.ApkName).
		SetPkgName(ext.PkgName).
		SetName(ext.Name).
		SetLang(ext.Lang).
		SetVersionCode(ext.VersionCode).
		SetIsStorage(preserveIsStorage).
		SetIsDisabled(!ext.Installed).
		SetMappings(mappings).
		Exec(ctx)
	if err != nil {
		log.Warn().Err(err).Str("name", ext.Name).Msg("failed to create provider")
	}
}

// ============================================================
// UpdateAllSeriesWorker — updates all series metadata/archives
// ============================================================

type UpdateAllSeriesWorker struct {
	river.WorkerDefaults[UpdateAllSeriesArgs]
	Deps *Deps
}

func (w *UpdateAllSeriesWorker) Work(ctx context.Context, job *river.Job[UpdateAllSeriesArgs]) error {
	jobID := fmt.Sprintf("update-all-%d", job.ID)
	log.Info().Msg("updating all series")

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeUpdateAllSeries),
		int(types.ProgressStatusRunning), 0, "Updating all Series...", nil)

	allSeries, err := w.Deps.DB.Series.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("query all series: %w", err)
	}

	if len(allSeries) == 0 {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeUpdateAllSeries),
			int(types.ProgressStatusCompleted), 100, "No series to update.", nil)
		return nil
	}

	step := 100.0 / float64(len(allSeries))
	acum := 0.0

	for _, s := range allSeries {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeUpdateAllSeries),
			int(types.ProgressStatusRunning), acum, "Updating "+s.Title+"...", nil)

		if err := w.updateSeries(ctx, s); err != nil {
			log.Warn().Err(err).Str("title", s.Title).Msg("failed to update series")
		}

		acum += step
	}

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeUpdateAllSeries),
		int(types.ProgressStatusCompleted), 100, "Update Complete.", nil)

	log.Info().Int("count", len(allSeries)).Msg("all series update complete")
	return nil
}

func (w *UpdateAllSeriesWorker) updateSeries(ctx context.Context, s *ent.Series) error {
	providers, err := w.Deps.DB.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(s.ID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query providers: %w", err)
	}

	storageDir := filepath.Join(w.Deps.Config.Storage.Folder, s.StoragePath)

	for _, sp := range providers {
		// Get max chapter for filename padding
		var maxChapter *float64
		for _, ch := range sp.Chapters {
			if ch.Number != nil {
				if maxChapter == nil || *ch.Number > *maxChapter {
					n := *ch.Number
					maxChapter = &n
				}
			}
		}

		chaptersChanged := false
		for i, ch := range sp.Chapters {
			if ch.Filename == "" || ch.IsDeleted {
				continue
			}

			// Regenerate expected filename
			expectedFilename := util.GenerateCBZFilename(
				sp.Provider, sp.Scanlator, sp.Language, s.Title,
				ch.Number, ch.Name, maxChapter,
			)

			oldPath := filepath.Join(storageDir, ch.Filename)
			newPath := filepath.Join(storageDir, expectedFilename)

			// Rename if filename changed
			if ch.Filename != expectedFilename {
				if _, err := os.Stat(oldPath); err == nil {
					if err := os.Rename(oldPath, newPath); err != nil {
						log.Warn().Err(err).
							Str("old", ch.Filename).
							Str("new", expectedFilename).
							Msg("failed to rename CBZ")
						continue
					}
					sp.Chapters[i].Filename = expectedFilename
					chaptersChanged = true
				}
			}

			// Update ComicInfo.xml in the CBZ
			cbzPath := newPath
			if _, err := os.Stat(cbzPath); err != nil {
				continue
			}

			ci := util.NewComicInfo(util.ChapterMeta{
				Title:         sp.Title,
				SeriesTitle:   s.Title,
				ProviderTitle: sp.Title,
				ChapterNumber: ch.Number,
				ChapterName:   ch.Name,
				ChapterCount:  len(sp.Chapters),
				PageCount:     derefIntDefault(ch.PageCount, 0),
				Language:      sp.Language,
				Provider:      sp.Provider,
				Scanlator:     sp.Scanlator,
				Author:        s.Author,
				Artist:        s.Artist,
				Genre:         s.Genre,
				Type:          derefStrDefault(s.Type, ""),
				URL:           ch.URL,
				UploadDate:    ch.ProviderUploadDate,
			})

			if err := util.UpdateCBZComicInfo(cbzPath, ci); err != nil {
				log.Warn().Err(err).Str("file", cbzPath).Msg("failed to update ComicInfo in CBZ")
			}
		}

		// Save updated chapters if any were renamed
		if chaptersChanged {
			if _, err := w.Deps.DB.SeriesProvider.UpdateOneID(sp.ID).
				SetChapters(sp.Chapters).
				Save(ctx); err != nil {
				log.Warn().Err(err).Str("provider", sp.Provider).Msg("failed to save updated chapters")
			}
		}
	}

	// Save kaizoku.json
	return saveSeriesKaizokuJSON(ctx, w.Deps.DB, s.ID, w.Deps.Config.Storage.Folder)
}

// ============================================================
// RefreshAllChaptersWorker — dispatches GetChapters for all active providers
// ============================================================

type RefreshAllChaptersWorker struct {
	river.WorkerDefaults[RefreshAllChaptersArgs]
	Deps *Deps
}

func (w *RefreshAllChaptersWorker) Work(ctx context.Context, job *river.Job[RefreshAllChaptersArgs]) error {
	providers, err := w.Deps.DB.SeriesProvider.Query().
		Where(
			seriesprovider.IsDisabledEQ(false),
			seriesprovider.IsUninstalledEQ(false),
		).All(ctx)
	if err != nil {
		return fmt.Errorf("query providers: %w", err)
	}

	riverClient := river.ClientFromContext[pgx.Tx](ctx)
	if riverClient == nil {
		return nil
	}

	enqueued := 0
	for _, sp := range providers {
		if sp.SuwayomiID == 0 {
			continue
		}
		// Check that the series isn't paused
		series, err := w.Deps.DB.Series.Get(ctx, sp.SeriesID)
		if err != nil || series.PauseDownloads {
			continue
		}
		if _, err := riverClient.Insert(ctx, GetChaptersArgs{ProviderID: sp.ID}, nil); err != nil {
			log.Warn().Err(err).Str("providerId", sp.ID.String()).Msg("failed to enqueue GetChapters")
		} else {
			enqueued++
		}
	}

	log.Info().Int("enqueued", enqueued).Int("providers", len(providers)).Msg("chapter refresh dispatched")
	return nil
}

// ============================================================
// RefreshAllLatestWorker — dispatches GetLatest for all enabled sources
// ============================================================

type RefreshAllLatestWorker struct {
	river.WorkerDefaults[RefreshAllLatestArgs]
	Deps *Deps
}

func (w *RefreshAllLatestWorker) Work(ctx context.Context, job *river.Job[RefreshAllLatestArgs]) error {
	// Fetch ALL installed sources from Suwayomi directly (not just ProviderStorage)
	// This ensures cloud-latest works even before any series are imported
	allSources, err := w.Deps.Suwayomi.GetSources(ctx)
	if err != nil {
		return fmt.Errorf("get suwayomi sources: %w", err)
	}

	riverClient := river.ClientFromContext[pgx.Tx](ctx)
	if riverClient == nil {
		return nil
	}

	enqueued := 0
	for _, src := range allSources {
		if !src.SupportsLatest {
			continue
		}
		if _, err := riverClient.Insert(ctx, GetLatestArgs{SourceID: src.ID}, nil); err != nil {
			log.Warn().Err(err).Str("sourceId", src.ID).Msg("failed to enqueue GetLatest")
		} else {
			enqueued++
		}
	}

	log.Info().Int("enqueued", enqueued).Int("sources", len(allSources)).Msg("latest refresh dispatched")
	return nil
}

// ============================================================
// DailyUpdateWorker — daily maintenance
// ============================================================

type DailyUpdateWorker struct {
	river.WorkerDefaults[DailyUpdateArgs]
	Deps *Deps
}

func (w *DailyUpdateWorker) Work(ctx context.Context, job *river.Job[DailyUpdateArgs]) error {
	log.Info().Msg("running daily maintenance")

	// Clean up old completed LatestSeries entries older than 30 days
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	deleted, err := w.Deps.DB.LatestSeries.Delete().
		Where(latestseries.FetchDateLT(cutoff)).
		Exec(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to clean up old latest series")
	} else if deleted > 0 {
		log.Info().Int("count", deleted).Msg("cleaned up old latest series entries")
	}

	// Clean up old source events older than 30 days
	seDeleted, err := w.Deps.DB.SourceEvent.Delete().
		Where(sourceevent.CreatedAtLT(cutoff)).
		Exec(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to clean up old source events")
	} else if seDeleted > 0 {
		log.Info().Int("count", seDeleted).Msg("cleaned up old source events")
	}

	// Note: PostgreSQL handles its own backups (unlike SQLite VACUUM INTO in .NET version)
	// The .NET version also cleaned up Suwayomi temp directory - that's handled by Suwayomi itself

	log.Info().Msg("daily maintenance complete")
	return nil
}

// ============================================================
// ScanLocalFilesWorker — scans local storage for archive files
// ============================================================

type ScanLocalFilesWorker struct {
	river.WorkerDefaults[ScanLocalFilesArgs]
	Deps *Deps
}

func (w *ScanLocalFilesWorker) Work(ctx context.Context, job *river.Job[ScanLocalFilesArgs]) error {
	scanPath := job.Args.Path
	jobID := fmt.Sprintf("scan-%d", job.ID)
	log.Info().Str("path", scanPath).Msg("scanning local files")

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeScanLocalFiles),
		int(types.ProgressStatusRunning), 0, "Scanning Directories...", nil)

	if _, err := os.Stat(scanPath); os.IsNotExist(err) {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeScanLocalFiles),
			int(types.ProgressStatusFailed), 100, "Directory not found", nil)
		return fmt.Errorf("directory not found: %s", scanPath)
	}

	scannedSeries, err := util.ScanDirectory(scanPath)
	if err != nil {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeScanLocalFiles),
			int(types.ProgressStatusFailed), 100, "Scan failed: "+err.Error(), nil)
		return fmt.Errorf("scan directory: %w", err)
	}

	if len(scannedSeries) == 0 {
		log.Info().Str("path", scanPath).Msg("no series directories with archives found")
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeScanLocalFiles),
			int(types.ProgressStatusCompleted), 100,
			fmt.Sprintf("No series found in %s", scanPath), nil)
		return nil
	}

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeScanLocalFiles),
		int(types.ProgressStatusRunning), 10,
		fmt.Sprintf("Found %d series, processing...", len(scannedSeries)), nil)

	// Get existing import paths
	existingImports, err := w.Deps.DB.ImportEntry.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("query existing imports: %w", err)
	}
	existingPaths := make(map[string]*ent.ImportEntry)
	for _, imp := range existingImports {
		existingPaths[strings.ToLower(imp.ID)] = imp
	}

	// Track which paths are still on disk
	scannedPaths := make(map[string]struct{})
	for _, info := range scannedSeries {
		scannedPaths[strings.ToLower(info.Path)] = struct{}{}
	}

	// Remove imports for folders that no longer exist (unless DoNotChange)
	for path, imp := range existingPaths {
		if _, exists := scannedPaths[path]; !exists && imp.Status != int(types.ImportStatusDoNotChange) {
			if err := w.Deps.DB.ImportEntry.DeleteOneID(imp.ID).Exec(ctx); err != nil {
				log.Warn().Err(err).Str("path", path).Msg("failed to remove stale import")
			}
		}
	}

	// Create/update imports
	for _, info := range scannedSeries {
		infoCopy := info
		pathKey := strings.ToLower(info.Path)

		if existing, ok := existingPaths[pathKey]; ok {
			// Update existing import — only update info, keep status
			_, err := w.Deps.DB.ImportEntry.UpdateOneID(existing.ID).
				SetInfo(&infoCopy).
				Save(ctx)
			if err != nil {
				log.Warn().Err(err).Str("path", info.Path).Msg("failed to update import")
			}
		} else {
			// Create new import
			err := w.Deps.DB.ImportEntry.Create().
				SetID(info.Path).
				SetTitle(info.Title).
				SetStatus(int(types.ImportStatusImport)).
				SetAction(int(types.ImportActionAdd)).
				SetInfo(&infoCopy).
				Exec(ctx)
			if err != nil {
				log.Warn().Err(err).Str("path", info.Path).Msg("failed to create import")
			}
		}
	}

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeScanLocalFiles),
		int(types.ProgressStatusCompleted), 100,
		fmt.Sprintf("Found %d series.", len(scannedSeries)), nil)

	log.Info().Int("count", len(scannedSeries)).Msg("scan complete")
	return nil
}

// ============================================================
// InstallExtensionsWorker — installs extensions needed by imports
// ============================================================

type InstallExtensionsWorker struct {
	river.WorkerDefaults[InstallExtensionsArgs]
	Deps *Deps
}

func (w *InstallExtensionsWorker) Work(ctx context.Context, job *river.Job[InstallExtensionsArgs]) error {
	jobID := fmt.Sprintf("install-ext-%d", job.ID)
	log.Info().Msg("installing required extensions for imports")

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeInstallAdditionalExtensions),
		int(types.ProgressStatusRunning), 0, "Checking required extensions...", nil)

	// Get all imports with status Import
	imports, err := w.Deps.DB.ImportEntry.Query().
		Where(importentry.StatusEQ(int(types.ImportStatusImport))).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query imports: %w", err)
	}

	// Extract unique provider names from imports
	providerNames := make(map[string]string) // name -> lang
	for _, imp := range imports {
		if imp.Info == nil {
			continue
		}
		for _, p := range imp.Info.Providers {
			if p.Provider != "" && p.Provider != "Unknown" {
				key := strings.ToLower(p.Provider + "|" + p.Language)
				if _, ok := providerNames[key]; !ok {
					providerNames[key] = p.Language
				}
			}
		}
	}

	if len(providerNames) == 0 {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeInstallAdditionalExtensions),
			int(types.ProgressStatusCompleted), 100, "No extensions needed.", nil)
		return nil
	}

	// Get installed extensions
	extensions, err := w.Deps.Suwayomi.GetExtensions(ctx)
	if err != nil {
		return fmt.Errorf("fetch extensions: %w", err)
	}

	// Find extensions that need installation
	var toInstall []suwayomi.SuwayomiExtension
	for _, ext := range extensions {
		if ext.Installed {
			continue
		}
		for key := range providerNames {
			parts := strings.SplitN(key, "|", 2)
			name := parts[0]
			lang := ""
			if len(parts) > 1 {
				lang = parts[1]
			}
			if strings.EqualFold(ext.Name, name) && (lang == "" || ext.Lang == lang || ext.Lang == "all") {
				toInstall = append(toInstall, ext)
				break
			}
		}
	}

	if len(toInstall) == 0 {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeInstallAdditionalExtensions),
			int(types.ProgressStatusCompleted), 100, "All extensions already installed.", nil)
		return nil
	}

	// Install each required extension
	step := 100.0 / float64(len(toInstall))
	acum := 0.0
	for _, ext := range toInstall {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeInstallAdditionalExtensions),
			int(types.ProgressStatusRunning), acum, ext.Name+" v"+fmt.Sprintf("%d", ext.VersionCode), nil)

		if err := w.Deps.Suwayomi.InstallExtension(ctx, ext.PkgName); err != nil {
			log.Warn().Err(err).Str("pkg", ext.PkgName).Msg("failed to install extension")
		} else {
			log.Info().Str("pkg", ext.PkgName).Msg("installed extension")
		}
		acum += step
	}

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeInstallAdditionalExtensions),
		int(types.ProgressStatusCompleted), 100, "Extensions installed successfully.", nil)

	log.Info().Int("installed", len(toInstall)).Msg("extension installation complete")
	return nil
}

// ============================================================
// SearchProvidersWorker — searches for imported series across sources
// ============================================================

type SearchProvidersWorker struct {
	river.WorkerDefaults[SearchProvidersArgs]
	Deps *Deps
}

func (w *SearchProvidersWorker) Work(ctx context.Context, job *river.Job[SearchProvidersArgs]) error {
	jobID := fmt.Sprintf("search-%d", job.ID)
	log.Info().Msg("searching for imported series across providers")

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeSearchProviders),
		int(types.ProgressStatusRunning), 0, "Starting series search...", nil)

	// Get all imports with status Import
	imports, err := w.Deps.DB.ImportEntry.Query().
		Where(importentry.StatusEQ(int(types.ImportStatusImport))).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query imports: %w", err)
	}

	if len(imports) == 0 {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeSearchProviders),
			int(types.ProgressStatusCompleted), 100, "No series to search, process complete", nil)
		return nil
	}

	// Get available sources
	sources, err := w.Deps.Suwayomi.GetSources(ctx)
	if err != nil {
		return fmt.Errorf("fetch sources: %w", err)
	}

	step := 100.0 / float64(len(imports))
	acum := 0.0

	for _, imp := range imports {
		if imp.Info == nil {
			acum += step
			continue
		}

		log.Info().Str("title", imp.Info.Title).Msg("searching for series")

		// Determine which languages to search
		langs := make(map[string]struct{})
		for _, p := range imp.Info.Providers {
			if p.Language != "" {
				langs[strings.ToLower(p.Language)] = struct{}{}
			}
		}
		if len(langs) == 0 {
			langs["en"] = struct{}{}
		}

		// Filter sources by language
		var filteredSources []suwayomi.SuwayomiSource
		for _, src := range sources {
			srcLang := strings.ToLower(src.Lang)
			if _, ok := langs[srcLang]; ok || srcLang == "all" {
				filteredSources = append(filteredSources, src)
			}
		}

		// Search for the series across filtered sources
		found := w.searchForSeries(ctx, imp.Info.Title, imp.Info.Providers, filteredSources)

		if len(found) > 0 {
			// Assign sequential importance (0 = highest priority)
			for i := range found {
				found[i].Importance = i
				found[i].IsSelected = i == 0
			}
			// Store search results in import
			seriesJSON := fullSeriesToRawJSON(found)
			_, err := w.Deps.DB.ImportEntry.UpdateOneID(imp.ID).
				SetSeries(seriesJSON).
				Save(ctx)
			if err != nil {
				log.Warn().Err(err).Str("path", imp.ID).Msg("failed to update import with search results")
			}

			providerList := make(map[string]struct{})
			for _, fs := range found {
				providerList[fs.Provider] = struct{}{}
			}
			provNames := make([]string, 0, len(providerList))
			for p := range providerList {
				provNames = append(provNames, p)
			}

			w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeSearchProviders),
				int(types.ProgressStatusRunning), acum,
				imp.Info.Title+" found in "+strings.Join(provNames, ",")+".", nil)
		} else {
			// Mark as skip
			_, err := w.Deps.DB.ImportEntry.UpdateOneID(imp.ID).
				SetStatus(int(types.ImportStatusSkip)).
				SetAction(int(types.ImportActionSkip)).
				Save(ctx)
			if err != nil {
				log.Warn().Err(err).Str("path", imp.ID).Msg("failed to update import status")
			}

			w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeSearchProviders),
				int(types.ProgressStatusRunning), acum,
				"Series "+imp.Title+" not found in available providers", nil)
		}

		acum += step
	}

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeSearchProviders),
		int(types.ProgressStatusCompleted), 100,
		fmt.Sprintf("Search completed for %d series", len(imports)), nil)

	log.Info().Int("count", len(imports)).Msg("provider search complete")
	return nil
}

// searchForSeries searches across sources for a series matching the given title and providers.
func (w *SearchProvidersWorker) searchForSeries(
	ctx context.Context,
	title string,
	providers []types.ProviderInfo,
	sources []suwayomi.SuwayomiSource,
) []types.FullSeries {
	var results []types.FullSeries

	// First pass: search using matched provider sources (by provider name)
	var matchedSources []suwayomi.SuwayomiSource
	var unmatchedSources []suwayomi.SuwayomiSource

	providerSet := make(map[string]struct{})
	for _, p := range providers {
		if p.Provider != "" && p.Provider != "Unknown" {
			providerSet[strings.ToLower(p.Provider)] = struct{}{}
		}
	}

	for _, src := range sources {
		if _, ok := providerSet[strings.ToLower(src.Name)]; ok {
			matchedSources = append(matchedSources, src)
		} else {
			unmatchedSources = append(unmatchedSources, src)
		}
	}

	// Search matched sources first
	if len(matchedSources) > 0 {
		results = w.searchSources(ctx, title, matchedSources)
	}

	// If nothing found in matched sources, try all remaining sources
	if len(results) == 0 && len(unmatchedSources) > 0 {
		results = w.searchSources(ctx, title, unmatchedSources)
	}

	return results
}

// searchSources searches for a series across multiple sources and returns augmented FullSeries.
func (w *SearchProvidersWorker) searchSources(
	ctx context.Context,
	title string,
	sources []suwayomi.SuwayomiSource,
) []types.FullSeries {
	type searchHit struct {
		series suwayomi.SuwayomiSeries
		source suwayomi.SuwayomiSource
	}

	var mu sync.Mutex
	var hits []searchHit
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, src := range sources {
		wg.Add(1)
		go func(source suwayomi.SuwayomiSource) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := w.Deps.Suwayomi.SearchSeries(ctx, source.ID, title, 1)
			if err != nil {
				log.Warn().Err(err).Str("source", source.Name).Msg("search error")
				return
			}
			if res == nil || len(res.MangaList) == 0 {
				return
			}

			// Find series with similar title
			for _, s := range res.MangaList {
				if areTitlesSimilar(s.Title, title, 0.1) {
					mu.Lock()
					hits = append(hits, searchHit{series: s, source: source})
					mu.Unlock()
					break // One match per source is enough
				}
			}
		}(src)
	}
	wg.Wait()

	if len(hits) == 0 {
		return nil
	}

	// Augment each hit with full series data and chapters
	var results []types.FullSeries
	for _, hit := range hits {
		fullData, err := w.Deps.Suwayomi.GetFullSeriesData(ctx, hit.series.ID, true)
		if err != nil {
			log.Warn().Err(err).Int("id", hit.series.ID).Msg("failed to fetch full data")
			continue
		}

		chapters, err := w.Deps.Suwayomi.GetChapters(ctx, hit.series.ID, true)
		if err != nil {
			log.Warn().Err(err).Int("id", hit.series.ID).Msg("failed to fetch chapters")
			continue
		}

		if len(chapters) == 0 {
			continue
		}

		// Group by scanlator
		groups := make(map[string][]suwayomi.SuwayomiChapter)
		for _, ch := range chapters {
			scanlator := hit.source.Name
			if ch.Scanlator != nil && *ch.Scanlator != "" {
				scanlator = *ch.Scanlator
			}
			groups[scanlator] = append(groups[scanlator], ch)
		}

		for scanlator, chs := range groups {
			sort.Slice(chs, func(i, j int) bool {
				return chs[i].Index < chs[j].Index
			})

			var convertedChapters []types.Chapter
			for _, ch := range chs {
				convertedChapters = append(convertedChapters, types.Chapter{
					Name:               ch.Name,
					Number:             ch.ChapterNumber,
					ProviderUploadDate: timeFromMillis(ch.UploadDate),
					URL:                ch.RealURL,
					ProviderIndex:      ch.Index,
					ShouldDownload:     true,
					PageCount:          &ch.PageCount,
				})
			}

			artist := ""
			if fullData.Artist != nil {
				artist = *fullData.Artist
			}
			author := ""
			if fullData.Author != nil {
				author = *fullData.Author
			}
			desc := ""
			if fullData.Description != nil {
				desc = *fullData.Description
			}

			idStr := fmt.Sprintf("%d", hit.series.ID)
			fs := types.FullSeries{
				ID:           idStr,
				ProviderID:   hit.source.ID,
				Provider:     hit.source.Name,
				Scanlator:    scanlator,
				Lang:         strings.ToLower(hit.source.Lang),
				ThumbnailURL: strPtr("serie/thumb/" + idStr),
				Title:        fullData.Title,
				Artist:       artist,
				Author:       author,
				Description:  desc,
				Genre:        fullData.Genre,
				ChapterCount: len(chs),
				URL:          fullData.RealURL,
				Meta:         fullData.Meta,
				Status:       types.SeriesStatus(fullData.Status),
				Chapters:     convertedChapters,
				IsSelected:   false,
			}

			if len(chs) > 0 {
				fs.LastUpdatedUTC = timeFromMillis(chs[0].UploadDate).Format(time.RFC3339)
			}

			fs.ChapterList = formatChapterRangesFromConverted(convertedChapters)
			fs.SuggestedFilename = util.MakeFolderNameSafe(fullData.Title)

			results = append(results, fs)
		}
	}

	return results
}

// ============================================================
// ImportSeriesWorker — imports scanned series into the library
// ============================================================

type ImportSeriesWorker struct {
	river.WorkerDefaults[ImportSeriesArgs]
	Deps *Deps
}

func (w *ImportSeriesWorker) Work(ctx context.Context, job *river.Job[ImportSeriesArgs]) error {
	jobID := fmt.Sprintf("import-%d", job.ID)
	disableDownloads := job.Args.DisableDownloads
	log.Info().Bool("disableDownloads", disableDownloads).Msg("importing series")

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeImportSeries),
		int(types.ProgressStatusRunning), 0, "Starting series import...", nil)

	imports, err := w.Deps.DB.ImportEntry.Query().
		Where(importentry.StatusNEQ(int(types.ImportStatusDoNotChange))).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query imports: %w", err)
	}

	if len(imports) == 0 {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeImportSeries),
			int(types.ProgressStatusCompleted), 100, "No series to import.", nil)
		return nil
	}

	step := 100.0 / float64(len(imports))
	acum := 0.0

	for _, imp := range imports {
		if imp.Action != int(types.ImportActionAdd) || len(imp.Series) == 0 {
			acum += step
			continue
		}

		// Convert raw series JSON back to typed FullSeries
		fullSeries := rawJSONToFullSeries(imp.Series)
		var selected []types.FullSeries
		for _, fs := range fullSeries {
			if fs.IsSelected {
				selected = append(selected, fs)
			}
		}

		if len(selected) == 0 {
			acum += step
			continue
		}

		// Create the series in the database
		seriesID, err := w.createSeriesFromImport(ctx, imp, selected, disableDownloads)
		if err != nil {
			log.Warn().Err(err).Str("title", imp.Title).Msg("failed to import series")
			acum += step
			continue
		}

		// Save kaizoku.json to the series directory
		w.saveKaizokuJSON(ctx, seriesID)

		// Mark import entry as already imported (DoNotChange) so it doesn't show in Add tab on re-run
		_, _ = w.Deps.DB.ImportEntry.UpdateOneID(imp.ID).
			SetStatus(int(types.ImportStatusDoNotChange)).
			SetAction(int(types.ImportActionSkip)).
			Save(ctx)

		acum += step
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeImportSeries),
			int(types.ProgressStatusRunning), acum, imp.Title+" imported.", nil)
	}

	// Mark wizard as complete
	w.Deps.DB.Setting.Create().
		SetID("IsWizardSetupComplete").
		SetValue("true").
		OnConflictColumns("id").UpdateNewValues().
		Exec(ctx)
	w.Deps.DB.Setting.Create().
		SetID("WizardSetupStepCompleted").
		SetValue("0").
		OnConflictColumns("id").UpdateNewValues().
		Exec(ctx)

	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeImportSeries),
		int(types.ProgressStatusCompleted), 100,
		fmt.Sprintf("Import completed for %d series", len(imports)), nil)

	log.Info().Int("count", len(imports)).Msg("series import complete")
	return nil
}

func (w *ImportSeriesWorker) createSeriesFromImport(
	ctx context.Context,
	imp *ent.ImportEntry,
	selected []types.FullSeries,
	disableDownloads bool,
) (uuid.UUID, error) {
	// Consolidate series metadata from selected providers
	consolidated := consolidateForImport(selected)

	storagePath := imp.ID // Path is the ID for ImportEntry

	dbSeries, err := w.Deps.DB.Series.Create().
		SetTitle(consolidated.Title).
		SetDescription(consolidated.Description).
		SetThumbnailURL(derefStrDefault(consolidated.ThumbnailURL, "")).
		SetArtist(consolidated.Artist).
		SetAuthor(consolidated.Author).
		SetGenre(consolidated.Genre).
		SetStatus(string(consolidated.Status)).
		SetStoragePath(storagePath).
		SetNillableType(consolidated.Type).
		SetChapterCount(consolidated.ChapterCount).
		SetPauseDownloads(disableDownloads).
		Save(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create series: %w", err)
	}

	// Create providers
	var createdProviderIDs []uuid.UUID
	for _, fs := range selected {
		suwayomiID, _ := strconv.Atoi(fs.ID)

		create := w.Deps.DB.SeriesProvider.Create().
			SetSeriesID(dbSeries.ID).
			SetSuwayomiID(suwayomiID).
			SetProvider(fs.Provider).
			SetScanlator(fs.Scanlator).
			SetTitle(fs.Title).
			SetLanguage(fs.Lang).
			SetNillableURL(fs.URL).
			SetNillableThumbnailURL(fs.ThumbnailURL).
			SetArtist(fs.Artist).
			SetAuthor(fs.Author).
			SetDescription(fs.Description).
			SetGenre(fs.Genre).
			SetStatus(string(fs.Status)).
			SetImportance(fs.Importance).
			SetIsTitle(fs.UseTitle).
			SetIsCover(fs.UseCover).
			SetIsUnknown(fs.IsUnknown).
			SetFetchDate(time.Now().UTC())

		if fs.ContinueAfterChapter != nil {
			create = create.SetContinueAfterChapter(*fs.ContinueAfterChapter)
		}
		cc := int64(fs.ChapterCount)
		if cc > 0 {
			create = create.SetChapterCount(cc)
		}
		if len(fs.Chapters) > 0 {
			create = create.SetChapters(fs.Chapters)
		}

		p, err := create.Save(ctx)
		if err != nil {
			log.Warn().Err(err).Str("provider", fs.Provider).Msg("failed to create provider for import")
			continue
		}
		createdProviderIDs = append(createdProviderIDs, p.ID)
	}

	// Match on-disk files to provider chapters
	storageFolder := w.Deps.Config.Storage.Folder
	seriesDir := filepath.Join(storageFolder, storagePath)
	w.matchOnDiskFiles(ctx, dbSeries.ID, seriesDir)

	// Run mandatory post-import verify to ensure clean state
	log.Info().Str("series", dbSeries.Title).Msg("running post-import integrity verification")
	w.Deps.VerifySeriesIntegrity(ctx, dbSeries.ID, storageFolder)

	// Enqueue GetChapters for non-disabled, non-unknown providers
	if !disableDownloads {
		for _, provID := range createdProviderIDs {
			sp, err := w.Deps.DB.SeriesProvider.Get(ctx, provID)
			if err != nil {
				continue
			}
			if sp.IsUnknown || sp.IsDisabled {
				continue
			}
			riverClient := river.ClientFromContext[pgx.Tx](ctx)
			if riverClient != nil {
				if _, err := riverClient.Insert(ctx, GetChaptersArgs{ProviderID: provID}, nil); err != nil {
					log.Warn().Err(err).Str("providerId", provID.String()).Msg("failed to enqueue GetChapters job")
				}
			}
		}
	}

	return dbSeries.ID, nil
}

// matchOnDiskFiles scans the series directory for archive files and matches them
// to provider chapter records by comparing provider name, scanlator, language, and chapter number.
func (w *ImportSeriesWorker) matchOnDiskFiles(ctx context.Context, seriesID uuid.UUID, seriesDir string) {
	entries, err := os.ReadDir(seriesDir)
	if err != nil {
		log.Debug().Err(err).Str("dir", seriesDir).Msg("import: cannot read series directory for file matching")
		return
	}

	// Parse all archive files on disk
	type matchableFile struct {
		util.DetectedChapter
		matched bool
	}
	var onDiskFiles []*matchableFile
	for _, entry := range entries {
		if entry.IsDir() || !util.IsArchive(entry.Name()) {
			continue
		}
		parsed := util.ParseArchiveFilename(entry.Name())
		onDiskFiles = append(onDiskFiles, &matchableFile{DetectedChapter: parsed})
	}

	if len(onDiskFiles) == 0 {
		return
	}

	// Load all providers for this series
	providers, err := w.Deps.DB.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(seriesID)).
		All(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("import: failed to load providers for file matching")
		return
	}

	now := time.Now().UTC()

	for _, p := range providers {
		provName := strings.ToLower(p.Provider)
		provScanlator := strings.ToLower(p.Scanlator)
		provLang := strings.ToLower(p.Language)

		changed := false
		for _, file := range onDiskFiles {
			if file.matched {
				continue
			}
			fileProv := strings.ToLower(file.Provider)
			fileScanlator := strings.ToLower(file.Scanlator)
			fileLang := strings.ToLower(file.Language)

			// Match by provider+scanlator+language
			if fileProv != provName {
				continue
			}
			if provScanlator != "" && fileScanlator != provScanlator {
				continue
			}
			if fileLang != provLang {
				continue
			}
			if file.ChapterNumber == nil {
				continue
			}

			// Find matching chapter in provider's chapter list
			for i := range p.Chapters {
				ch := &p.Chapters[i]
				if ch.Number == nil || *ch.Number != *file.ChapterNumber {
					continue
				}
				// Verify file actually exists and is valid
				archivePath := filepath.Join(seriesDir, file.Filename)
				if util.CheckArchive(archivePath) == types.ArchiveResultFine {
					ch.Filename = file.Filename
					ch.DownloadDate = &now
					ch.ShouldDownload = false
					ch.IsDeleted = false
					file.matched = true
					changed = true
				}
				break
			}
		}

		if changed {
			// Recalculate ContinueAfterChapter from actual downloaded chapters
			var maxDownloaded *float64
			for _, ch := range p.Chapters {
				if ch.Filename != "" && ch.Number != nil {
					if maxDownloaded == nil || *ch.Number > *maxDownloaded {
						n := *ch.Number
						maxDownloaded = &n
					}
				}
			}

			update := w.Deps.DB.SeriesProvider.UpdateOneID(p.ID).
				SetChapters(p.Chapters)
			if maxDownloaded != nil {
				update = update.SetContinueAfterChapter(*maxDownloaded)
			} else {
				update = update.SetContinueAfterChapter(0)
			}
			if err := update.Exec(ctx); err != nil {
				log.Warn().Err(err).Str("provider", p.Provider).Msg("import: failed to update provider after file matching")
			}
		}
	}

	// Handle unmatched files — create Unknown providers for orphan archives
	type unknownKey struct {
		provider  string
		scanlator string
		language  string
	}
	unmatchedGroups := make(map[unknownKey][]types.Chapter)
	unmatchedArchives := make(map[unknownKey][]types.ArchiveInfo)

	for _, file := range onDiskFiles {
		if file.matched {
			continue
		}
		provider := file.Provider
		if provider == "" {
			provider = "Unknown"
		}
		key := unknownKey{
			provider:  provider,
			scanlator: file.Scanlator,
			language:  file.Language,
		}
		ch := types.Chapter{
			Name:       file.ChapterName,
			Number:     file.ChapterNumber,
			Filename:   file.Filename,
			IsDeleted:  false,
			DownloadDate: &now,
		}
		unmatchedGroups[key] = append(unmatchedGroups[key], ch)
		unmatchedArchives[key] = append(unmatchedArchives[key], types.ArchiveInfo{
			Filename:      file.Filename,
			ChapterName:   file.ChapterName,
			ChapterNumber: file.ChapterNumber,
		})
	}

	for key, chapters := range unmatchedGroups {
		lang := key.language
		if lang == "" {
			lang = "en"
		}
		_, err := w.Deps.DB.SeriesProvider.Create().
			SetSeriesID(seriesID).
			SetProvider(key.provider).
			SetScanlator(key.scanlator).
			SetLanguage(lang).
			SetTitle("Unknown").
			SetIsUnknown(true).
			SetChapters(chapters).
			SetChapterCount(int64(len(chapters))).
			SetFetchDate(now).
			Save(ctx)
		if err != nil {
			log.Warn().Err(err).Str("provider", key.provider).Msg("import: failed to create unknown provider for unmatched files")
		}
	}

	log.Info().
		Int("onDisk", len(onDiskFiles)).
		Int("unmatchedGroups", len(unmatchedGroups)).
		Msg("import: file-to-chapter matching complete")
}

func (w *ImportSeriesWorker) saveKaizokuJSON(ctx context.Context, seriesID uuid.UUID) {
	if err := saveSeriesKaizokuJSON(ctx, w.Deps.DB, seriesID, w.Deps.Config.Storage.Folder); err != nil {
		log.Warn().Err(err).Str("seriesId", seriesID.String()).Msg("failed to save kaizoku.json")
	}
}

// --- Import helpers ---

func consolidateForImport(series []types.FullSeries) types.FullSeries {
	if len(series) == 0 {
		return types.FullSeries{Status: types.SeriesStatusUnknown}
	}
	// Sort by importance so the most important provider is the base
	sort.Slice(series, func(i, j int) bool {
		return series[i].Importance < series[j].Importance
	})
	best := series[0]
	for _, fs := range series[1:] {
		if fs.UseCover && fs.ThumbnailURL != nil {
			best.ThumbnailURL = fs.ThumbnailURL
		}
		if fs.Description != "" && best.Description == "" {
			best.Description = fs.Description
		}
		if fs.Author != "" && best.Author == "" {
			best.Author = fs.Author
		}
		if fs.Artist != "" && best.Artist == "" {
			best.Artist = fs.Artist
		}
		if fs.ChapterCount > best.ChapterCount {
			best.ChapterCount = fs.ChapterCount
		}
	}
	return best
}

func derefStrDefault(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}

// fullSeriesToRawJSON converts []types.FullSeries to []map[string]interface{} for Ent storage.
func fullSeriesToRawJSON(series []types.FullSeries) []map[string]interface{} {
	data, err := json.Marshal(series)
	if err != nil {
		return nil
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// rawJSONToFullSeries converts []map[string]interface{} from Ent back to []types.FullSeries.
func rawJSONToFullSeries(raw []map[string]interface{}) []types.FullSeries {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var result []types.FullSeries
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// areTitlesSimilar compares two titles for similarity using normalized Levenshtein distance.
func areTitlesSimilar(a, b string, threshold float64) bool {
	na := normTitle(a)
	nb := normTitle(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	dist := levenshteinDist(na, nb)
	maxLen := len(na)
	if len(nb) > maxLen {
		maxLen = len(nb)
	}
	if maxLen == 0 {
		return true
	}
	return float64(dist)/float64(maxLen) <= threshold
}

func normTitle(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			return r
		}
		return -1
	}, s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func levenshteinDist(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}
	prev := make([]int, len(s2)+1)
	curr := make([]int, len(s2)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(s1); i++ {
		curr[0] = i
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[len(s2)]
}

func timeFromMillis(ms int64) *time.Time {
	if ms == 0 {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return &t
}

func formatChapterRangesFromConverted(chapters []types.Chapter) string {
	var nums []float64
	for _, ch := range chapters {
		if ch.Number != nil {
			nums = append(nums, *ch.Number)
		}
	}
	if len(nums) == 0 {
		return ""
	}
	sort.Float64s(nums)

	var ranges []string
	start := nums[0]
	end := nums[0]
	for i := 1; i < len(nums); i++ {
		if nums[i]-end <= 1.1 {
			end = nums[i]
		} else {
			ranges = append(ranges, fmtRange(start, end))
			start = nums[i]
			end = nums[i]
		}
	}
	ranges = append(ranges, fmtRange(start, end))
	return strings.Join(ranges, ", ")
}

func fmtRange(start, end float64) string {
	if start == end {
		return fmtNum(start)
	}
	return fmtNum(start) + "-" + fmtNum(end)
}

func fmtNum(n float64) string {
	if n == float64(int(n)) {
		return strconv.Itoa(int(n))
	}
	return strconv.FormatFloat(n, 'f', 1, 64)
}

// --- Helpers ---

func latestChapter(chapters []suwayomi.SuwayomiChapter) *suwayomi.SuwayomiChapter {
	if len(chapters) == 0 {
		return nil
	}
	sorted := make([]suwayomi.SuwayomiChapter, len(chapters))
	copy(sorted, chapters)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Index > sorted[j].Index
	})
	return &sorted[0]
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefIntDefault(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// saveSeriesKaizokuJSON loads a series with providers and writes kaizoku.json to its storage directory.
func saveSeriesKaizokuJSON(ctx context.Context, db *ent.Client, seriesID uuid.UUID, storageFolder string) error {
	s, err := db.Series.Get(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("load series: %w", err)
	}

	providers, err := db.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(seriesID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("load providers: %w", err)
	}

	info := buildKaizokuInfo(s, providers)
	seriesDir := filepath.Join(storageFolder, s.StoragePath)
	return util.SaveKaizokuJSON(seriesDir, &info)
}

// buildKaizokuInfo creates a KaizokuInfo struct from a Series and its providers.
func buildKaizokuInfo(s *ent.Series, providers []*ent.SeriesProvider) types.KaizokuInfo {
	now := time.Now().UTC()
	info := types.KaizokuInfo{
		Title:          s.Title,
		Status:         types.SeriesStatus(s.Status),
		Artist:         s.Artist,
		Author:         s.Author,
		Description:    s.Description,
		Genre:          s.Genre,
		Type:           derefStrDefault(s.Type, ""),
		ChapterCount:   s.ChapterCount,
		LastUpdatedUTC: &now,
		IsDisabled:     s.PauseDownloads,
		KaizokuVersion: 1,
		Path:           s.StoragePath,
	}

	for _, sp := range providers {
		pi := types.ProviderInfo{
			Provider:     sp.Provider,
			Language:     sp.Language,
			Scanlator:    sp.Scanlator,
			Title:        sp.Title,
			ThumbnailURL: derefStrDefault(sp.ThumbnailURL, ""),
			Status:       types.SeriesStatus(sp.Status),
			Importance:   sp.Importance,
			IsDisabled:   sp.IsDisabled,
			ChapterCount: len(sp.Chapters),
			ChapterList:  buildChapterRanges(sp.Chapters),
		}

		for _, ch := range sp.Chapters {
			if ch.Filename != "" && !ch.IsDeleted {
				pi.Archives = append(pi.Archives, types.ArchiveInfo{
					Filename:      ch.Filename,
					ChapterName:   ch.Name,
					ChapterNumber: ch.Number,
				})
			}
		}

		info.Providers = append(info.Providers, pi)
	}

	return info
}

// VerifySeriesIntegrity performs comprehensive integrity verification for a single series.
// It checks files against DB, fixes DB records for missing/bad files, detects orphans,
// recalculates ContinueAfterChapter, regenerates kaizoku.json, and enqueues re-downloads.
func (d *Deps) VerifySeriesIntegrity(ctx context.Context, seriesID uuid.UUID, storageFolder string) types.SeriesIntegrityResult {
	result := types.SeriesIntegrityResult{
		BadFiles:    []types.ArchiveIntegrityResult{},
		OrphanFiles: []string{},
	}

	s, err := d.DB.Series.Get(ctx, seriesID)
	if err != nil {
		log.Error().Err(err).Str("seriesId", seriesID.String()).Msg("verify: failed to load series")
		return result
	}

	if s.StoragePath == "" {
		result.Success = true
		return result
	}

	seriesDir := filepath.Join(storageFolder, s.StoragePath)

	providers, err := d.DB.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(seriesID)).
		All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("verify: failed to load providers")
		return result
	}

	// Remove empty unknown providers
	for _, p := range providers {
		if p.IsUnknown && allChaptersEmpty(p.Chapters) {
			_ = d.DB.SeriesProvider.DeleteOneID(p.ID).Exec(ctx)
		}
	}

	// Reload providers after cleanup
	providers, err = d.DB.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(seriesID)).
		All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("verify: failed to reload providers")
		return result
	}

	// Build set of ALL tracked filenames across all providers
	trackedFiles := make(map[string]bool)
	for _, p := range providers {
		for _, ch := range p.Chapters {
			if ch.Filename != "" && !ch.IsDeleted {
				trackedFiles[ch.Filename] = true
			}
		}
	}

	// Check each tracked chapter against disk
	affectedProviderIDs := make(map[uuid.UUID]bool)
	for _, p := range providers {
		changed := false
		for i := range p.Chapters {
			ch := &p.Chapters[i]
			if ch.Filename == "" || ch.IsDeleted {
				continue
			}
			archivePath := filepath.Join(seriesDir, ch.Filename)
			archiveResult := util.CheckArchive(archivePath)
			if archiveResult != types.ArchiveResultFine {
				result.BadFiles = append(result.BadFiles, types.ArchiveIntegrityResult{
					Filename: ch.Filename,
					Result:   archiveResult,
				})

				// Delete corrupt files from disk (not just missing ones)
				if archiveResult == types.ArchiveResultNoImages || archiveResult == types.ArchiveResultNotAnArchive {
					if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
						log.Warn().Err(err).Str("file", archivePath).Msg("verify: failed to delete bad archive")
					}
				}

				// Fix DB record: clear filename, mark for re-download
				delete(trackedFiles, ch.Filename)
				ch.Filename = ""
				ch.DownloadDate = nil
				ch.IsDeleted = false
				ch.ShouldDownload = true
				result.MissingFiles++
				result.FixedCount++
				changed = true
			}
		}

		if changed {
			// Recalculate ContinueAfterChapter from actual downloaded chapters
			var maxDownloaded *float64
			for _, ch := range p.Chapters {
				if ch.Filename != "" && ch.Number != nil {
					if maxDownloaded == nil || *ch.Number > *maxDownloaded {
						n := *ch.Number
						maxDownloaded = &n
					}
				}
			}

			update := d.DB.SeriesProvider.UpdateOneID(p.ID).
				SetChapters(p.Chapters)
			if maxDownloaded != nil {
				update = update.SetContinueAfterChapter(*maxDownloaded)
			} else {
				update = update.SetContinueAfterChapter(0)
			}
			if err := update.Exec(ctx); err != nil {
				log.Warn().Err(err).Str("provider", p.Provider).Msg("verify: failed to update provider")
			}

			affectedProviderIDs[p.ID] = true
		}
	}

	// Scan directory for orphan files (on disk but not tracked in DB)
	entries, err := os.ReadDir(seriesDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if !util.IsArchive(entry.Name()) {
				continue
			}
			if !trackedFiles[entry.Name()] {
				result.OrphanFiles = append(result.OrphanFiles, entry.Name())
			}
		}
	}

	// Regenerate kaizoku.json from corrected DB state
	if err := saveSeriesKaizokuJSON(ctx, d.DB, seriesID, storageFolder); err != nil {
		log.Warn().Err(err).Msg("verify: failed to regenerate kaizoku.json")
	}

	// Enqueue GetChapters for affected providers to trigger re-downloads
	for provID := range affectedProviderIDs {
		sp, err := d.DB.SeriesProvider.Get(ctx, provID)
		if err != nil {
			continue
		}
		// Only re-download for active, non-unknown providers
		if sp.IsDisabled || sp.IsUnknown || sp.IsUninstalled {
			continue
		}
		if _, err := d.enqueueGetChapters(ctx, provID); err != nil {
			log.Warn().Err(err).Str("providerId", provID.String()).Msg("verify: failed to enqueue GetChapters")
		} else {
			result.RedownloadQueued++
		}
	}

	result.Success = len(result.BadFiles) == 0 && result.MissingFiles == 0
	return result
}

// enqueueGetChapters inserts a GetChapters job for the given provider.
func (d *Deps) enqueueGetChapters(ctx context.Context, providerID uuid.UUID) (bool, error) {
	if d.RiverClient == nil {
		return false, fmt.Errorf("river client not available")
	}
	_, err := d.RiverClient.Insert(ctx, GetChaptersArgs{ProviderID: providerID}, nil)
	if err != nil {
		return false, err
	}
	return true, nil
}

func allChaptersEmpty(chapters []types.Chapter) bool {
	for _, ch := range chapters {
		if ch.Filename != "" {
			return false
		}
	}
	return true
}

// ============================================================
// VerifyAllSeriesWorker — verifies all series in the library
// ============================================================

type VerifyAllSeriesWorker struct {
	river.WorkerDefaults[VerifyAllSeriesArgs]
	Deps *Deps
}

func (w *VerifyAllSeriesWorker) Work(ctx context.Context, j *river.Job[VerifyAllSeriesArgs]) error {
	jobID := fmt.Sprintf("verify-all-%d", j.ID)
	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeVerifyAll),
		int(types.ProgressStatusRunning), 0, "Starting library verification...", nil)

	storageFolder := w.Deps.Config.Storage.Folder

	allSeries, err := w.Deps.DB.Series.Query().All(ctx)
	if err != nil {
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeVerifyAll),
			int(types.ProgressStatusFailed), 0, "Failed to load series", nil)
		return fmt.Errorf("load all series: %w", err)
	}

	total := len(allSeries)
	totalBadFiles := 0
	totalMissing := 0
	totalOrphans := 0
	totalFixed := 0

	for i, s := range allSeries {
		pct := float64(i+1) / float64(total) * 100
		w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeVerifyAll),
			int(types.ProgressStatusRunning), pct,
			fmt.Sprintf("Verifying %s (%d/%d)", s.Title, i+1, total), nil)

		result := w.Deps.VerifySeriesIntegrity(ctx, s.ID, storageFolder)
		totalBadFiles += len(result.BadFiles)
		totalMissing += result.MissingFiles
		totalOrphans += len(result.OrphanFiles)
		totalFixed += result.FixedCount
	}

	msg := fmt.Sprintf("Verified %d series: %d bad files, %d missing, %d orphans, %d fixed",
		total, totalBadFiles, totalMissing, totalOrphans, totalFixed)
	w.Deps.Progress.BroadcastProgress(jobID, int(types.JobTypeVerifyAll),
		int(types.ProgressStatusCompleted), 100, msg, nil)

	return nil
}

// buildChapterRanges creates StartStop ranges from a list of downloaded chapters.
func buildChapterRanges(chapters []types.Chapter) []types.StartStop {
	var nums []float64
	for _, ch := range chapters {
		if ch.Number != nil && ch.Filename != "" && !ch.IsDeleted {
			nums = append(nums, *ch.Number)
		}
	}
	if len(nums) == 0 {
		return nil
	}
	sort.Float64s(nums)

	var ranges []types.StartStop
	start := nums[0]
	end := nums[0]
	for i := 1; i < len(nums); i++ {
		if nums[i]-end <= 1.1 {
			end = nums[i]
		} else {
			ranges = append(ranges, types.StartStop{Start: start, Stop: end})
			start = nums[i]
			end = nums[i]
		}
	}
	ranges = append(ranges, types.StartStop{Start: start, Stop: end})
	return ranges
}
