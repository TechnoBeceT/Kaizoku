package handler

import (
	"context"
	"errors"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	settingssvc "github.com/technobecet/kaizoku-go/internal/service/settings"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
	"github.com/technobecet/kaizoku-go/internal/types"
	"github.com/technobecet/kaizoku-go/internal/util"
)

type SearchHandler struct {
	config   *config.Config
	db       *ent.Client
	suwayomi *suwayomi.Client
	settings *settingssvc.Service

	cacheMu sync.RWMutex
	cache   map[string]searchCacheEntry
}

type searchCacheEntry struct {
	results   []types.LinkedSeries
	expiresAt time.Time
}

// getSearchConcurrency returns the number of simultaneous searches from DB settings, falling back to 10.
func (h *SearchHandler) getSearchConcurrency(ctx context.Context) int {
	if h.settings != nil {
		if s, err := h.settings.Get(ctx); err == nil && s != nil && s.NumberOfSimultaneousSearches > 0 {
			return s.NumberOfSimultaneousSearches
		}
	}
	return 10
}

func (h *SearchHandler) SearchSeries(c echo.Context) error {
	keyword := c.QueryParam("keyword")
	if keyword == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "keyword is required"})
	}

	languagesParam := c.QueryParam("languages")
	var languages []string
	if languagesParam != "" {
		languages = strings.Split(languagesParam, ",")
	}

	searchSources := c.QueryParams()["searchSources"]

	ctx := c.Request().Context()

	// Default to preferred languages from settings when no languages param provided
	if len(languages) == 0 && len(searchSources) == 0 {
		if dbSettings, err := h.settings.Get(ctx); err == nil && dbSettings != nil && len(dbSettings.PreferredLanguages) > 0 {
			languages = dbSettings.PreferredLanguages
		} else if len(h.config.Settings.PreferredLanguages) > 0 {
			languages = h.config.Settings.PreferredLanguages
		}
	}

	// Check cache (include languages in key to avoid serving wrong cached results)
	cacheKey := "S" + keyword + "_" + strings.Join(searchSources, ",") + "_L" + strings.Join(languages, ",")
	h.cacheMu.RLock()
	if h.cache != nil {
		if entry, ok := h.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
			h.cacheMu.RUnlock()
			return c.JSON(http.StatusOK, entry.results)
		}
	}
	h.cacheMu.RUnlock()

	// No-retry context: search should fail fast, retries are only for downloads
	noRetryCtx := suwayomi.WithNoRetry(ctx)

	// Get sources filtered by language
	allSources, err := h.suwayomi.GetSources(noRetryCtx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Filter by languages
	var filteredSources []suwayomi.SuwayomiSource
	if len(languages) > 0 {
		langSet := make(map[string]struct{})
		for _, l := range languages {
			langSet[strings.ToLower(l)] = struct{}{}
		}
		for _, s := range allSources {
			if _, ok := langSet[strings.ToLower(s.Lang)]; ok || s.Lang == "all" {
				filteredSources = append(filteredSources, s)
			}
		}
	} else {
		filteredSources = allSources
	}

	// Further filter by specific search sources
	if len(searchSources) > 0 {
		srcSet := make(map[string]struct{})
		for _, s := range searchSources {
			srcSet[s] = struct{}{}
		}
		var filtered []suwayomi.SuwayomiSource
		for _, s := range filteredSources {
			if _, ok := srcSet[s.ID]; ok {
				filtered = append(filtered, s)
			}
		}
		filteredSources = filtered
	}

	// Parallel search across sources
	type searchResult struct {
		source suwayomi.SuwayomiSource
		series []suwayomi.SuwayomiSeries
	}

	var mu sync.Mutex
	var results []searchResult
	sem := make(chan struct{}, h.getSearchConcurrency(noRetryCtx))
	var wg sync.WaitGroup

	for _, src := range filteredSources {
		wg.Add(1)
		go func(source suwayomi.SuwayomiSource) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			srcStart := time.Now()
			res, err := h.suwayomi.SearchSeries(noRetryCtx, source.ID, keyword, 1)
			srcDuration := time.Since(srcStart).Milliseconds()
			if err != nil {
				log.Warn().Err(err).Str("source", source.Name).Msg("search error")
				// Skip logging if HTTP request context was canceled (user navigated away)
				if !errors.Is(err, context.Canceled) {
					util.LogSourceEvent(h.db, source.ID, source.Name, strings.ToLower(source.Lang),
						"search", "failed", srcDuration,
						util.WithError(err), util.WithMetadata(map[string]string{"keyword": keyword, "origin": "http_request"}))
				}
				return
			}
			count := 0
			if res != nil && len(res.MangaList) > 0 {
				// Dedup within source
				seen := make(map[int]struct{})
				var unique []suwayomi.SuwayomiSeries
				for _, s := range res.MangaList {
					if _, ok := seen[s.ID]; !ok {
						seen[s.ID] = struct{}{}
						unique = append(unique, s)
					}
				}
				count = len(unique)
				mu.Lock()
				results = append(results, searchResult{source: source, series: unique})
				mu.Unlock()
			}
			util.LogSourceEvent(h.db, source.ID, source.Name, strings.ToLower(source.Lang),
				"search", "success", srcDuration,
				util.WithItemsCount(count), util.WithMetadata(map[string]string{"keyword": keyword, "origin": "http_request"}))
		}(src)
	}
	wg.Wait()

	// Build source map for enrichment
	sourceMap := make(map[string]suwayomi.SuwayomiSource)
	for _, s := range filteredSources {
		sourceMap[s.ID] = s
	}

	// Collect all series and create LinkedSeries
	var allSeries []suwayomi.SuwayomiSeries
	for _, r := range results {
		allSeries = append(allSeries, r.series...)
	}

	linked := findAndLinkSimilar(allSeries, 0.1)

	// Enrich with provider info
	for i := range linked {
		if src, ok := sourceMap[linked[i].ProviderID]; ok {
			linked[i].Provider = src.Name
			linked[i].Lang = strings.ToLower(src.Lang)
		}
		linked[i].ThumbnailURL = strPtr("serie/thumb/" + linked[i].ID)
	}

	// Deduplicate by ID
	seen := make(map[string]struct{})
	var deduped []types.LinkedSeries
	for _, ls := range linked {
		if _, ok := seen[ls.ID]; !ok {
			seen[ls.ID] = struct{}{}
			deduped = append(deduped, ls)
		}
	}

	// Sort by Levenshtein distance to keyword
	sort.Slice(deduped, func(i, j int) bool {
		di := levenshtein(normalizeTitle(deduped[i].Title), normalizeTitle(keyword))
		dj := levenshtein(normalizeTitle(deduped[j].Title), normalizeTitle(keyword))
		return di < dj
	})

	if deduped == nil {
		deduped = []types.LinkedSeries{}
	}

	// Cache for 30 seconds
	h.cacheMu.Lock()
	if h.cache == nil {
		h.cache = make(map[string]searchCacheEntry)
	}
	h.cache[cacheKey] = searchCacheEntry{
		results:   deduped,
		expiresAt: time.Now().Add(30 * time.Second),
	}
	h.cacheMu.Unlock()

	return c.JSON(http.StatusOK, deduped)
}

