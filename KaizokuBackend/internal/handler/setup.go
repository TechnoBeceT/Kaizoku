package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/ent/importentry"
	"github.com/technobecet/kaizoku-go/internal/job"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
	"github.com/technobecet/kaizoku-go/internal/types"
	"github.com/technobecet/kaizoku-go/internal/util"
)

type SetupHandler struct {
	config   *config.Config
	db       *ent.Client
	suwayomi *suwayomi.Client
	river    riverClient
}

// ScanLocalFiles enqueues a job to scan the storage directory.
// POST /api/setup/scan
func (h *SetupHandler) ScanLocalFiles(c echo.Context) error {
	ctx := c.Request().Context()

	storageFolder := h.config.Storage.Folder
	if storageFolder == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "storage folder not configured"})
	}

	_, err := h.river.Insert(ctx, job.ScanLocalFilesArgs{Path: storageFolder}, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to enqueue ScanLocalFiles job")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enqueue job"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "message": "Scan job queued"})
}

// InstallExtensions enqueues a job to install required extensions.
// POST /api/setup/install-extensions
func (h *SetupHandler) InstallExtensions(c echo.Context) error {
	ctx := c.Request().Context()

	_, err := h.river.Insert(ctx, job.InstallExtensionsArgs{}, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to enqueue InstallExtensions job")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enqueue job"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "message": "Install extensions job queued"})
}

// SearchProviders enqueues a job to search for series across providers.
// POST /api/setup/search
func (h *SetupHandler) SearchProviders(c echo.Context) error {
	ctx := c.Request().Context()

	_, err := h.river.Insert(ctx, job.SearchProvidersArgs{}, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to enqueue SearchProviders job")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enqueue job"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "message": "Search providers job queued"})
}

// AugmentImport augments an import with full series data from search results.
// POST /api/setup/augment?path=<encoded_path>
// Body: LinkedSeries[] (JSON array)
func (h *SetupHandler) AugmentImport(c echo.Context) error {
	pathParam := c.QueryParam("path")
	if pathParam == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "path is required"})
	}

	var linkedSeries []types.LinkedSeries
	if err := json.NewDecoder(c.Request().Body).Decode(&linkedSeries); err != nil {
		linkedSeries = nil
	}

	ctx := c.Request().Context()

	imp, err := h.db.ImportEntry.Get(ctx, pathParam)
	if err != nil {
		if ent.IsNotFound(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "import not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	if len(linkedSeries) > 0 {
		// Fetch full data + chapters for each selected series (parallel)
		// Use indexed slots to preserve the original linkedSeries order.
		type augResult struct {
			linked  types.LinkedSeries
			details *suwayomi.SuwayomiSeries
		}
		results := make([]augResult, len(linkedSeries))
		sem := make(chan struct{}, 10)
		var wg sync.WaitGroup

		for idx, ls := range linkedSeries {
			wg.Add(1)
			go func(idx int, ls types.LinkedSeries) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				id, err := strconv.Atoi(ls.ID)
				if err != nil {
					return
				}
				full, err := h.suwayomi.GetFullSeriesData(ctx, id, true)
				if err != nil {
					log.Warn().Err(err).Str("id", ls.ID).Msg("augment: failed to fetch full data")
					return
				}
				chapters, err := h.suwayomi.GetChapters(ctx, id, true)
				if err != nil {
					log.Warn().Err(err).Str("id", ls.ID).Msg("augment: failed to fetch chapters")
					return
				}
				if full != nil {
					for i := range chapters {
						if chapters[i].Scanlator == nil || *chapters[i].Scanlator == "" {
							s := ls.Provider
							chapters[i].Scanlator = &s
						}
					}
					full.Chapters = chapters
					results[idx] = augResult{linked: ls, details: full}
				}
			}(idx, ls)
		}
		wg.Wait()

		// Filter out empty results (failed fetches)
		var fetched []augResult
		for _, r := range results {
			if r.details != nil {
				fetched = append(fetched, r)
			}
		}

		// Build FullSeries grouped by scanlator (same as SearchProvidersWorker)
		var fullSeriesList []types.FullSeries
		importance := 0
		for _, item := range fetched {
			details := item.details
			ls := item.linked

			groups := make(map[string][]suwayomi.SuwayomiChapter)
			for _, ch := range details.Chapters {
				scanlator := ls.Provider
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

				artist, author, desc := "", "", ""
				if details.Artist != nil {
					artist = *details.Artist
				}
				if details.Author != nil {
					author = *details.Author
				}
				if details.Description != nil {
					desc = *details.Description
				}

				fs := types.FullSeries{
					ID:           ls.ID,
					ProviderID:   ls.ProviderID,
					Provider:     ls.Provider,
					Scanlator:    scanlator,
					Lang:         ls.Lang,
					ThumbnailURL: strPtr("serie/thumb/" + ls.ID),
					Title:        details.Title,
					Artist:       artist,
					Author:       author,
					Description:  desc,
					Genre:        details.Genre,
					ChapterCount: len(chs),
					URL:          details.RealURL,
					Meta:         details.Meta,
					UseCover:     ls.UseCover,
					Importance:   importance,
					Status:       types.SeriesStatus(details.Status),
					Chapters:     convertedChapters,
					IsSelected:   importance == 0,
				}
				if len(chs) > 0 {
					fs.LastUpdatedUTC = timeFromMillis(chs[0].UploadDate).UTC().Format(time.RFC3339)
				}
				// Format chapter list ranges
				var chNums []float64
				for _, ch := range convertedChapters {
					if ch.Number != nil {
						chNums = append(chNums, *ch.Number)
					}
				}
				fs.ChapterList = formatDecimalRanges(chNums)

				fullSeriesList = append(fullSeriesList, fs)
				importance++
			}
		}

		// Convert to raw JSON and store
		seriesJSON, err := json.Marshal(fullSeriesList)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to marshal series"})
		}
		var rawSeries []map[string]interface{}
		if err := json.Unmarshal(seriesJSON, &rawSeries); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to process series"})
		}

		imp, err = h.db.ImportEntry.UpdateOneID(imp.ID).
			SetSeries(rawSeries).
			Save(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update import"})
		}
	}

	return c.JSON(http.StatusOK, importEntryToInfo(imp))
}

