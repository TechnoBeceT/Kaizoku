package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/ent/latestseries"
	entseries "github.com/technobecet/kaizoku-go/internal/ent/series"
	"github.com/technobecet/kaizoku-go/internal/ent/seriesprovider"
	"github.com/technobecet/kaizoku-go/internal/job"
	settingssvc "github.com/technobecet/kaizoku-go/internal/service/settings"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
	"github.com/technobecet/kaizoku-go/internal/types"
	"github.com/technobecet/kaizoku-go/internal/util"
)

// placeholderThumb is a 200x200 gray PNG returned when no thumbnail is available.
var placeholderThumb []byte

func init() {
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, gray)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	placeholderThumb = buf.Bytes()
}

type SeriesHandler struct {
	config    *config.Config
	db        *ent.Client
	suwayomi  *suwayomi.Client
	settings  *settingssvc.Service
	river     riverClient
	downloads *job.DownloadDispatcher
	jobDeps   *job.Deps
}

// baseURL returns the API base URL for thumbnail/icon rewriting.
func (h *SeriesHandler) baseURL(c echo.Context) string {
	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/", scheme, c.Request().Host)
}

// GetSeries returns extended information about a series by ID.
// GET /api/serie?id=<uuid>
func (h *SeriesHandler) GetSeries(c echo.Context) error {
	idStr := c.QueryParam("id")
	if idStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id required"})
	}

	uid, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	ctx := c.Request().Context()

	s, err := h.db.Series.Get(ctx, uid)
	if err != nil {
		if ent.IsNotFound(err) {
			return c.JSON(http.StatusOK, types.SeriesExtendedInfo{})
		}
		log.Error().Err(err).Str("id", idStr).Msg("failed to get series")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error getting series."})
	}

	// Eager load providers
	providers, err := s.QueryProviders().Order(seriesprovider.ByImportance()).All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to load providers")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error getting series."})
	}

	settings, err := h.settings.Get(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to load settings for series")
	}

	result := h.toSeriesExtendedInfo(c, s, providers, settings)
	return c.JSON(http.StatusOK, result)
}

// GetLibrary returns all series in the library.
// GET /api/serie/library
func (h *SeriesHandler) GetLibrary(c echo.Context) error {
	ctx := c.Request().Context()

	seriesList, err := h.db.Series.Query().
		WithProviders().
		All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get library")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error getting library."})
	}

	baseURL := h.baseURL(c)
	result := make([]types.SeriesInfo, 0, len(seriesList))
	for _, s := range seriesList {
		providers := s.Edges.Providers
		result = append(result, toSeriesInfo(s, providers, baseURL))
	}

	return c.JSON(http.StatusOK, result)
}

// GetLatest returns latest or popular series with optional filtering and pagination.
// GET /api/serie/latest?start=0&count=50&sourceId=src1,src2&keyword=&mode=latest|popular
func (h *SeriesHandler) GetLatest(c echo.Context) error {
	start, _ := strconv.Atoi(c.QueryParam("start"))
	count, _ := strconv.Atoi(c.QueryParam("count"))
	sourceIDParam := c.QueryParam("sourceId")
	keyword := c.QueryParam("keyword")
	mode := c.QueryParam("mode") // "latest" (default) or "popular"

	if count <= 0 {
		count = 50
	}

	// Parse comma-separated source IDs
	var sourceIDs []string
	if sourceIDParam != "" {
		for _, s := range strings.Split(sourceIDParam, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				sourceIDs = append(sourceIDs, s)
			}
		}
	}

	ctx := c.Request().Context()
	baseURL := h.baseURL(c)

	if mode == "popular" {
		return h.getPopular(c, ctx, baseURL, start, count, sourceIDs, keyword)
	}

	// Default: latest mode — query from DB
	query := h.db.LatestSeries.Query()

	if len(sourceIDs) == 1 {
		query = query.Where(latestseries.SuwayomiSourceIDEQ(sourceIDs[0]))
	} else if len(sourceIDs) > 1 {
		query = query.Where(latestseries.SuwayomiSourceIDIn(sourceIDs...))
	}
	if keyword != "" {
		query = query.Where(latestseries.TitleContainsFold(keyword))
	}

	query = query.Order(ent.Desc(latestseries.FieldFetchDate))

	if start > 0 {
		query = query.Offset(start)
	}

	entries, err := query.Limit(count).All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get latest series")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error getting latest cloud library."})
	}

	result := make([]types.LatestSeriesInfo, 0, len(entries))
	for _, e := range entries {
		result = append(result, toLatestSeriesInfo(e, baseURL))
	}

	return c.JSON(http.StatusOK, result)
}

// getPopular fetches popular series on-demand from Suwayomi sources.
func (h *SeriesHandler) getPopular(c echo.Context, ctx context.Context, baseURL string, start, count int, sourceIDs []string, keyword string) error {
	if len(sourceIDs) == 0 {
		return c.JSON(http.StatusOK, []types.LatestSeriesInfo{})
	}

	// Build source info lookup
	sourceMap := make(map[string]suwayomi.SuwayomiSource)
	sources, _ := h.suwayomi.GetSources(ctx)
	for _, s := range sources {
		sourceMap[s.ID] = s
	}

	// Calculate which Suwayomi page to request based on start/count
	// Each Suwayomi page has ~20 items; we fetch enough to fill the request
	suwayomiPage := (start / 20) + 1

	var allResults []types.LatestSeriesInfo
	now := time.Now().UTC().Format(time.RFC3339)

	for _, srcID := range sourceIDs {
		sourceName := srcID
		sourceLang := ""
		if src, ok := sourceMap[srcID]; ok {
			sourceName = src.Name
			sourceLang = strings.ToLower(src.Lang)
		}

		popStart := time.Now()
		popCount := 0
		var popErr error

		// Fetch enough pages to cover the requested range
		pagesNeeded := (count / 20) + 1
		for p := 0; p < pagesNeeded; p++ {
			result, err := h.suwayomi.GetPopularSeries(ctx, srcID, suwayomiPage+p)
			if err != nil {
				log.Warn().Err(err).Str("sourceId", srcID).Msg("failed to fetch popular")
				popErr = err
				break
			}
			if result == nil || len(result.MangaList) == 0 {
				break
			}

			for _, manga := range result.MangaList {
				// Apply keyword filter client-side
				if keyword != "" && !strings.Contains(strings.ToLower(manga.Title), strings.ToLower(keyword)) {
					continue
				}

				popCount++
				thumbProxy := fmt.Sprintf("serie/thumb/%d", manga.ID)
				thumb := baseURL + thumbProxy
				info := types.LatestSeriesInfo{
					ID:               strconv.Itoa(manga.ID),
					SuwayomiSourceID: srcID,
					Provider:         sourceName,
					Language:         sourceLang,
					URL:              manga.RealURL,
					Title:            manga.Title,
					ThumbnailURL:     &thumb,
					Artist:           manga.Artist,
					Author:           manga.Author,
					Description:      manga.Description,
					Genre:            manga.Genre,
					FetchDate:        now,
					ChapterCount:     manga.ChapterCount,
					Status:           types.SeriesStatus(manga.Status),
					InLibrary:        types.InLibraryNotInLibrary,
				}
				allResults = append(allResults, info)
			}

			if !result.HasNextPage {
				break
			}
		}

		// Skip logging if the HTTP request context was canceled (user navigated away — not a source issue)
		if popErr != nil && !errors.Is(popErr, context.Canceled) {
			util.LogSourceEvent(h.db, srcID, sourceName, sourceLang,
				"get_popular", "failed", time.Since(popStart).Milliseconds(),
				util.WithError(popErr),
				util.WithMetadata(map[string]string{"origin": "http_request", "endpoint": "get_popular"}))
		} else if popErr == nil {
			util.LogSourceEvent(h.db, srcID, sourceName, sourceLang,
				"get_popular", "success", time.Since(popStart).Milliseconds(),
				util.WithItemsCount(popCount),
				util.WithMetadata(map[string]string{"origin": "http_request", "endpoint": "get_popular"}))
		}
	}

	// Apply pagination offset within the merged results
	offset := start % 20
	if offset > 0 && offset < len(allResults) {
		allResults = allResults[offset:]
	} else if offset >= len(allResults) {
		allResults = nil
	}

	// Limit to requested count
	if len(allResults) > count {
		allResults = allResults[:count]
	}

	if allResults == nil {
		allResults = []types.LatestSeriesInfo{}
	}

	return c.JSON(http.StatusOK, allResults)
}

// GetSources returns all available Suwayomi sources.
// GET /api/serie/source
func (h *SeriesHandler) GetSources(c echo.Context) error {
	ctx := c.Request().Context()

	sources, err := h.suwayomi.GetSources(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get sources")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error getting sources."})
	}

	// Rewrite icon URLs to our proxy
	baseURL := h.baseURL(c)
	for i := range sources {
		if sources[i].IconURL != "" {
			sources[i].IconURL = baseURL + "serie/source/icon/" + sources[i].ID
		}
	}

	return c.JSON(http.StatusOK, sources)
}