func (h *SearchHandler) GetSearchSources(c echo.Context) error {
	ctx := c.Request().Context()

	sources, err := h.suwayomi.GetSources(suwayomi.WithNoRetry(ctx))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Filter by preferred languages from settings (matching .NET SearchQueryService behavior)
	var languages []string
	if dbSettings, err := h.settings.Get(ctx); err == nil && dbSettings != nil && len(dbSettings.PreferredLanguages) > 0 {
		languages = dbSettings.PreferredLanguages
	} else if len(h.config.Settings.PreferredLanguages) > 0 {
		languages = h.config.Settings.PreferredLanguages
	}
	// Default to English if no preferred languages configured
	if len(languages) == 0 {
		languages = []string{"en"}
	}

	langSet := make(map[string]struct{})
	for _, l := range languages {
		langSet[strings.ToLower(l)] = struct{}{}
	}

	var result []types.SearchSource
	for _, s := range sources {
		lang := strings.ToLower(s.Lang)
		if _, ok := langSet[lang]; !ok && lang != "all" {
			continue
		}
		result = append(result, types.SearchSource{
			SourceID:       s.ID,
			SourceName:     s.DisplayName,
			Language:       lang,
			SupportsLatest: s.SupportsLatest,
		})
	}

	if result == nil {
		result = []types.SearchSource{}
	}

	return c.JSON(http.StatusOK, result)
}