func timeFromMillis(ms int64) *time.Time {
	t := time.UnixMilli(ms).UTC()
	return &t
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

// UpdateImport updates an import's selection before final import.
// The frontend sends SmallSeries (user-editable fields only). We merge those
// changes into the existing FullSeries data so we don't lose chapters, metadata, etc.
// POST /api/setup/update
func (h *SetupHandler) UpdateImport(c echo.Context) error {
	var req types.ImportInfo
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Path == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "path is required"})
	}

	ctx := c.Request().Context()

	// Load the existing import entry to get the stored FullSeries data
	existing, err := h.db.ImportEntry.Get(ctx, req.Path)
	if err != nil {
		if ent.IsNotFound(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "import not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	update := h.db.ImportEntry.UpdateOneID(req.Path).
		SetStatus(int(req.Status)).
		SetAction(int(req.Action))

	if req.ContinueAfterChapter != nil {
		update = update.SetContinueAfterChapter(*req.ContinueAfterChapter)
	}

	// Merge SmallSeries user changes into existing FullSeries data
	if len(req.Series) > 0 && len(existing.Series) > 0 {
		fullList := rawJSONToFullSeries(existing.Series)
		if fullList != nil {
			// Build lookup: key = id+scanlator → SmallSeries
			smallByKey := make(map[string]types.SmallSeries, len(req.Series))
			for _, ss := range req.Series {
				key := ss.ID + "|" + ss.Scanlator
				smallByKey[key] = ss
			}

			// Apply user-editable fields from SmallSeries onto FullSeries.
			// The frontend uses "preferred" as the selection toggle (isSelected isn't in the TS type),
			// so we map preferred → IsSelected for the import worker.
			for i := range fullList {
				key := fullList[i].ID + "|" + fullList[i].Scanlator
				if ss, ok := smallByKey[key]; ok {
					fullList[i].Importance = ss.Importance
					fullList[i].UseCover = ss.UseCover
					fullList[i].UseTitle = ss.UseTitle
					fullList[i].IsSelected = ss.Preferred
				}
			}

			// Sort by importance to match user's intended order
			sort.Slice(fullList, func(i, j int) bool {
				return fullList[i].Importance < fullList[j].Importance
			})

			rawSeries := fullSeriesToRawJSON(fullList)
			if rawSeries != nil {
				update = update.SetSeries(rawSeries)
			}
		}
	}

	imp, err := update.Save(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, importEntryToInfo(imp))
}

// ImportSeries enqueues a job to import selected series into the library.
// POST /api/setup/import
func (h *SetupHandler) ImportSeries(c echo.Context) error {
	ctx := c.Request().Context()

	disableDownloads := c.QueryParam("disableDownloads") == "true"

	_, err := h.river.Insert(ctx, job.ImportSeriesArgs{DisableDownloads: disableDownloads}, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to enqueue ImportSeries job")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enqueue job"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "message": "Import job queued"})
}