// GetSourceIcon proxies the extension icon for a source.
// GET /api/serie/source/icon/:apk
func (h *SeriesHandler) GetSourceIcon(c echo.Context) error {
	apkName := c.Param("apk")
	if apkName == "" {
		return c.NoContent(http.StatusNotFound)
	}

	// Strip cache-busting suffix
	realApk := apkName
	if idx := strings.Index(apkName, "!"); idx >= 0 {
		realApk = apkName[:idx]
	}

	ctx := c.Request().Context()
	data, contentType, err := h.suwayomi.GetExtensionIcon(ctx, realApk)
	if err != nil {
		log.Warn().Err(err).Str("apk", realApk).Msg("failed to get source icon")
		return c.NoContent(http.StatusNotFound)
	}

	if contentType == "" {
		contentType = "image/png"
	}
	return c.Blob(http.StatusOK, contentType, data)
}

// GetSeriesThumbnail proxies the manga thumbnail from Suwayomi.
// GET /api/serie/thumb/:id
func (h *SeriesHandler) GetSeriesThumbnail(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.NoContent(http.StatusNotFound)
	}

	if strings.HasPrefix(id, "unknown") {
		return c.Blob(http.StatusOK, "image/png", placeholderThumb)
	}

	// Strip cache-busting suffix
	realID := id
	if parts := strings.SplitN(id, "!", 2); len(parts) > 1 {
		realID = parts[0]
	}

	mangaID, err := strconv.Atoi(realID)
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}

	ctx := c.Request().Context()
	data, contentType, err := h.suwayomi.GetMangaThumbnail(ctx, mangaID)
	if err != nil {
		log.Warn().Err(err).Int("mangaId", mangaID).Msg("failed to get manga thumbnail")
		return c.NoContent(http.StatusNotFound)
	}

	if contentType == "" {
		contentType = "image/jpeg"
	}
	return c.Blob(http.StatusOK, contentType, data)
}

// VerifyIntegrity performs comprehensive integrity verification for a series.
// Checks files against DB, fixes DB records for missing/bad files, detects orphans,
// recalculates ContinueAfterChapter, regenerates kaizoku.json, and enqueues re-downloads.
// GET /api/serie/verify?g=<uuid>
func (h *SeriesHandler) VerifyIntegrity(c echo.Context) error {
	idStr := c.QueryParam("g")
	if idStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id required"})
	}

	uid, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	ctx := c.Request().Context()

	_, err = h.db.Series.Get(ctx, uid)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Series not found"})
	}

	settings, _ := h.settings.Get(ctx)
	if settings == nil {
		return c.JSON(http.StatusOK, types.SeriesIntegrityResult{
			Success:     true,
			BadFiles:    []types.ArchiveIntegrityResult{},
			OrphanFiles: []string{},
		})
	}

	result := h.jobDeps.VerifySeriesIntegrity(ctx, uid, settings.StorageFolder)
	return c.JSON(http.StatusOK, result)
}

// CleanupSeries removes bad archives and fixes DB records for a series.
// Now delegates to the same shared verify logic as VerifyIntegrity.
// GET /api/serie/cleanup?g=<uuid>
func (h *SeriesHandler) CleanupSeries(c echo.Context) error {
	idStr := c.QueryParam("g")
	if idStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id required"})
	}

	uid, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	ctx := c.Request().Context()

	_, err = h.db.Series.Get(ctx, uid)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Series not found"})
	}

	settings, _ := h.settings.Get(ctx)
	if settings == nil {
		return c.JSON(http.StatusOK, nil)
	}

	h.jobDeps.VerifySeriesIntegrity(ctx, uid, settings.StorageFolder)
	return c.JSON(http.StatusOK, nil)
}

// DeepVerify performs content validation by reading ComicInfo.xml from CBZ files.
// Compares series titles in metadata against expected titles and checks Suwayomi source links.
// GET /api/serie/deep-verify?g=<uuid>
func (h *SeriesHandler) DeepVerify(c echo.Context) error {
	idStr := c.QueryParam("g")
	if idStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id required"})
	}

	uid, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	ctx := c.Request().Context()

	s, err := h.db.Series.Get(ctx, uid)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Series not found"})
	}

	settings, _ := h.settings.Get(ctx)
	if settings == nil || s.StoragePath == "" {
		return c.JSON(http.StatusOK, types.DeepVerifyResult{
			Success:         true,
			SuspiciousFiles: []types.SuspiciousFile{},
			SourceIssues:    []types.SourceIssue{},
		})
	}

	seriesDir := filepath.Join(settings.StorageFolder, s.StoragePath)

	providers, err := s.QueryProviders().All(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error verifying series."})
	}

	result := types.DeepVerifyResult{
		SuspiciousFiles: []types.SuspiciousFile{},
		SourceIssues:    []types.SourceIssue{},
	}

	// 1. Source link verification — check Suwayomi still has the right manga
	for _, p := range providers {
		if p.IsUnknown || p.IsDisabled || p.SuwayomiID == 0 {
			continue
		}

		suwayomiData, err := h.suwayomi.GetManga(ctx, p.SuwayomiID)
		if err != nil {
			log.Warn().Err(err).Int("suwayomiId", p.SuwayomiID).Msg("deep-verify: failed to fetch from Suwayomi")
			continue
		}

		if suwayomiData.Title != "" && !areTitlesSimilar(suwayomiData.Title, p.Title) {
			suwayomiURL := ""
			if suwayomiData.RealURL != nil {
				suwayomiURL = *suwayomiData.RealURL
			} else if suwayomiData.URL != "" {
				suwayomiURL = suwayomiData.URL
			}

			result.SourceIssues = append(result.SourceIssues, types.SourceIssue{
				ProviderID:    p.ID.String(),
				Provider:      p.Provider,
				ExpectedTitle: p.Title,
				CurrentTitle:  suwayomiData.Title,
				SuwayomiURL:   suwayomiURL,
				Reason:        "source_changed",
			})
		}
	}

	// 2. Check each CBZ: page count truncation + ComicInfo.xml metadata
	for _, p := range providers {
		for _, ch := range p.Chapters {
			if ch.Filename == "" || ch.IsDeleted {
				continue
			}

			archivePath := filepath.Join(seriesDir, ch.Filename)
			chapNum := ""
			if ch.Number != nil {
				chapNum = util.FormatChapterNumber(*ch.Number)
			}

			// 2a. Page count truncation check
			if ch.PageCount != nil && *ch.PageCount > 0 {
				localPages := util.CountCBZPages(archivePath)
				expected := *ch.PageCount
				if localPages > 0 && float64(localPages) < float64(expected)*0.8 {
					result.SuspiciousFiles = append(result.SuspiciousFiles, types.SuspiciousFile{
						Filename:      ch.Filename,
						Provider:      p.Provider,
						ExpectedTitle: p.Title,
						ActualTitle:   fmt.Sprintf("%d pages (expected %d)", localPages, expected),
						ChapterNumber: chapNum,
						Reason:        "truncated",
					})
				}
			}

			// 2b. ComicInfo.xml content validation
			ci, err := util.ReadComicInfoFromCBZ(archivePath)
			if err != nil {
				log.Debug().Err(err).Str("file", ch.Filename).Msg("deep-verify: failed to read ComicInfo.xml")
				continue
			}
			if ci == nil {
				continue // No ComicInfo.xml in this CBZ
			}

			// Check title match
			titleMatch := false
			comicInfoTitle := ci.Series
			if comicInfoTitle == "" {
				comicInfoTitle = ci.LocalizedSeries
			}
			if comicInfoTitle == "" {
				continue // No title to compare
			}

			// Compare against both provider title and series title
			if areTitlesSimilar(comicInfoTitle, p.Title) || areTitlesSimilar(comicInfoTitle, s.Title) {
				titleMatch = true
			}
			// Also check LocalizedSeries if Series was used
			if !titleMatch && ci.LocalizedSeries != "" {
				if areTitlesSimilar(ci.LocalizedSeries, p.Title) || areTitlesSimilar(ci.LocalizedSeries, s.Title) {
					titleMatch = true
				}
			}

			if !titleMatch {
				result.SuspiciousFiles = append(result.SuspiciousFiles, types.SuspiciousFile{
					Filename:      ch.Filename,
					Provider:      p.Provider,
					ExpectedTitle: p.Title,
					ActualTitle:   comicInfoTitle,
					ChapterNumber: chapNum,
					Reason:        "title_mismatch",
				})
			}

			// Check chapter number mismatch
			if ch.Number != nil && ci.Number != "" {
				expectedNum := util.FormatChapterNumber(*ch.Number)
				if ci.Number != expectedNum {
					result.SuspiciousFiles = append(result.SuspiciousFiles, types.SuspiciousFile{
						Filename:      ch.Filename,
						Provider:      p.Provider,
						ExpectedTitle: p.Title,
						ActualTitle:   comicInfoTitle,
						ChapterNumber: fmt.Sprintf("expected:%s actual:%s", expectedNum, ci.Number),
						Reason:        "chapter_mismatch",
					})
				}
			}
		}
	}

	result.Success = len(result.SuspiciousFiles) == 0 && len(result.SourceIssues) == 0
	return c.JSON(http.StatusOK, result)
}