func (h *SearchHandler) AugmentSeries(c echo.Context) error {
	var linkedSeries []types.LinkedSeries
	if err := c.Bind(&linkedSeries); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if len(linkedSeries) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no series provided"})
	}

	ctx := suwayomi.WithNoRetry(c.Request().Context())

	// Parallel fetch full data for each series
	type augResult struct {
		linked  types.LinkedSeries
		details *suwayomi.SuwayomiSeries
	}

	var mu sync.Mutex
	var fetched []augResult
	sem := make(chan struct{}, h.getSearchConcurrency(ctx))
	var wg sync.WaitGroup

	for _, ls := range linkedSeries {
		wg.Add(1)
		go func(ls types.LinkedSeries) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			id, err := strconv.Atoi(ls.ID)
			if err != nil {
				return
			}

			full, err := h.suwayomi.GetFullSeriesData(ctx, id, true)
			if err != nil {
				log.Warn().Err(err).Str("id", ls.ID).Msg("failed to fetch full data")
				return
			}

			chapters, err := h.suwayomi.GetChapters(ctx, id, true)
			if err != nil {
				log.Warn().Err(err).Str("id", ls.ID).Msg("failed to fetch chapters")
				return
			}

			if full != nil && len(chapters) > 0 {
				// Set default scanlator
				for i := range chapters {
					if chapters[i].Scanlator == nil || *chapters[i].Scanlator == "" {
						s := ls.Provider
						chapters[i].Scanlator = &s
					}
				}
				full.Chapters = chapters
				mu.Lock()
				fetched = append(fetched, augResult{linked: ls, details: full})
				mu.Unlock()
			}
		}(ls)
	}
	wg.Wait()

	// Convert to FullSeries grouped by scanlator
	var fullSeriesList []map[string]interface{}

	for _, item := range fetched {
		details := item.details
		ls := item.linked

		// Group chapters by scanlator
		groups := make(map[string][]suwayomi.SuwayomiChapter)
		for _, ch := range details.Chapters {
			scanlator := ls.Provider
			if ch.Scanlator != nil && *ch.Scanlator != "" {
				scanlator = *ch.Scanlator
			}
			groups[scanlator] = append(groups[scanlator], ch)
		}

		for scanlator, chapters := range groups {
			// Sort by index
			sort.Slice(chapters, func(i, j int) bool {
				return chapters[i].Index < chapters[j].Index
			})

			// Convert chapters
			var chapterList []map[string]interface{}
			for _, ch := range chapters {
				chapterMap := map[string]interface{}{
					"name":               ch.Name,
					"number":             ch.ChapterNumber,
					"providerUploadDate": time.UnixMilli(ch.UploadDate).UTC().Format(time.RFC3339),
					"url":                ch.RealURL,
					"providerIndex":      ch.Index,
					"shouldDownload":     true,
					"isDeleted":          false,
					"pageCount":          ch.PageCount,
				}
				chapterList = append(chapterList, chapterMap)
			}

			artist := ""
			if details.Artist != nil {
				artist = *details.Artist
			}
			author := ""
			if details.Author != nil {
				author = *details.Author
			}
			desc := ""
			if details.Description != nil {
				desc = *details.Description
			}

			fs := map[string]interface{}{
				"id":                ls.ID,
				"providerId":       ls.ProviderID,
				"provider":         ls.Provider,
				"scanlator":        scanlator,
				"lang":             ls.Lang,
				"thumbnailUrl":     "serie/thumb/" + ls.ID,
				"title":            details.Title,
				"artist":           artist,
				"author":           author,
				"description":      desc,
				"genre":            details.Genre,
				"chapterCount":     len(chapters),
				"url":              details.RealURL,
				"meta":             details.Meta,
				"useCover":         ls.UseCover,
				"importance":       0,
				"isUnknown":        false,
				"useTitle":         false,
				"existingProvider":  false,
				"suggestedFilename": makeFolderNameSafe(details.Title),
				"chapters":         chapterList,
				"status":           details.Status,
				"chapterList":      formatChapterRanges(chapters),
				"isSelected":       false,
			}

			if len(chapters) > 0 {
				fs["lastUpdatedUTC"] = time.UnixMilli(chapters[0].UploadDate).UTC().Format(time.RFC3339)
			}

			fullSeriesList = append(fullSeriesList, fs)
		}
	}

	if fullSeriesList == nil {
		fullSeriesList = []map[string]interface{}{}
	}

	// Fetch categories and preferred languages from settings DB (not static config)
	dbSettings, err := h.settings.Get(ctx)
	categories := h.config.Settings.Categories
	preferredLanguages := h.config.Settings.PreferredLanguages
	categorizedFolders := true
	if err == nil && dbSettings != nil {
		categories = dbSettings.Categories
		preferredLanguages = dbSettings.PreferredLanguages
		categorizedFolders = dbSettings.CategorizedFolders
	}

	resp := map[string]interface{}{
		"storageFolderPath":    h.config.Storage.Folder,
		"useCategoriesForPath": categorizedFolders,
		"existingSeries":       false,
		"existingSeriesId":     nil,
		"categories":           categories,
		"series":               fullSeriesList,
		"preferredLanguages":   preferredLanguages,
		"disableJobs":          false,
	}

	return c.JSON(http.StatusOK, resp)
}

// --- Linking & similarity helpers ---

