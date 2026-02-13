package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/technobecet/kaizoku-go/internal/types"
)

// Archive file extensions.
var archiveExts = map[string]bool{
	".cbz": true, ".cbr": true, ".zip": true, ".rar": true,
	".7z": true, ".pdf": true, ".epub": true,
}

// kaizokuRegex matches the Kaizoku filename format:
// [Provider][Language] Title ChapterNumber (ChapterName)
var kaizokuRegex = regexp.MustCompile(
	`^\[([^\]]+)\](?:\[([^\]]+)\])?\s+(.+?)(?:\s+(-?\d+(?:\.\d+)?))?\s*(?:\(([^)]+)\))?$`,
)

// IsArchive checks if a filename has an archive extension.
func IsArchive(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return archiveExts[ext]
}

// DetectedChapter represents a parsed archive file.
type DetectedChapter struct {
	Filename      string
	Provider      string
	Scanlator     string
	Language      string
	Title         string
	ChapterNumber *float64
	ChapterName   string
	IsKaizoku     bool
}

// ParseArchiveFilename parses a Kaizoku-format archive filename.
// Format: [Provider-Scanlator][Language] Title ChapterNumber (ChapterName).ext
func ParseArchiveFilename(filename string) DetectedChapter {
	// Remove extension for parsing
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	m := kaizokuRegex.FindStringSubmatch(name)
	if m == nil {
		return DetectedChapter{
			Filename: filename,
			Title:    name,
		}
	}

	provider := m[1]
	lang := m[2]
	title := m[3]
	chapterStr := m[4]
	chapterName := m[5]

	if lang == "" {
		lang = "en"
	}

	// Split provider by hyphen to get provider and scanlator
	scanlator := ""
	if idx := strings.Index(provider, "-"); idx > 0 {
		scanlator = provider[idx+1:]
		provider = provider[:idx]
	}

	var chapterNum *float64
	if chapterStr != "" {
		if n, err := strconv.ParseFloat(chapterStr, 64); err == nil {
			chapterNum = &n
		}
	}

	return DetectedChapter{
		Filename:      filename,
		Provider:      provider,
		Scanlator:     scanlator,
		Language:      lang,
		Title:         strings.TrimSpace(title),
		ChapterNumber: chapterNum,
		ChapterName:   chapterName,
		IsKaizoku:     true,
	}
}

// ScanDirectory recursively scans a directory for series folders containing archives.
// Returns one KaizokuInfo per series directory found. Matches .NET's
// Directory.GetDirectories(seriesFolder, "*.*", SearchOption.AllDirectories) behavior.
func ScanDirectory(rootPath string) ([]types.KaizokuInfo, error) {
	var results []types.KaizokuInfo

	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible directories
		}
		if !d.IsDir() || path == rootPath {
			return nil
		}

		// Compute the relative path from the storage root
		relPath, relErr := filepath.Rel(rootPath, path)
		if relErr != nil {
			return nil
		}

		info := scanSeriesDir(path, relPath)
		if info != nil {
			results = append(results, *info)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}

// scanSeriesDir scans a single series directory for archive files.
// relPath is the path relative to the storage root (e.g. "Manga/Naruto").
func scanSeriesDir(dirPath, relPath string) *types.KaizokuInfo {
	// Check for existing kaizoku.json
	existing := LoadKaizokuJSON(dirPath)

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil
	}

	var chapters []DetectedChapter
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !IsArchive(entry.Name()) {
			continue
		}
		ch := ParseArchiveFilename(entry.Name())
		chapters = append(chapters, ch)
	}

	if len(chapters) == 0 {
		return nil
	}

	// Use the leaf directory name for display title (not the full relative path)
	dirName := filepath.Base(relPath)

	// Group chapters by provider+scanlator+language
	type provKey struct {
		Provider  string
		Scanlator string
		Language  string
	}
	provGroups := make(map[provKey][]DetectedChapter)
	for _, ch := range chapters {
		key := provKey{Provider: ch.Provider, Scanlator: ch.Scanlator, Language: ch.Language}
		provGroups[key] = append(provGroups[key], ch)
	}

	var providers []types.ProviderInfo
	for key, chs := range provGroups {
		var archives []types.ArchiveInfo
		for _, ch := range chs {
			archives = append(archives, types.ArchiveInfo{
				Filename:      ch.Filename,
				ChapterName:   ch.ChapterName,
				ChapterNumber: ch.ChapterNumber,
			})
		}

		provider := key.Provider
		if provider == "" {
			provider = "Unknown"
		}

		title := dirName
		if len(chs) > 0 && chs[0].IsKaizoku && chs[0].Title != "" {
			title = chs[0].Title
		}

		providers = append(providers, types.ProviderInfo{
			Provider:     provider,
			Language:     key.Language,
			Scanlator:    key.Scanlator,
			Title:        title,
			ChapterCount: len(archives),
			Archives:     archives,
		})
	}

	// Use existing metadata if available
	title := dirName
	if existing != nil && existing.Title != "" {
		title = existing.Title
	} else if len(chapters) > 0 && chapters[0].IsKaizoku && chapters[0].Title != "" {
		title = chapters[0].Title
	}

	info := &types.KaizokuInfo{
		Title:        title,
		Status:       types.SeriesStatusUnknown,
		ChapterCount: len(chapters),
		Providers:    providers,
		Path:         relPath,
	}

	if existing != nil {
		// Preserve metadata from existing kaizoku.json
		if existing.Artist != "" {
			info.Artist = existing.Artist
		}
		if existing.Author != "" {
			info.Author = existing.Author
		}
		if existing.Description != "" {
			info.Description = existing.Description
		}
		if len(existing.Genre) > 0 {
			info.Genre = existing.Genre
		}
		if existing.Type != "" {
			info.Type = existing.Type
		}
		if existing.Status != types.SeriesStatusUnknown {
			info.Status = existing.Status
		}
	}

	return info
}

// LoadKaizokuJSON loads metadata from a kaizoku.json file if it exists.
func LoadKaizokuJSON(dirPath string) *types.KaizokuInfo {
	jsonPath := filepath.Join(dirPath, "kaizoku.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil
	}
	var info types.KaizokuInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil
	}
	return &info
}

// MakeFolderNameSafe removes unsafe characters from a folder name.
func MakeFolderNameSafe(name string) string {
	unsafe := regexp.MustCompile(`[<>:"/\\|?*]`)
	return strings.TrimSpace(unsafe.ReplaceAllString(name, ""))
}