// areTitlesSimilar compares two titles for similarity using normalized comparison.
func areTitlesSimilar(a, b string) bool {
	na := normTitle(a)
	nb := normTitle(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	// Check containment (one title might be a substring of the other)
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return true
	}
	// Levenshtein distance check
	dist := levenshteinDist(na, nb)
	maxLen := len(na)
	if len(nb) > maxLen {
		maxLen = len(nb)
	}
	if maxLen == 0 {
		return true
	}
	return float64(dist)/float64(maxLen) <= 0.3
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

// VerifyAll enqueues a background job to verify all series in the library.
// POST /api/serie/verify-all
func (h *SeriesHandler) VerifyAll(c echo.Context) error {
	ctx := c.Request().Context()
	_, err := h.river.Insert(ctx, job.VerifyAllSeriesArgs{}, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to enqueue VerifyAllSeries job")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to enqueue verification job."})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "queued"})
}

// GetProviderMatch returns the match info for an unknown provider.
// GET /api/serie/match/:providerId
func (h *SeriesHandler) GetProviderMatch(c echo.Context) error {
	providerIDStr := c.Param("providerId")
	if providerIDStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "providerId required"})
	}

	providerID, err := uuid.Parse(providerIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid providerId"})
	}

	ctx := c.Request().Context()

	// Find the provider
	provider, err := h.db.SeriesProvider.Get(ctx, providerID)
	if err != nil {
		if ent.IsNotFound(err) {
			return c.JSON(http.StatusOK, nil)
		}
		log.Error().Err(err).Msg("failed to get provider")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error getting provider match."})
	}

	// Only match unknown providers (or those with SuwayomiID == 0)
	if provider.SuwayomiID != 0 && !provider.IsUnknown {
		return c.JSON(http.StatusOK, nil)
	}

	// Get sibling providers (same series, not unknown)
	siblings, err := h.db.SeriesProvider.Query().
		Where(
			seriesprovider.SeriesIDEQ(provider.SeriesID),
			seriesprovider.IsUnknownEQ(false),
			seriesprovider.SuwayomiIDNEQ(0),
		).
		All(ctx)
	if err != nil || len(siblings) == 0 {
		return c.JSON(http.StatusOK, nil)
	}

	matchInfos := make([]types.MatchInfo, 0, len(siblings))
	for _, sib := range siblings {
		matchInfos = append(matchInfos, types.MatchInfo{
			ID:       sib.ID.String(),
			Provider: sib.Provider,
			Scanlator: sib.Scanlator,
			Language: sib.Language,
		})
	}

	chapters := make([]types.ProviderMatchChapter, 0)
	for _, ch := range provider.Chapters {
		if ch.IsDeleted || ch.Filename == "" {
			continue
		}
		chapters = append(chapters, types.ProviderMatchChapter{
			ChapterNumber: ch.Number,
			ChapterName:   ch.Name,
			MatchInfoID:   nil,
			Filename:      fileNameWithoutExt(ch.Filename),
		})
	}
	sort.Slice(chapters, func(i, j int) bool {
		ni := float64(0)
		nj := float64(0)
		if chapters[i].ChapterNumber != nil {
			ni = *chapters[i].ChapterNumber
		}
		if chapters[j].ChapterNumber != nil {
			nj = *chapters[j].ChapterNumber
		}
		return ni < nj
	})

	result := types.ProviderMatch{
		ID:         provider.ID.String(),
		MatchInfos: matchInfos,
		Chapters:   chapters,
	}
	return c.JSON(http.StatusOK, result)
}

// SetProviderMatch applies chapter-to-provider matching.
// Renames files, updates ComicInfo.xml, and verifies page counts.
// POST /api/serie/match
func (h *SeriesHandler) SetProviderMatch(c echo.Context) error {
	var pm types.ProviderMatch
	if err := c.Bind(&pm); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	ctx := c.Request().Context()

	unknownID, err := uuid.Parse(pm.ID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid provider id"})
	}

	unknown, err := h.db.SeriesProvider.Get(ctx, unknownID)
	if err != nil {
		return c.JSON(http.StatusOK, types.MatchResult{Success: false})
	}

	s, err := h.db.Series.Get(ctx, unknown.SeriesID)
	if err != nil {
		return c.JSON(http.StatusOK, types.MatchResult{Success: false})
	}

	providers, err := s.QueryProviders().All(ctx)
	if err != nil {
		return c.JSON(http.StatusOK, types.MatchResult{Success: false})
	}

	// Build provider ID → provider map
	provMap := make(map[uuid.UUID]*ent.SeriesProvider)
	for _, p := range providers {
		provMap[p.ID] = p
	}

	settings, _ := h.settings.Get(ctx)
	storageFolder := ""
	if settings != nil {
		storageFolder = settings.StorageFolder
	}

	seriesDir := ""
	if storageFolder != "" && s.StoragePath != "" {
		seriesDir = filepath.Join(storageFolder, s.StoragePath)
	}

	result := types.MatchResult{Success: true}

	for _, chap := range pm.Chapters {
		if chap.MatchInfoID == nil {
			continue
		}

		matchID, err := uuid.Parse(*chap.MatchInfoID)
		if err != nil {
			continue
		}

		target, ok := provMap[matchID]
		if !ok {
			continue
		}

		// Find the chapter in the unknown provider
		chIdx := -1
		for i, ch := range unknown.Chapters {
			if fileNameWithoutExt(ch.Filename) == chap.Filename {
				chIdx = i
				break
			}
		}
		if chIdx < 0 {
			continue
		}

		// Find the destination chapter in the target provider
		dstIdx := -1
		for i, ch := range target.Chapters {
			if ch.Number != nil && chap.ChapterNumber != nil && *ch.Number == *chap.ChapterNumber {
				dstIdx = i
				break
			}
		}
		if dstIdx < 0 {
			continue
		}

		ch := unknown.Chapters[chIdx]

		// Page count verification
		if seriesDir != "" {
			archivePath := filepath.Join(seriesDir, ch.Filename)
			localPages := util.CountCBZPages(archivePath)
			sourcePages := 0
			if target.Chapters[dstIdx].PageCount != nil {
				sourcePages = *target.Chapters[dstIdx].PageCount
			}

			// If source reports page count and local file has >20% fewer pages, flag for re-download
			if sourcePages > 0 && localPages > 0 && float64(localPages) < float64(sourcePages)*0.8 {
				result.Redownloads++
				result.MismatchFiles = append(result.MismatchFiles, types.ProviderMatchChapter{
					Filename:      ch.Filename,
					ChapterNumber: chap.ChapterNumber,
					ChapterName:   chap.ChapterName,
					LocalPages:    localPages,
					SourcePages:   sourcePages,
					PageMismatch:  true,
				})
				// Flag chapter for re-download instead of transferring
				target.Chapters[dstIdx].ShouldDownload = true
				target.Chapters[dstIdx].Filename = ""
				target.Chapters[dstIdx].DownloadDate = nil

				// Remove the bad file from unknown provider
				unknown.Chapters = append(unknown.Chapters[:chIdx], unknown.Chapters[chIdx+1:]...)

				// Delete the incomplete archive from disk
				_ = os.Remove(archivePath)

				// Save target provider
				if err := h.db.SeriesProvider.UpdateOneID(target.ID).
					SetChapters(target.Chapters).
					Exec(ctx); err != nil {
					log.Warn().Err(err).Msg("failed to update target provider for re-download")
				}
				continue
			}
		}

		// Rename the file
		maxChap := maxChapterNumber(target.Chapters)
		newBase := makeFileNameSafe(target.Provider, target.Scanlator, target.Title, target.Language,
			target.Chapters[dstIdx].Number, target.Chapters[dstIdx].Name, maxChap)
		ext := path.Ext(ch.Filename)
		newFilename := newBase + ext

		if seriesDir != "" {
			originalPath := filepath.Join(seriesDir, ch.Filename)
			newPath := filepath.Join(seriesDir, newFilename)
			if originalPath != newPath {
				if err := os.Rename(originalPath, newPath); err != nil {
					log.Warn().Err(err).Str("from", originalPath).Str("to", newPath).Msg("failed to rename matched chapter file")
				}
			}

			// Update ComicInfo.xml inside the CBZ
			cbzPath := filepath.Join(seriesDir, newFilename)
			h.updateMatchedComicInfo(ctx, cbzPath, s, target, &target.Chapters[dstIdx])
		}

		// Update target chapter
		target.Chapters[dstIdx].Filename = newFilename
		target.Chapters[dstIdx].DownloadDate = ch.DownloadDate
		target.Chapters[dstIdx].ShouldDownload = false

		// Remove from unknown
		unknown.Chapters = append(unknown.Chapters[:chIdx], unknown.Chapters[chIdx+1:]...)

		// Save target provider chapters
		if err := h.db.SeriesProvider.UpdateOneID(target.ID).
			SetChapters(target.Chapters).
			Exec(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to update target provider chapters")
		}

		result.Matched++
	}

	if len(unknown.Chapters) == 0 {
		// Remove the unknown provider
		if err := h.db.SeriesProvider.DeleteOneID(unknown.ID).Exec(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to delete empty unknown provider")
		}
	} else if result.Matched > 0 || result.Redownloads > 0 {
		if err := h.db.SeriesProvider.UpdateOneID(unknown.ID).
			SetChapters(unknown.Chapters).
			Exec(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to update unknown provider chapters")
		}
	}

	// Save kaizoku.json after matching
	if s.StoragePath != "" && storageFolder != "" {
		h.saveKaizokuJSON(ctx, s.ID, storageFolder)
	}

	return c.JSON(http.StatusOK, result)
}