func findAndLinkSimilar(series []suwayomi.SuwayomiSeries, threshold float64) []types.LinkedSeries {
	if len(series) == 0 {
		return nil
	}

	// Stage 1: Group by normalized title
	groups := make(map[string][]suwayomi.SuwayomiSeries)
	for _, s := range series {
		if s.Title == "" {
			continue
		}
		key := normalizeTitle(s.Title)
		groups[key] = append(groups[key], s)
	}

	var linked []types.LinkedSeries
	for _, group := range groups {
		allIDs := make([]string, len(group))
		for i, s := range group {
			allIDs[i] = strconv.Itoa(s.ID)
		}

		for _, s := range group {
			ls := types.LinkedSeries{
				ID:         strconv.Itoa(s.ID),
				ProviderID: s.SourceID,
				Title:      s.Title,
				LinkedIDs:  allIDs,
			}
			linked = append(linked, ls)
		}
	}

	// Stage 2: Fuzzy merge
	mergeSimilar(linked, threshold)
	return linked
}

func mergeSimilar(linked []types.LinkedSeries, threshold float64) {
	if len(linked) <= 1 {
		return
	}

	similarityGroups := make(map[string]map[string]struct{})

	for i := 0; i < len(linked); i++ {
		for j := i + 1; j < len(linked); j++ {
			// Skip if already linked
			if shareIDs(linked[i].LinkedIDs, linked[j].LinkedIDs) {
				continue
			}

			if areSimilar(linked[i].Title, linked[j].Title, threshold) {
				// Merge groups
				g1, ok1 := similarityGroups[linked[i].ID]
				if !ok1 {
					g1 = make(map[string]struct{})
					for _, id := range linked[i].LinkedIDs {
						g1[id] = struct{}{}
					}
					similarityGroups[linked[i].ID] = g1
				}
				g2, ok2 := similarityGroups[linked[j].ID]
				if !ok2 {
					g2 = make(map[string]struct{})
					for _, id := range linked[j].LinkedIDs {
						g2[id] = struct{}{}
					}
					similarityGroups[linked[j].ID] = g2
				}
				for id := range g2 {
					g1[id] = struct{}{}
				}
				for id := range g1 {
					g2[id] = struct{}{}
				}
			}
		}
	}

	// Apply groups
	for i := range linked {
		if group, ok := similarityGroups[linked[i].ID]; ok {
			ids := make([]string, 0, len(group))
			for id := range group {
				if id != linked[i].ID {
					ids = append(ids, id)
				}
			}
			linked[i].LinkedIDs = ids
		}
	}
}

func shareIDs(a, b []string) bool {
	set := make(map[string]struct{}, len(a))
	for _, id := range a {
		set[id] = struct{}{}
	}
	for _, id := range b {
		if _, ok := set[id]; ok {
			return true
		}
	}
	return false
}

var nonAlphaNum = regexp.MustCompile(`[^\w\s]`)
var multiSpace = regexp.MustCompile(`\s+`)

func normalizeTitle(title string) string {
	s := strings.ToLower(title)
	for _, word := range []string{"the ", "a ", "an ", "of ", "in ", "on ", "at ", "by "} {
		s = strings.ReplaceAll(s, word, " ")
	}
	s = nonAlphaNum.ReplaceAllString(s, "")
	s = multiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func areSimilar(a, b string, threshold float64) bool {
	na := normalizeTitle(a)
	nb := normalizeTitle(b)
	if na == "" || nb == "" {
		return false
	}
	dist := levenshtein(na, nb)
	maxLen := math.Max(float64(len(na)), float64(len(nb)))
	if maxLen == 0 {
		return true
	}
	return float64(dist)/maxLen <= threshold
}

func levenshtein(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,
				min(matrix[i][j-1]+1, matrix[i-1][j-1]+cost),
			)
		}
	}
	return matrix[len(s1)][len(s2)]
}

func makeFolderNameSafe(name string) string {
	unsafe := regexp.MustCompile(`[<>:"/\\|?*]`)
	return strings.TrimSpace(unsafe.ReplaceAllString(name, ""))
}

func formatChapterRanges(chapters []suwayomi.SuwayomiChapter) string {
	var nums []float64
	for _, ch := range chapters {
		if ch.ChapterNumber != nil {
			nums = append(nums, *ch.ChapterNumber)
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
			ranges = append(ranges, formatRange(start, end))
			start = nums[i]
			end = nums[i]
		}
	}
	ranges = append(ranges, formatRange(start, end))
	return strings.Join(ranges, ", ")
}

func formatRange(start, end float64) string {
	if start == end {
		return formatNum(start)
	}
	return formatNum(start) + "-" + formatNum(end)
}

func formatNum(n float64) string {
	if n == math.Floor(n) {
		return strconv.Itoa(int(n))
	}
	return strconv.FormatFloat(n, 'f', 1, 64)
}

func strPtr(s string) *string {
	return &s
}