// GetImports returns all import entries.
// GET /api/setup/imports
func (h *SetupHandler) GetImports(c echo.Context) error {
	ctx := c.Request().Context()

	imports, err := h.db.ImportEntry.Query().All(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Build set of existing series storage paths to detect already-imported.
	// Normalize paths so .NET-style (Unicode replacements) and Go-style paths match.
	allSeries, _ := h.db.Series.Query().All(ctx)
	existingPaths := make(map[string]bool)
	for _, s := range allSeries {
		if s.StoragePath != "" {
			existingPaths[util.NormalizePathForComparison(s.StoragePath)] = true
		}
	}

	var result []types.ImportInfo
	for _, imp := range imports {
		info := importEntryToInfo(imp)
		impPath := util.NormalizePathForComparison(imp.ID)
		inLibrary := existingPaths[impPath]

		// If series with this path already exists in library, mark as Already Imported
		if info.Status == types.ImportStatusImport && inLibrary {
			info.Status = types.ImportStatusDoNotChange
			info.Action = types.ImportActionSkip
		}
		// If marked as Already Imported but series was removed from library, reset
		if info.Status == types.ImportStatusDoNotChange && !inLibrary {
			info.Status = types.ImportStatusImport
			info.Action = types.ImportActionAdd
		}

		result = append(result, info)
	}

	if result == nil {
		result = []types.ImportInfo{}
	}

	return c.JSON(http.StatusOK, result)
}

// GetImportTotals returns summary counts for pending imports.
// GET /api/setup/imports/totals
func (h *SetupHandler) GetImportTotals(c echo.Context) error {
	ctx := c.Request().Context()

	imports, err := h.db.ImportEntry.Query().
		Where(
			importentry.StatusNEQ(int(types.ImportStatusDoNotChange)),
			importentry.ActionEQ(int(types.ImportActionAdd)),
		).
		All(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	totalSeries := len(imports)
	providerSet := make(map[string]struct{})
	totalDownloads := 0

	for _, imp := range imports {
		if len(imp.Series) == 0 {
			continue
		}

		// Count providers and downloads from selected series
		var fullSeries []types.FullSeries
		data, _ := json.Marshal(imp.Series)
		json.Unmarshal(data, &fullSeries)

		for _, fs := range fullSeries {
			if !fs.IsSelected {
				continue
			}
			providerSet[fs.Provider+"|"+fs.Lang] = struct{}{}

			continueAfter := 0.0
			if imp.ContinueAfterChapter != nil {
				continueAfter = *imp.ContinueAfterChapter
			}
			for _, ch := range fs.Chapters {
				if ch.Number != nil && *ch.Number > continueAfter {
					totalDownloads++
				}
			}
		}
	}

	return c.JSON(http.StatusOK, types.ImportTotals{
		TotalSeries:    totalSeries,
		TotalProviders: len(providerSet),
		TotalDownloads: totalDownloads,
	})
}

// --- Helpers ---

// importEntryToInfo converts an Ent ImportEntry to the API ImportInfo DTO.
// Matches .NET's Import.ToImportInfo() logic including preferred series selection,
// ContinueAfterChapter calculation, thumbnail URL rewriting, and completion status.
func importEntryToInfo(imp *ent.ImportEntry) types.ImportInfo {
	// Calculate ContinueAfterChapter from local archives (max chapter number)
	var lastRecordChap *float64
	if imp.Info != nil {
		for _, p := range imp.Info.Providers {
			for _, a := range p.Archives {
				if a.ChapterNumber != nil {
					if lastRecordChap == nil || *a.ChapterNumber > *lastRecordChap {
						n := *a.ChapterNumber
						lastRecordChap = &n
					}
				}
			}
		}
	}
	if lastRecordChap == nil {
		neg := -1.0
		lastRecordChap = &neg
	}

	// Use path and title from Info if available (matches .NET: import.Info.Path / import.Info.Title)
	path := imp.ID
	title := imp.Title
	if imp.Info != nil {
		if imp.Info.Path != "" {
			path = imp.Info.Path
		}
		if imp.Info.Title != "" {
			title = imp.Info.Title
		}
	}

	info := types.ImportInfo{
		Path:                 path,
		Title:                title,
		Status:               types.ImportStatus(imp.Status),
		Action:               types.ImportAction(imp.Action),
		ContinueAfterChapter: lastRecordChap,
	}

	// Convert raw series JSON to FullSeries for processing
	var fullSeries []types.FullSeries
	if len(imp.Series) > 0 {
		data, _ := json.Marshal(imp.Series)
		json.Unmarshal(data, &fullSeries)
	}

	if len(fullSeries) == 0 {
		info.Series = []types.SmallSeries{}
		return info
	}

	// Convert FullSeries -> SmallSeries (like .NET)
	for _, fs := range fullSeries {
		// Rewrite thumbnail URLs
		thumb := fs.ThumbnailURL
		if thumb == nil || *thumb == "" {
			unknownThumb := "/api/serie/thumb/unknown"
			thumb = &unknownThumb
		}

		// Calculate last chapter from chapters
		var lastChapter *float64
		for _, ch := range fs.Chapters {
			if ch.Number != nil {
				if lastChapter == nil || *ch.Number > *lastChapter {
					n := *ch.Number
					lastChapter = &n
				}
			}
		}

		// Format chapter list from chapters if not already set
		chapterList := fs.ChapterList
		if chapterList == "" && len(fs.Chapters) > 0 {
			var nums []float64
			for _, ch := range fs.Chapters {
				if ch.Number != nil {
					nums = append(nums, *ch.Number)
				}
			}
			chapterList = formatDecimalRanges(nums)
		}

		info.Series = append(info.Series, types.SmallSeries{
			ID:           fs.ID,
			ProviderID:   fs.ProviderID,
			Provider:     fs.Provider,
			Scanlator:    fs.Scanlator,
			Lang:         fs.Lang,
			ThumbnailURL: thumb,
			Title:        fs.Title,
			ChapterCount: int64(fs.ChapterCount),
			URL:          fs.URL,
			LastChapter:  lastChapter,
			ChapterList:  chapterList,
			UseCover:     fs.UseCover,
			Importance:   fs.Importance,
			UseTitle:     fs.UseTitle,
			Preferred:    fs.IsSelected,
			IsSelected:   fs.IsSelected,
		})
	}

	// Apply preferred series selection (matches .NET's SetPreferredSeries)
	var providerInfos []types.ProviderInfo
	if imp.Info != nil {
		providerInfos = imp.Info.Providers
	}
	setPreferredSeries(info.Series, providerInfos)

	// Ensure preferred storage series (matches .NET's EnsurePreferredStorageSeries)
	ensurePreferredPrimarySeries(info.Series)

	// Check completion status (matches .NET logic)
	var maxSeriesChapter *float64
	for i := range info.Series {
		if info.Series[i].LastChapter != nil {
			if maxSeriesChapter == nil || *info.Series[i].LastChapter > *maxSeriesChapter {
				n := *info.Series[i].LastChapter
				maxSeriesChapter = &n
			}
		}
	}

	// If any series is COMPLETED or PUBLISHING_FINISHED, check if import is complete
	for _, fs := range fullSeries {
		if fs.Status == types.SeriesStatusCompleted || fs.Status == types.SeriesStatusPublishingFinished {
			var maxFSChapter *float64
			for _, ch := range fs.Chapters {
				if ch.Number != nil {
					if maxFSChapter == nil || *ch.Number > *maxFSChapter {
						n := *ch.Number
						maxFSChapter = &n
					}
				}
			}
			if maxFSChapter != nil && lastRecordChap != nil && *maxFSChapter <= *lastRecordChap &&
				info.Action != types.ImportActionSkip {
				info.Status = types.ImportStatusCompleted
				info.Action = types.ImportActionAdd
			}
			break
		}
	}

	// Ensure preferred series is the most important (importance 0) with UseTitle + UseCover
	var preferredPrimary *types.SmallSeries
	for i := range info.Series {
		if info.Series[i].Preferred && info.Series[i].Importance == 0 {
			preferredPrimary = &info.Series[i]
			break
		}
	}
	if preferredPrimary == nil {
		for i := range info.Series {
			if info.Series[i].Preferred {
				info.Series[i].Importance = 0
				preferredPrimary = &info.Series[i]
				break
			}
		}
	}
	if preferredPrimary != nil {
		preferredPrimary.UseTitle = true
		preferredPrimary.UseCover = true
	}

	return info
}

// setPreferredSeries selects preferred series based on provider matching.
// Matches .NET's SetPreferredSeries logic.
func setPreferredSeries(seriesList []types.SmallSeries, providers []types.ProviderInfo) {
	if len(seriesList) == 0 {
		return
	}

	// If any are already preferred, return
	for _, s := range seriesList {
		if s.Preferred {
			return
		}
	}

	// First pass: match against import providers
	for _, p := range providers {
		for i := range seriesList {
			s := &seriesList[i]
			if p.Scanlator == "" || strings.EqualFold(p.Provider, p.Scanlator) {
				// No specific scanlator — match by provider + lang
				if strings.EqualFold(s.Provider, p.Provider) &&
					strings.EqualFold(s.Lang, p.Language) {
					s.Preferred = true
				}
			} else {
				// Match by provider + scanlator + lang
				if strings.EqualFold(s.Provider, p.Provider) &&
					strings.EqualFold(s.Scanlator, p.Scanlator) &&
					strings.EqualFold(s.Lang, p.Language) {
					s.Preferred = true
				}
			}
		}
	}

	// If any matched, we're done
	for _, s := range seriesList {
		if s.Preferred {
			return
		}
	}

	// Second pass: pick the series with the highest last chapter
	if len(seriesList) == 0 {
		return
	}

	// Find max last chapter
	var maxLast *float64
	for _, s := range seriesList {
		if s.LastChapter != nil {
			if maxLast == nil || *s.LastChapter > *maxLast {
				n := *s.LastChapter
				maxLast = &n
			}
		}
	}

	if maxLast == nil {
		return
	}

	// Collect series with the max last chapter
	var candidates []int
	for i, s := range seriesList {
		if s.LastChapter != nil && *s.LastChapter == *maxLast {
			candidates = append(candidates, i)
		}
	}

	if len(candidates) == 1 {
		seriesList[candidates[0]].Preferred = true
	} else if len(candidates) > 1 {
		// Pick the one with most chapters
		bestIdx := candidates[0]
		bestCount := seriesList[bestIdx].ChapterCount
		for _, idx := range candidates[1:] {
			if seriesList[idx].ChapterCount > bestCount {
				bestIdx = idx
				bestCount = seriesList[idx].ChapterCount
			}
		}
		seriesList[bestIdx].Preferred = true
	}
}

// ensurePreferredPrimarySeries ensures that if no preferred series is the primary
// (most important, importance == 0), the most complete primary series is also marked as preferred.
func ensurePreferredPrimarySeries(seriesList []types.SmallSeries) {
	if len(seriesList) == 0 {
		return
	}

	// Check if any preferred series is also the primary (importance 0)
	hasPreferredPrimary := false
	hasPreferred := false
	for _, s := range seriesList {
		if s.Preferred {
			hasPreferred = true
			if s.Importance == 0 {
				hasPreferredPrimary = true
				break
			}
		}
	}

	if !hasPreferred || hasPreferredPrimary {
		return
	}

	// No preferred is primary — find best primary series and mark it preferred
	bestIdx := -1
	var bestCount int64
	for i, s := range seriesList {
		if s.Importance == 0 {
			if bestIdx == -1 || s.ChapterCount > bestCount {
				bestIdx = i
				bestCount = s.ChapterCount
			}
		}
	}
	if bestIdx >= 0 {
		seriesList[bestIdx].Preferred = true
	}
}

// formatDecimalRanges formats a slice of chapter numbers into range strings.
func formatDecimalRanges(nums []float64) string {
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
			ranges = append(ranges, fmtRangeStr(start, end))
			start = nums[i]
			end = nums[i]
		}
	}
	ranges = append(ranges, fmtRangeStr(start, end))
	return strings.Join(ranges, ", ")
}

func fmtRangeStr(start, end float64) string {
	if start == end {
		return fmtNumStr(start)
	}
	return fmtNumStr(start) + "-" + fmtNumStr(end)
}

func fmtNumStr(n float64) string {
	if n == float64(int(n)) {
		return strconv.Itoa(int(n))
	}
	return strconv.FormatFloat(n, 'f', 1, 64)
}