// updateMatchedComicInfo updates the ComicInfo.xml inside a CBZ after matching to a known provider.
func (h *SeriesHandler) updateMatchedComicInfo(ctx context.Context, cbzPath string, s *ent.Series, target *ent.SeriesProvider, ch *types.Chapter) {
	meta := util.ChapterMeta{
		SeriesTitle:   s.Title,
		ProviderTitle: target.Title,
		ChapterNumber: ch.Number,
		ChapterName:   ch.Name,
		ChapterCount:  len(target.Chapters),
		Language:      target.Language,
		Provider:      target.Provider,
		Scanlator:     target.Scanlator,
		Genre:         s.Genre,
	}
	if target.Author != nil {
		meta.Author = *target.Author
	}
	if target.Artist != nil {
		meta.Artist = *target.Artist
	}
	if s.Type != nil {
		meta.Type = *s.Type
	}
	if target.URL != nil && *target.URL != "" {
		meta.URL = *target.URL
	}
	if ch.PageCount != nil {
		meta.PageCount = *ch.PageCount
	}
	if ch.ProviderUploadDate != nil {
		meta.UploadDate = ch.ProviderUploadDate
	}

	ci := util.NewComicInfo(meta)
	if err := util.UpdateCBZComicInfo(cbzPath, ci); err != nil {
		log.Warn().Err(err).Str("path", cbzPath).Msg("failed to update ComicInfo.xml after match")
	}
}

// autoMatchUnknownProviders transfers downloaded files from Unknown providers to matching known providers.
// Called after adding new providers to a series. Matches by provider name + language.
func (h *SeriesHandler) autoMatchUnknownProviders(ctx context.Context, s *ent.Series, settings *types.Settings) {
	allProviders, err := h.db.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(s.ID)).
		All(ctx)
	if err != nil {
		return
	}

	var unknowns []*ent.SeriesProvider
	var knowns []*ent.SeriesProvider
	for _, p := range allProviders {
		if p.IsUnknown {
			unknowns = append(unknowns, p)
		} else {
			knowns = append(knowns, p)
		}
	}
	if len(unknowns) == 0 || len(knowns) == 0 {
		return
	}

	storageFolder := ""
	if settings != nil {
		storageFolder = settings.StorageFolder
	}
	seriesDir := ""
	if storageFolder != "" && s.StoragePath != "" {
		seriesDir = filepath.Join(storageFolder, s.StoragePath)
	}

	for _, unknown := range unknowns {
		unknownProv := strings.ToLower(unknown.Provider)
		unknownLang := strings.ToLower(unknown.Language)

		// Find a matching known provider (by provider name + language)
		var target *ent.SeriesProvider
		for _, k := range knowns {
			if strings.ToLower(k.Provider) == unknownProv && strings.ToLower(k.Language) == unknownLang {
				target = k
				break
			}
		}
		// Also try matching by language only if the unknown provider name is "Unknown"
		if target == nil && unknownProv == "unknown" {
			for _, k := range knowns {
				if strings.ToLower(k.Language) == unknownLang {
					target = k
					break
				}
			}
		}
		if target == nil {
			continue
		}

		transferred := 0
		remaining := make([]types.Chapter, 0, len(unknown.Chapters))

		for _, ch := range unknown.Chapters {
			if ch.Filename == "" || ch.Number == nil {
				remaining = append(remaining, ch)
				continue
			}

			// Find matching chapter number in target
			dstIdx := -1
			for i, tch := range target.Chapters {
				if tch.Number != nil && *tch.Number == *ch.Number {
					dstIdx = i
					break
				}
			}
			if dstIdx < 0 {
				remaining = append(remaining, ch)
				continue
			}

			// Page count verification
			if seriesDir != "" {
				archivePath := filepath.Join(seriesDir, ch.Filename)
				localPages := util.CountCBZPages(archivePath)
				sourcePages := 0
				if target.Chapters[dstIdx].PageCount != nil {
					sourcePages = *target.Chapters[dstIdx].PageCount
				}
				if sourcePages > 0 && localPages > 0 && float64(localPages) < float64(sourcePages)*0.8 {
					// Mark for re-download, discard the bad file
					target.Chapters[dstIdx].ShouldDownload = true
					_ = os.Remove(archivePath)
					transferred++ // Still counts as handled
					continue
				}
			}

			// Rename file to target naming convention
			maxChap := maxChapterNumber(target.Chapters)
			newBase := makeFileNameSafe(target.Provider, target.Scanlator, target.Title, target.Language,
				target.Chapters[dstIdx].Number, target.Chapters[dstIdx].Name, maxChap)
			ext := path.Ext(ch.Filename)
			newFilename := newBase + ext

			if seriesDir != "" {
				originalPath := filepath.Join(seriesDir, ch.Filename)
				newPath := filepath.Join(seriesDir, newFilename)
				if originalPath != newPath {
					if err := os.Rename(originalPath, newPath); err != nil {
						log.Warn().Err(err).Str("from", originalPath).Str("to", newPath).Msg("auto-match: failed to rename")
						remaining = append(remaining, ch)
						continue
					}
				}

				// Update ComicInfo.xml
				cbzPath := filepath.Join(seriesDir, newFilename)
				h.updateMatchedComicInfo(ctx, cbzPath, s, target, &target.Chapters[dstIdx])
			}

			// Transfer to target
			target.Chapters[dstIdx].Filename = newFilename
			target.Chapters[dstIdx].DownloadDate = ch.DownloadDate
			target.Chapters[dstIdx].ShouldDownload = false
			transferred++
		}

		if transferred == 0 {
			continue
		}

		// Save target provider
		var maxDownloaded *float64
		for _, ch := range target.Chapters {
			if ch.Filename != "" && ch.Number != nil {
				if maxDownloaded == nil || *ch.Number > *maxDownloaded {
					n := *ch.Number
					maxDownloaded = &n
				}
			}
		}
		update := h.db.SeriesProvider.UpdateOneID(target.ID).
			SetChapters(target.Chapters)
		if maxDownloaded != nil {
			update = update.SetContinueAfterChapter(*maxDownloaded)
		}
		if err := update.Exec(ctx); err != nil {
			log.Warn().Err(err).Str("provider", target.Provider).Msg("auto-match: failed to save target provider")
		}

		// Update or delete unknown provider
		if len(remaining) == 0 {
			if err := h.db.SeriesProvider.DeleteOneID(unknown.ID).Exec(ctx); err != nil {
				log.Warn().Err(err).Msg("auto-match: failed to delete empty unknown provider")
			}
		} else {
			if err := h.db.SeriesProvider.UpdateOneID(unknown.ID).
				SetChapters(remaining).
				Exec(ctx); err != nil {
				log.Warn().Err(err).Msg("auto-match: failed to update unknown provider")
			}
		}

		log.Info().
			Str("series", s.Title).
			Str("target", target.Provider).
			Int("transferred", transferred).
			Int("remaining", len(remaining)).
			Msg("auto-matched unknown provider to known source")
	}
}

// AddSeries creates a new series from augmented data.
// POST /api/serie
func (h *SeriesHandler) AddSeries(c echo.Context) error {
	var req types.AugmentedResponse
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if len(req.Series) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "No series provided to add"})
	}

	ctx := c.Request().Context()

	settings, err := h.settings.Get(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error adding full series."})
	}

	// Check if this is an update to an existing series
	var existingSeriesID *uuid.UUID
	if req.ExistingSeriesID != nil && *req.ExistingSeriesID != "" {
		uid, err := uuid.Parse(*req.ExistingSeriesID)
		if err == nil {
			existingSeriesID = &uid
		}
	}

	var dbSeries *ent.Series

	if existingSeriesID != nil {
		dbSeries, err = h.db.Series.Get(ctx, *existingSeriesID)
		if err != nil {
			log.Error().Err(err).Msg("existing series not found")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error adding full series."})
		}
		req.StorageFolderPath = dbSeries.StoragePath
	}

	// Consolidate series data from providers
	consolidated := consolidateFullSeries(req.Series)

	if dbSeries == nil {
		// Derive relative storage path
		storagePath := req.StorageFolderPath
		if storagePath != "" && strings.HasPrefix(storagePath, settings.StorageFolder) {
			storagePath = strings.TrimPrefix(storagePath, settings.StorageFolder)
			storagePath = strings.TrimLeft(storagePath, "/\\")
		}

		// Check if a series with this storage path already exists (prevent duplicates)
		if storagePath != "" {
			existing, _ := h.db.Series.Query().
				Where(entseries.StoragePathEqualFold(storagePath)).
				First(ctx)
			if existing != nil {
				log.Info().Str("title", existing.Title).Str("storagePath", storagePath).
					Msg("series already exists at storage path, updating instead of creating duplicate")
				dbSeries = existing
			}
		}

		if dbSeries == nil {
			// Create new series
			dbSeries, err = h.db.Series.Create().
				SetTitle(consolidated.Title).
				SetDescription(consolidated.Description).
				SetThumbnailURL(derefStr(consolidated.ThumbnailURL)).
				SetArtist(consolidated.Artist).
				SetAuthor(consolidated.Author).
				SetGenre(consolidated.Genre).
				SetStatus(string(consolidated.Status)).
				SetStoragePath(storagePath).
				SetNillableType(consolidated.Type).
				SetChapterCount(consolidated.ChapterCount).
				SetPauseDownloads(req.DisableJobs).
				Save(ctx)
			if err != nil {
				log.Error().Err(err).Msg("failed to create series")
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error adding full series."})
			}
		}
	}

	if existingSeriesID != nil {
		// Update existing series metadata (explicit ID provided by caller)
		dbSeries, err = h.db.Series.UpdateOneID(dbSeries.ID).
			SetTitle(consolidated.Title).
			SetDescription(consolidated.Description).
			SetThumbnailURL(derefStr(consolidated.ThumbnailURL)).
			SetArtist(consolidated.Artist).
			SetAuthor(consolidated.Author).
			SetGenre(consolidated.Genre).
			SetStatus(string(consolidated.Status)).
			SetNillableType(consolidated.Type).
			SetChapterCount(maxInt(dbSeries.ChapterCount, consolidated.ChapterCount)).
			Save(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to update series")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error adding full series."})
		}
	}

	// Create/update providers
	existingProviders, _ := h.db.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(dbSeries.ID)).
		All(ctx)

	existingMap := make(map[string]*ent.SeriesProvider)
	for _, ep := range existingProviders {
		key := strings.ToLower(ep.Provider + "|" + ep.Language + "|" + ep.Scanlator)
		existingMap[key] = ep
	}

	for _, fs := range req.Series {
		key := strings.ToLower(fs.Provider + "|" + fs.Lang + "|" + fs.Scanlator)
		suwayomiID, _ := strconv.Atoi(fs.ID)

		if existing, ok := existingMap[key]; ok {
			// Update existing provider
			update := h.db.SeriesProvider.UpdateOneID(existing.ID).
				SetSuwayomiID(suwayomiID).
				SetNillableURL(fs.URL).
				SetNillableThumbnailURL(fs.ThumbnailURL).
				SetArtist(fs.Artist).
				SetAuthor(fs.Author).
				SetDescription(fs.Description).
				SetGenre(fs.Genre).
				SetStatus(string(fs.Status)).
				SetImportance(fs.Importance).
				SetIsTitle(fs.UseTitle).
				SetIsCover(fs.UseCover)

			if fs.ContinueAfterChapter != nil {
				update = update.SetContinueAfterChapter(*fs.ContinueAfterChapter)
			}
			if int64(fs.ChapterCount) > 0 {
				cc := int64(fs.ChapterCount)
				update = update.SetChapterCount(cc)
			}
			if len(fs.Chapters) > 0 {
				update = update.SetChapters(fs.Chapters)
			}
			if err := update.Exec(ctx); err != nil {
				log.Warn().Err(err).Str("provider", fs.Provider).Msg("failed to update provider")
			}
		} else {
			// Create new provider
			create := h.db.SeriesProvider.Create().
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
				SetIsUnknown(fs.IsUnknown)

			if fs.ContinueAfterChapter != nil {
				create = create.SetContinueAfterChapter(*fs.ContinueAfterChapter)
			}
			if int64(fs.ChapterCount) > 0 {
				cc := int64(fs.ChapterCount)
				create = create.SetChapterCount(cc)
			}
			if len(fs.Chapters) > 0 {
				create = create.SetChapters(fs.Chapters)
			}
			now := time.Now().UTC()
			create = create.SetFetchDate(now)

			if err := create.Exec(ctx); err != nil {
				log.Warn().Err(err).Str("provider", fs.Provider).Msg("failed to create provider")
			}
		}
	}

	// Auto-match Unknown providers to newly created known providers.
	// When a user adds a source to a series that has an Unknown provider from import,
	// try to automatically transfer downloaded files from Unknown → new provider.
	h.autoMatchUnknownProviders(ctx, dbSeries, settings)

	// Enqueue GetChapters jobs for each non-disabled, non-unknown provider
	if !req.DisableJobs {
		allProviders, _ := h.db.SeriesProvider.Query().
			Where(seriesprovider.SeriesIDEQ(dbSeries.ID)).
			All(ctx)
		for _, p := range allProviders {
			if p.IsDisabled || p.IsUnknown {
				continue
			}
			if _, err := h.river.Insert(ctx, job.GetChaptersArgs{ProviderID: p.ID}, nil); err != nil {
				log.Warn().Err(err).Str("providerId", p.ID.String()).Msg("failed to enqueue GetChapters job")
			}
		}
	}

	// Save kaizoku.json
	if dbSeries.StoragePath != "" {
		h.saveKaizokuJSON(ctx, dbSeries.ID, settings.StorageFolder)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"id": dbSeries.ID.String()})
}

// UpdateAllSeries enqueues a job to update all series.
// POST /api/serie/update-all
func (h *SeriesHandler) UpdateAllSeries(c echo.Context) error {
	ctx := c.Request().Context()
	_, err := h.river.Insert(ctx, job.UpdateAllSeriesArgs{}, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to enqueue UpdateAllSeries job")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enqueue job"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "message": "Update All Series Queued"})
}

// UpdateSeries updates an existing series and its providers.
// PATCH /api/serie
func (h *SeriesHandler) UpdateSeries(c echo.Context) error {
	var req types.SeriesExtendedInfo
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.ID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "No series provided to update"})
	}

	uid, err := uuid.Parse(req.ID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid series id"})
	}

	ctx := c.Request().Context()

	dbSeries, err := h.db.Series.Get(ctx, uid)
	if err != nil {
		if ent.IsNotFound(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Series not found"})
		}
		log.Error().Err(err).Msg("failed to get series for update")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error updating series."})
	}

	providers, err := h.db.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(dbSeries.ID)).
		All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get providers for update")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error updating series."})
	}

	provMap := make(map[uuid.UUID]*ent.SeriesProvider)
	for _, p := range providers {
		provMap[p.ID] = p
	}

	// Update provider settings
	deletedSuwayomiIDs := make([]int, 0)
	var deletedProviderIDs []uuid.UUID
	var newlyDisabledProviderIDs []uuid.UUID
	needsChapterRefetch := false // Track if changes require re-fetching chapters
	for _, p := range req.Providers {
		pid, err := uuid.Parse(p.ID)
		if err != nil {
			continue
		}

		existing, ok := provMap[pid]
		if !ok {
			continue
		}

		if p.IsDeleted {
			// Track deleted provider for download cancellation
			deletedProviderIDs = append(deletedProviderIDs, existing.ID)
			needsChapterRefetch = true

			// Delete physical chapter files if requested
			if p.DeleteFiles && hasDownloadedChapters(existing.Chapters) {
				settings, _ := h.settings.Get(ctx)
				if settings != nil && dbSeries.StoragePath != "" {
					seriesDir := filepath.Join(settings.StorageFolder, dbSeries.StoragePath)
					for _, ch := range existing.Chapters {
						if ch.Filename == "" {
							continue
						}
						fullPath := filepath.Join(seriesDir, ch.Filename)
						if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
							log.Warn().Err(err).Str("file", fullPath).Msg("failed to delete chapter file")
						}
					}
				}
			}

			// Delete provider from DB (always fully delete when user explicitly deletes)
			if !p.DeleteFiles && hasDownloadedChapters(existing.Chapters) {
				// Keep files: convert to unknown provider so files stay tracked
				if err := h.db.SeriesProvider.UpdateOneID(existing.ID).
					SetSuwayomiID(0).
					SetProvider("Unknown").
					SetScanlator("").
					SetURL("").
					SetThumbnailURL("").
					SetIsUnknown(true).
					SetIsTitle(false).
					SetIsCover(false).
					SetIsDisabled(false).
					SetStatus(string(types.SeriesStatusUnknown)).
					Exec(ctx); err != nil {
					log.Warn().Err(err).Msg("failed to convert provider to unknown")
				}
			} else {
				// Fully delete: no downloaded files, or files are being deleted
				if err := h.db.SeriesProvider.DeleteOneID(existing.ID).Exec(ctx); err != nil {
					log.Warn().Err(err).Msg("failed to delete provider")
				}
				deletedSuwayomiIDs = append(deletedSuwayomiIDs, existing.SuwayomiID)
			}
			continue
		}

		// Track providers that are being newly disabled or re-enabled
		if p.IsDisabled && !existing.IsDisabled {
			newlyDisabledProviderIDs = append(newlyDisabledProviderIDs, existing.ID)
			needsChapterRefetch = true
		}
		if !p.IsDisabled && existing.IsDisabled {
			needsChapterRefetch = true
		}

		// Normal update
		update := h.db.SeriesProvider.UpdateOneID(existing.ID).
			SetIsDisabled(p.IsDisabled).
			SetImportance(p.Importance).
			SetIsTitle(p.UseTitle).
			SetIsCover(p.UseCover)

		if p.ContinueAfterChapter != nil {
			if existing.ContinueAfterChapter == nil || *existing.ContinueAfterChapter != *p.ContinueAfterChapter {
				needsChapterRefetch = true
			}
			update = update.SetContinueAfterChapter(*p.ContinueAfterChapter)
		}

		if err := update.Exec(ctx); err != nil {
			log.Warn().Err(err).Str("id", p.ID).Msg("failed to update provider")
		}
	}

	// Cancel queued downloads for deleted providers
	for _, pid := range deletedProviderIDs {
		if cancelled, err := h.downloads.CancelProviderDownloads(ctx, pid); err != nil {
			log.Warn().Err(err).Str("providerId", pid.String()).Msg("failed to cancel downloads for deleted provider")
		} else if cancelled > 0 {
			log.Info().Int("count", cancelled).Str("providerId", pid.String()).Msg("cancelled downloads for deleted provider")
		}
	}

	// Cancel queued downloads for newly disabled providers
	for _, pid := range newlyDisabledProviderIDs {
		if cancelled, err := h.downloads.CancelProviderDownloads(ctx, pid); err != nil {
			log.Warn().Err(err).Str("providerId", pid.String()).Msg("failed to cancel downloads for disabled provider")
		} else if cancelled > 0 {
			log.Info().Int("count", cancelled).Str("providerId", pid.String()).Msg("cancelled downloads for disabled provider")
		}
	}

	// Track pause state changes for chapter refetch
	if req.PausedDownloads != dbSeries.PauseDownloads {
		needsChapterRefetch = true
	}

	// Cancel all queued downloads if pause is being toggled on
	if req.PausedDownloads && !dbSeries.PauseDownloads {
		if cancelled, err := h.downloads.PauseSeriesDownloads(ctx, uid); err != nil {
			log.Warn().Err(err).Msg("failed to cancel downloads for paused series")
		} else if cancelled > 0 {
			log.Info().Int("count", cancelled).Str("seriesId", uid.String()).Msg("cancelled downloads for paused series")
		}
	}

	// Reconsolidate series from remaining providers
	remainingProviders, _ := h.db.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(dbSeries.ID)).
		All(ctx)

	if len(remainingProviders) > 0 {
		consolidated := consolidateProviders(remainingProviders)

		if _, err := h.db.Series.UpdateOneID(dbSeries.ID).
			SetTitle(consolidated.title).
			SetDescription(consolidated.description).
			SetThumbnailURL(consolidated.thumbnailURL).
			SetArtist(consolidated.artist).
			SetAuthor(consolidated.author).
			SetGenre(consolidated.genre).
			SetStatus(consolidated.status).
			SetChapterCount(maxInt(dbSeries.ChapterCount, consolidated.chapterCount)).
			SetPauseDownloads(req.PausedDownloads).
			Save(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to reconsolidate series")
		}
	} else {
		// Update just pauseDownloads
		if _, err := h.db.Series.UpdateOneID(dbSeries.ID).
			SetPauseDownloads(req.PausedDownloads).
			Save(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to update series pause state")
		}
	}

	// Update InLibrary status for deleted providers
	if len(deletedSuwayomiIDs) > 0 {
		latests, _ := h.db.LatestSeries.Query().
			Where(latestseries.IDIn(deletedSuwayomiIDs...)).
			All(ctx)
		for _, l := range latests {
			_, _ = h.db.LatestSeries.UpdateOneID(l.ID).
				SetInLibrary(int(types.InLibraryNotInLibrary)).
				Save(ctx)
		}
	}

	// Re-read updated series to return
	dbSeries, _ = h.db.Series.Get(ctx, uid)
	remainingProviders, _ = h.db.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(uid)).
		Order(seriesprovider.ByImportance()).
		All(ctx)

	settings, _ := h.settings.Get(ctx)
	result := h.toSeriesExtendedInfo(c, dbSeries, remainingProviders, settings)

	// Reschedule GetChapters jobs only when significant changes occurred
	// (not for pure reorder/cover/title changes)
	if needsChapterRefetch {
		for _, p := range remainingProviders {
			if p.IsDisabled || p.IsUnknown {
				continue
			}
			if _, err := h.river.Insert(ctx, job.GetChaptersArgs{ProviderID: p.ID}, nil); err != nil {
				log.Warn().Err(err).Str("providerId", p.ID.String()).Msg("failed to enqueue GetChapters job")
			}
		}
	}

	// Save kaizoku.json
	if dbSeries != nil && dbSeries.StoragePath != "" && settings != nil {
		h.saveKaizokuJSON(ctx, dbSeries.ID, settings.StorageFolder)
	}

	return c.JSON(http.StatusOK, result)
}

// DeleteSeries removes a series from the database.
// DELETE /api/serie?id=<uuid>&alsoPhysical=false
func (h *SeriesHandler) DeleteSeries(c echo.Context) error {
	idStr := c.QueryParam("id")
	alsoPhysical, _ := strconv.ParseBool(c.QueryParam("alsoPhysical"))

	if idStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id required"})
	}

	uid, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	ctx := c.Request().Context()

	// Get the series with providers
	dbSeries, err := h.db.Series.Get(ctx, uid)
	if err != nil {
		if ent.IsNotFound(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Series not found"})
		}
		log.Error().Err(err).Msg("failed to get series for delete")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error deleting series."})
	}

	providers, err := h.db.SeriesProvider.Query().
		Where(seriesprovider.SeriesIDEQ(uid)).
		All(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get providers for delete")
	}

	suwayomiIDs := make([]int, 0, len(providers))
	for _, p := range providers {
		suwayomiIDs = append(suwayomiIDs, p.SuwayomiID)
	}

	if alsoPhysical {
		settings, _ := h.settings.Get(ctx)
		if settings != nil && dbSeries.StoragePath != "" {
			seriesDir := filepath.Join(settings.StorageFolder, dbSeries.StoragePath)
			if err := os.RemoveAll(seriesDir); err != nil {
				log.Warn().Err(err).Str("dir", seriesDir).Msg("failed to delete series directory")
			} else {
				log.Info().Str("dir", seriesDir).Msg("deleted series directory")
			}
		}
	}

	// Delete providers first (FK constraint)
	_, err = h.db.SeriesProvider.Delete().
		Where(seriesprovider.SeriesIDEQ(uid)).
		Exec(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to delete providers")
	}

	// Delete the series
	if err := h.db.Series.DeleteOneID(uid).Exec(ctx); err != nil {
		log.Error().Err(err).Msg("failed to delete series")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error deleting series."})
	}

	// Update InLibrary status for affected latest series
	if len(suwayomiIDs) > 0 {
		latests, _ := h.db.LatestSeries.Query().
			Where(latestseries.IDIn(suwayomiIDs...)).
			All(ctx)
		for _, l := range latests {
			_, _ = h.db.LatestSeries.UpdateOneID(l.ID).
				SetInLibrary(int(types.InLibraryNotInLibrary)).
				SetNillableSeriesID(nil).
				Save(ctx)
		}
	}

	// Cancel pending download jobs for this series
	if cancelled, err := h.downloads.CancelSeriesDownloads(ctx, uid); err != nil {
		log.Warn().Err(err).Msg("failed to cancel pending downloads for series")
	} else if cancelled > 0 {
		log.Info().Int("count", cancelled).Str("seriesId", uid.String()).Msg("cancelled pending downloads for deleted series")
	}

	return c.JSON(http.StatusOK, nil)
}

// --- Conversion helpers ---

func toSeriesInfo(s *ent.Series, providers []*ent.SeriesProvider, baseURL string) types.SeriesInfo {
	info := types.SeriesInfo{
		ID:              s.ID.String(),
		Title:           s.Title,
		ThumbnailURL:    baseURL + s.ThumbnailURL,
		Artist:          s.Artist,
		Author:          s.Author,
		Description:     s.Description,
		Genre:           distinctPascalCase(s.Genre),
		Status:          types.SeriesStatus(s.Status),
		StoragePath:     s.StoragePath,
		Type:            s.Type,
		ChapterCount:    s.ChapterCount,
		IsActive:        false,
		PausedDownloads: s.PauseDownloads,
		Providers:       make([]types.SmallProviderInfo, 0),
	}

	if len(providers) == 0 {
		info.LastChangeProvider = &types.SmallProviderInfo{}
		return info
	}

	var lastChangeProvider *types.SmallProviderInfo
	var lastChangeTime time.Time
	var maxChapter *float64

	for _, p := range providers {
		if !p.IsDisabled && !p.IsUninstalled && !p.IsUnknown {
			info.IsActive = true
		}
		if p.IsUnknown {
			info.HasUnknown = true
		}

		if p.IsDisabled {
			continue
		}

		sm := types.SmallProviderInfo{
			Provider:   p.Provider,
			Scanlator:  p.Scanlator,
			Language:   p.Language,
			URL:        p.URL,
			Importance: p.Importance,
		}

		// Find last download date from chapters
		for _, ch := range p.Chapters {
			if ch.Filename != "" && ch.DownloadDate != nil {
				if ch.DownloadDate.After(lastChangeTime) {
					lastChangeTime = *ch.DownloadDate
					lcp := sm // copy
					lastChangeProvider = &lcp
				}
			}
		}

		info.Providers = append(info.Providers, sm)

		// Track max chapter
		for _, ch := range p.Chapters {
			if ch.Number != nil {
				if maxChapter == nil || *ch.Number > *maxChapter {
					n := *ch.Number
					maxChapter = &n
				}
			}
		}
	}

	info.LastChapter = maxChapter

	if lastChangeProvider != nil {
		info.LastChangeProvider = lastChangeProvider
		t := lastChangeTime.UTC().Format(time.RFC3339)
		info.LastChangeUTC = &t
	} else {
		info.LastChangeProvider = &types.SmallProviderInfo{}
		minTime := time.Time{}.UTC().Format(time.RFC3339)
		info.LastChangeUTC = &minTime
	}

	return info
}

func (h *SeriesHandler) toSeriesExtendedInfo(c echo.Context, s *ent.Series, providers []*ent.SeriesProvider, settings *types.Settings) types.SeriesExtendedInfo {
	baseURL := h.baseURL(c)

	info := types.SeriesExtendedInfo{
		ID:              s.ID.String(),
		Title:           s.Title,
		ThumbnailURL:    baseURL + s.ThumbnailURL,
		Artist:          s.Artist,
		Author:          s.Author,
		Description:     s.Description,
		Genre:           distinctPascalCase(s.Genre),
		Status:          types.SeriesStatus(s.Status),
		StoragePath:     s.StoragePath,
		Type:            s.Type,
		ChapterCount:    s.ChapterCount,
		IsActive:        false,
		PausedDownloads: s.PauseDownloads,
		Providers:       make([]types.ProviderExtendedInfo, 0, len(providers)),
	}

	storagePath := ""
	if settings != nil && s.StoragePath != "" {
		storagePath = settings.StorageFolder + "/" + s.StoragePath
	}
	info.Path = storagePath

	var lastChangeProvider *types.SmallProviderInfo
	var lastChangeTime time.Time
	var maxChapter *float64
	allChapterNumbers := make([]float64, 0)

	for _, p := range providers {
		if !p.IsDisabled && !p.IsUninstalled && !p.IsUnknown {
			info.IsActive = true
		}
		if p.IsUnknown {
			info.HasUnknown = true
		}

		// Rewrite provider thumbnail URL through proxy
		var provThumb *string
		if p.ThumbnailURL != nil && *p.ThumbnailURL != "" {
			thumb := *p.ThumbnailURL
			if !strings.HasPrefix(thumb, "http") {
				thumb = baseURL + thumb
			}
			provThumb = &thumb
		}

		pei := types.ProviderExtendedInfo{
			ID:                   p.ID.String(),
			Provider:             p.Provider,
			Scanlator:            p.Scanlator,
			Lang:                 p.Language,
			ThumbnailURL:         provThumb,
			Title:                p.Title,
			Artist:               derefStrPtr(p.Artist),
			Author:               derefStrPtr(p.Author),
			Description:          derefStrPtr(p.Description),
			Genre:                p.Genre,
			Type:                 s.Type,
			URL:                  p.URL,
			Meta:                 make(map[string]string),
			UseCover:             p.IsCover,
			Importance:           p.Importance,
			IsUnknown:            p.IsUnknown,
			UseTitle:             p.IsTitle,
			IsDisabled:           p.IsDisabled,
			IsUninstalled:        p.IsUninstalled,
			Status:               types.SeriesStatus(p.Status),
			ContinueAfterChapter: p.ContinueAfterChapter,
			MatchID:              p.ID.String(),
		}

		if p.ChapterCount != nil {
			pei.ChapterCount = *p.ChapterCount
		}

		// Find last chapter and download info
		var pMaxChapter *float64
		var pLastChangeTime time.Time
		chapterNums := make([]float64, 0)
		for _, ch := range p.Chapters {
			if ch.Number != nil {
				chapterNums = append(chapterNums, *ch.Number)
				if pMaxChapter == nil || *ch.Number > *pMaxChapter {
					n := *ch.Number
					pMaxChapter = &n
				}
				if maxChapter == nil || *ch.Number > *maxChapter {
					n := *ch.Number
					maxChapter = &n
				}
			}
			if ch.Filename != "" && ch.DownloadDate != nil {
				if ch.DownloadDate.After(pLastChangeTime) {
					pLastChangeTime = *ch.DownloadDate
				}
				if ch.DownloadDate.After(lastChangeTime) {
					lastChangeTime = *ch.DownloadDate
					lcp := types.SmallProviderInfo{
						Provider:   p.Provider,
						Scanlator:  p.Scanlator,
						Language:   p.Language,
						URL:        p.URL,
						Importance: p.Importance,
					}
					lastChangeProvider = &lcp
				}
			}
		}

		pei.LastChapter = pMaxChapter
		if !pLastChangeTime.IsZero() {
			pei.LastChangeUTC = pLastChangeTime.UTC().Format(time.RFC3339)
		}
		if p.FetchDate != nil {
			pei.LastUpdatedUTC = p.FetchDate.UTC().Format(time.RFC3339)
		}
		pei.ChapterList = formatChapterRangesFloat(chapterNums)

		allChapterNumbers = append(allChapterNumbers, chapterNums...)

		info.Providers = append(info.Providers, pei)
	}

	info.LastChapter = maxChapter
	info.ChapterList = formatChapterRangesFloat(allChapterNumbers)

	if lastChangeProvider != nil {
		info.LastChangeProvider = lastChangeProvider
		t := lastChangeTime.UTC().Format(time.RFC3339)
		info.LastChangeUTC = &t
	} else {
		info.LastChangeProvider = &types.SmallProviderInfo{}
		minTime := time.Time{}.UTC().Format(time.RFC3339)
		info.LastChangeUTC = &minTime
	}

	return info
}

func toLatestSeriesInfo(e *ent.LatestSeries, baseURL string) types.LatestSeriesInfo {
	info := types.LatestSeriesInfo{
		ID:                 strconv.Itoa(e.ID),
		SuwayomiSourceID:   e.SuwayomiSourceID,
		Provider:           e.Provider,
		Language:           e.Language,
		URL:                e.URL,
		Title:              e.Title,
		Artist:             e.Artist,
		Author:             e.Author,
		Description:        e.Description,
		Genre:              e.Genre,
		FetchDate:          e.FetchDate.UTC().Format(time.RFC3339),
		ChapterCount:       e.ChapterCount,
		LatestChapter:      e.LatestChapter,
		LatestChapterTitle: e.LatestChapterTitle,
		Status:             types.SeriesStatus(e.Status),
		InLibrary:          types.InLibraryStatus(e.InLibrary),
	}

	// Rewrite thumbnail URL
	if e.ThumbnailURL != nil && *e.ThumbnailURL != "" {
		thumb := *e.ThumbnailURL
		if !strings.HasPrefix(thumb, "http") {
			thumb = baseURL + thumb
		}
		info.ThumbnailURL = &thumb
	} else {
		thumb := baseURL + "serie/thumb/unknown"
		info.ThumbnailURL = &thumb
	}

	if e.SeriesID != nil {
		sid := e.SeriesID.String()
		info.SeriesID = &sid
	}

	return info
}

// --- Business logic helpers ---

type consolidatedData struct {
	title        string
	description  string
	thumbnailURL string
	artist       string
	author       string
	genre        []string
	status       string
	chapterCount int
}

func consolidateProviders(providers []*ent.SeriesProvider) consolidatedData {
	if len(providers) == 0 {
		return consolidatedData{status: string(types.SeriesStatusUnknown)}
	}

	// Find best provider (most complete data)
	best := providers[0]
	bestScore := countProviderFields(best)
	var bestCover string
	if best.ThumbnailURL != nil {
		bestCover = *best.ThumbnailURL
	}

	for _, p := range providers {
		score := countProviderFields(p)
		if score > bestScore {
			best = p
			bestScore = score
		}
		if p.IsCover && p.ThumbnailURL != nil && *p.ThumbnailURL != "" {
			bestCover = *p.ThumbnailURL
		}
	}

	// Consolidate genres
	genreSet := make(map[string]struct{})
	for _, p := range providers {
		for _, g := range p.Genre {
			genreSet[strings.ToLower(g)] = struct{}{}
		}
	}

	// Override with title/cover provider
	title := best.Title
	for _, p := range providers {
		if p.IsTitle {
			title = p.Title
			break
		}
	}
	for _, p := range providers {
		if p.IsCover && p.ThumbnailURL != nil {
			bestCover = *p.ThumbnailURL
			break
		}
	}

	desc := derefStrPtr(best.Description)
	if desc == "" {
		for _, p := range providers {
			if p.Description != nil && *p.Description != "" {
				desc = *p.Description
				break
			}
		}
	}

	artist := derefStrPtr(best.Artist)
	if artist == "" {
		for _, p := range providers {
			if p.Artist != nil && *p.Artist != "" {
				artist = *p.Artist
				break
			}
		}
	}

	author := derefStrPtr(best.Author)
	if author == "" {
		for _, p := range providers {
			if p.Author != nil && *p.Author != "" {
				author = *p.Author
				break
			}
		}
	}

	status := bestStatus(providers)

	maxCC := 0
	for _, p := range providers {
		if p.ChapterCount != nil && int(*p.ChapterCount) > maxCC {
			maxCC = int(*p.ChapterCount)
		}
	}

	return consolidatedData{
		title:        title,
		description:  desc,
		thumbnailURL: bestCover,
		artist:       artist,
		author:       author,
		genre:        distinctPascalCase(mapKeys(genreSet)),
		status:       status,
		chapterCount: maxCC,
	}
}

func consolidateFullSeries(series []types.FullSeries) types.FullSeries {
	if len(series) == 0 {
		return types.FullSeries{Status: types.SeriesStatusUnknown}
	}

	best := series[0]
	bestScore := countFullSeriesFields(&best)

	for _, fs := range series[1:] {
		score := countFullSeriesFields(&fs)
		if score > bestScore {
			best = fs
			bestScore = score
		}
	}

	// Override thumbnail with the cover provider's thumbnail (check all, including index 0)
	for _, fs := range series {
		if fs.UseCover && fs.ThumbnailURL != nil {
			best.ThumbnailURL = fs.ThumbnailURL
			break
		}
	}

	// Override title with the title provider's title
	for _, fs := range series {
		if fs.UseTitle && fs.Title != "" {
			best.Title = fs.Title
			break
		}
	}

	// Consolidate genres
	genreSet := make(map[string]struct{})
	for _, fs := range series {
		for _, g := range fs.Genre {
			genreSet[strings.ToLower(g)] = struct{}{}
		}
	}
	best.Genre = distinctPascalCase(mapKeys(genreSet))

	// Highest chapter count
	maxCC := best.ChapterCount
	for _, fs := range series {
		if fs.ChapterCount > maxCC {
			maxCC = fs.ChapterCount
		}
	}
	best.ChapterCount = maxCC

	// Fill empty fields
	if best.Description == "" {
		for _, fs := range series {
			if fs.Description != "" {
				best.Description = fs.Description
				break
			}
		}
	}
	if best.Author == "" {
		for _, fs := range series {
			if fs.Author != "" {
				best.Author = fs.Author
				break
			}
		}
	}
	if best.Artist == "" {
		for _, fs := range series {
			if fs.Artist != "" {
				best.Artist = fs.Artist
				break
			}
		}
	}

	// Override status with the most important (lowest importance number) provider's status
	bestImportance := -1
	for _, fs := range series {
		if fs.Status != types.SeriesStatusUnknown {
			if bestImportance < 0 || fs.Importance < bestImportance {
				best.Status = fs.Status
				bestImportance = fs.Importance
			}
		}
	}

	return best
}

func countProviderFields(p *ent.SeriesProvider) int {
	count := 0
	if p.Title != "" {
		count++
	}
	if p.Artist != nil && *p.Artist != "" {
		count++
	}
	if p.Author != nil && *p.Author != "" {
		count++
	}
	if p.Description != nil && *p.Description != "" {
		count++
	}
	if p.ThumbnailURL != nil && *p.ThumbnailURL != "" {
		count++
	}
	if len(p.Genre) > 0 {
		count += len(p.Genre)
	}
	if p.ChapterCount != nil && *p.ChapterCount > 0 {
		count++
	}
	return count
}

func countFullSeriesFields(fs *types.FullSeries) int {
	count := 0
	if fs.Title != "" {
		count++
	}
	if fs.Artist != "" {
		count++
	}
	if fs.Author != "" {
		count++
	}
	if fs.Description != "" {
		count++
	}
	if fs.ThumbnailURL != nil && *fs.ThumbnailURL != "" {
		count++
	}
	if len(fs.Genre) > 0 {
		count += len(fs.Genre)
	}
	if fs.ChapterCount > 0 {
		count++
	}
	return count
}

func bestStatus(providers []*ent.SeriesProvider) string {
	// Pick status from the highest-importance (lowest number) active provider
	best := (*ent.SeriesProvider)(nil)
	for _, p := range providers {
		if p.IsDisabled {
			continue
		}
		if p.Status == string(types.SeriesStatusUnknown) {
			continue
		}
		if best == nil || p.Importance < best.Importance {
			best = p
		}
	}
	if best != nil {
		return best.Status
	}
	// Fallback: first non-unknown status from any provider
	for _, p := range providers {
		if p.Status != string(types.SeriesStatusUnknown) {
			return p.Status
		}
	}
	if len(providers) > 0 {
		return providers[0].Status
	}
	return string(types.SeriesStatusUnknown)
}

// --- Utility helpers ---

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefStrPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func fileNameWithoutExt(filename string) string {
	ext := path.Ext(filename)
	return strings.TrimSuffix(filename, ext)
}

func maxChapterNumber(chapters []types.Chapter) *float64 {
	var max *float64
	for _, ch := range chapters {
		if ch.Number != nil {
			if max == nil || *ch.Number > *max {
				n := *ch.Number
				max = &n
			}
		}
	}
	return max
}

func allChaptersEmpty(chapters []types.Chapter) bool {
	for _, ch := range chapters {
		if ch.Filename != "" {
			return false
		}
	}
	return true
}

func hasDownloadedChapters(chapters []types.Chapter) bool {
	for _, ch := range chapters {
		if ch.Filename != "" {
			return true
		}
	}
	return false
}

func makeFileNameSafe(provider, scanlator, title, lang string, number *float64, name string, maxChapter *float64) string {
	provPart := makeFolderNameSafe(provider)
	if scanlator != "" && scanlator != provider {
		provPart += "-" + makeFolderNameSafe(scanlator)
	}

	titlePart := makeFolderNameSafe(title)

	// Determine zero-padding width
	width := 1
	if maxChapter != nil {
		width = len(fmt.Sprintf("%.0f", math.Floor(*maxChapter)))
	}
	if width < 1 {
		width = 1
	}

	numStr := "0"
	if number != nil {
		if *number == math.Floor(*number) {
			numStr = fmt.Sprintf("%0*d", width, int(*number))
		} else {
			numStr = fmt.Sprintf("%0*.1f", width+2, *number)
		}
	}

	result := fmt.Sprintf("[%s][%s] %s - %s", provPart, lang, titlePart, numStr)
	if name != "" {
		result += " (" + makeFolderNameSafe(name) + ")"
	}
	return result
}

func distinctPascalCase(items []string) []string {
	if items == nil {
		return []string{}
	}
	seen := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		lower := strings.ToLower(item)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		// PascalCase: capitalize first letter
		if item == "" {
			continue
		}
		result = append(result, strings.ToUpper(item[:1])+item[1:])
	}
	sort.Strings(result)
	return result
}

func mapKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func formatChapterRangesFloat(nums []float64) string {
	if len(nums) == 0 {
		return ""
	}

	sort.Float64s(nums)

	// Deduplicate
	deduped := make([]float64, 0, len(nums))
	for i, n := range nums {
		if i == 0 || n != nums[i-1] {
			deduped = append(deduped, n)
		}
	}

	type floatRange struct {
		start, end float64
	}

	ranges := []floatRange{{deduped[0], deduped[0]}}
	for i := 1; i < len(deduped); i++ {
		last := &ranges[len(ranges)-1]
		if deduped[i]-last.end <= 1.0 {
			last.end = deduped[i]
		} else {
			ranges = append(ranges, floatRange{deduped[i], deduped[i]})
		}
	}

	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r.start == r.end {
			parts = append(parts, formatFloat(r.start))
		} else {
			parts = append(parts, formatFloat(r.start)+"-"+formatFloat(r.end))
		}
	}

	return strings.Join(parts, ", ")
}

func formatFloat(f float64) string {
	if f == math.Floor(f) {
		return strconv.Itoa(int(f))
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// saveKaizokuJSON loads a series + providers and writes kaizoku.json to the storage directory.
func (h *SeriesHandler) saveKaizokuJSON(ctx context.Context, seriesID uuid.UUID, storageFolder string) {
	s, err := h.db.Series.Get(ctx, seriesID)
	if err != nil {
		log.Warn().Err(err).Str("seriesId", seriesID.String()).Msg("failed to load series for kaizoku.json")
		return
	}
	providers, err := s.QueryProviders().All(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to load providers for kaizoku.json")
		return
	}

	now := time.Now().UTC()
	info := types.KaizokuInfo{
		Title:          s.Title,
		Status:         types.SeriesStatus(s.Status),
		Artist:         s.Artist,
		Author:         s.Author,
		Description:    s.Description,
		Genre:          s.Genre,
		ChapterCount:   s.ChapterCount,
		LastUpdatedUTC: &now,
		IsDisabled:     s.PauseDownloads,
		KaizokuVersion: 1,
		Path:           s.StoragePath,
	}
	if s.Type != nil {
		info.Type = *s.Type
	}

	info.Providers = make([]types.ProviderInfo, 0, len(providers))
	for _, p := range providers {
		pi := types.ProviderInfo{
			Provider:   p.Provider,
			Language:   p.Language,
			Scanlator:  p.Scanlator,
			Title:      p.Title,
			Status:     types.SeriesStatus(p.Status),
			Importance: p.Importance,
			IsDisabled: p.IsDisabled,
		}
		if p.ThumbnailURL != nil {
			pi.ThumbnailURL = *p.ThumbnailURL
		}
		if p.ChapterCount != nil {
			pi.ChapterCount = int(*p.ChapterCount)
		}

		archives := make([]types.ArchiveInfo, 0)
		for _, ch := range p.Chapters {
			if ch.Filename != "" {
				archives = append(archives, types.ArchiveInfo{
					Filename:      ch.Filename,
					ChapterName:   ch.Name,
					ChapterNumber: ch.Number,
				})
			}
		}
		pi.Archives = archives
		info.Providers = append(info.Providers, pi)
	}

	dir := filepath.Join(storageFolder, s.StoragePath)
	if err := util.SaveKaizokuJSON(dir, &info); err != nil {
		log.Warn().Err(err).Str("dir", dir).Msg("failed to save kaizoku.json")
	}
}
