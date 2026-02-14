package util

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/technobecet/kaizoku-go/internal/types"
)

// CreateCBZ creates a CBZ (ZIP) archive from a set of page images and ComicInfo.xml.
// Pages are stored uncompressed; ComicInfo.xml is deflated.
func CreateCBZ(destPath string, pages []PageData, comicInfo *ComicInfo) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Write to temp file first for atomicity
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		f.Close()
		os.Remove(tmpPath) // Clean up if still exists
	}()

	w := zip.NewWriter(f)

	// Add pages (uncompressed for fast access)
	for _, page := range pages {
		header := &zip.FileHeader{
			Name:   page.Filename,
			Method: zip.Store, // No compression for images
		}
		entry, err := w.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("create page entry: %w", err)
		}
		if _, err := entry.Write(page.Data); err != nil {
			return fmt.Errorf("write page data: %w", err)
		}
	}

	// Add ComicInfo.xml (deflated)
	if comicInfo != nil {
		xmlData, err := MarshalComicInfo(*comicInfo)
		if err != nil {
			return fmt.Errorf("marshal ComicInfo: %w", err)
		}

		header := &zip.FileHeader{
			Name:   "ComicInfo.xml",
			Method: zip.Deflate,
		}
		entry, err := w.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("create ComicInfo entry: %w", err)
		}
		if _, err := entry.Write(xmlData); err != nil {
			return fmt.Errorf("write ComicInfo: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename to final path: %w", err)
	}

	return nil
}

// PageData holds data for a single page image.
type PageData struct {
	Filename string
	Data     []byte
}

// DetectImageExtension detects the image format from data bytes.
func DetectImageExtension(data []byte) string {
	ct := http.DetectContentType(data)
	switch {
	case strings.Contains(ct, "jpeg"):
		return ".jpg"
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "bmp"):
		return ".bmp"
	default:
		// Try manual detection for formats Go doesn't recognize
		return detectByMagic(data)
	}
}

func detectByMagic(data []byte) string {
	if len(data) < 12 {
		return ".bin"
	}
	// WebP: RIFF....WEBP
	if bytes.HasPrefix(data, []byte("RIFF")) && string(data[8:12]) == "WEBP" {
		return ".webp"
	}
	// AVIF: ....ftypavif
	if len(data) >= 12 && string(data[4:12]) == "ftypavif" {
		return ".avif"
	}
	// JXL
	if len(data) >= 12 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0x0C &&
		data[4] == 0x4A && data[5] == 0x58 && data[6] == 0x4C && data[7] == 0x20 {
		return ".jxl"
	}
	return ".bin"
}

// GenerateCBZFilename creates a safe filename for a CBZ archive.
// Format: [Provider-Scanlator][Language] Series ChapterNumber (ChapterName).cbz
func GenerateCBZFilename(provider, scanlator, language, title string, chapterNum *float64, chapterName string, maxChapter *float64) string {
	// Sanitize provider
	prov := strings.ReplaceAll(provider, "-", "_")
	if scanlator != "" && scanlator != provider {
		prov += "-" + scanlator
	}
	prov = strings.ReplaceAll(prov, "[", "(")
	prov = strings.ReplaceAll(prov, "]", ")")

	// Language tag
	lang := ""
	if language != "" {
		lang = "[" + strings.ToLower(language) + "]"
	}

	// Chapter number with zero-padding
	chapterStr := ""
	if chapterNum != nil {
		chapterStr = FormatChapterNumber(*chapterNum)
		if maxChapter != nil {
			maxLen := len(fmt.Sprintf("%d", int(*maxChapter)))
			// Pad the integer part
			parts := strings.SplitN(chapterStr, ".", 2)
			for len(parts[0]) < maxLen {
				parts[0] = "0" + parts[0]
			}
			chapterStr = strings.Join(parts, ".")
		}
	}

	// Chapter title
	chapTitle := ""
	if chapterName != "" {
		cleaned := strings.TrimSpace(chapterName)
		cleaned = strings.ReplaceAll(cleaned, "(", "[")
		cleaned = strings.ReplaceAll(cleaned, ")", "]")
		if !isTitleChapter(cleaned) {
			chapTitle = " (" + cleaned + ")"
		}
	}

	// Clean title
	title = strings.ReplaceAll(title, "(", "")
	title = strings.ReplaceAll(title, ")", "")

	// Assemble filename
	name := fmt.Sprintf("[%s]%s %s%s %s", prov, lang, strings.TrimSpace(title), chapTitle, chapterStr)

	// Sanitize
	name = sanitizeFilename(name)
	name = collapseSpaces(name)

	return name + ".cbz"
}

// GeneratePageFilename creates a safe filename for a page within a CBZ.
func GeneratePageFilename(provider, scanlator, language, title string, chapterNum *float64, chapterName string, maxChapter *float64, pageNum, maxPages int, ext string) string {
	base := GenerateCBZFilename(provider, scanlator, language, title, chapterNum, chapterName, maxChapter)
	base = strings.TrimSuffix(base, ".cbz")

	// Zero-pad page number
	maxPadLen := len(fmt.Sprintf("%d", maxPages))
	pageStr := fmt.Sprintf("%0*d", maxPadLen, pageNum)

	return base + " " + pageStr + ext
}

var multiSpaceRe = regexp.MustCompile(`\s+`)

func sanitizeFilename(name string) string {
	return ReplaceInvalidPathCharacters(name)
}

func collapseSpaces(s string) string {
	return strings.TrimSpace(multiSpaceRe.ReplaceAllString(s, " "))
}

// isTitleChapter checks if a chapter name is just "Chapter X" which is redundant.
func isTitleChapter(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(lower, "chapter ") ||
		strings.HasPrefix(lower, "ch. ") ||
		strings.HasPrefix(lower, "ch ")
}

// UpdateCBZComicInfo replaces the ComicInfo.xml inside an existing CBZ archive.
func UpdateCBZComicInfo(cbzPath string, comicInfo ComicInfo) error {
	reader, err := zip.OpenReader(cbzPath)
	if err != nil {
		return fmt.Errorf("open cbz: %w", err)
	}
	defer reader.Close()

	tmpPath := cbzPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		f.Close()
		os.Remove(tmpPath)
	}()

	w := zip.NewWriter(f)

	// Copy all entries except ComicInfo.xml
	for _, entry := range reader.File {
		if entry.Name == "ComicInfo.xml" {
			continue
		}
		header := entry.FileHeader
		writer, err := w.CreateHeader(&header)
		if err != nil {
			return fmt.Errorf("create entry %s: %w", entry.Name, err)
		}
		rc, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open entry %s: %w", entry.Name, err)
		}
		if _, err := io.Copy(writer, rc); err != nil {
			rc.Close()
			return fmt.Errorf("copy entry %s: %w", entry.Name, err)
		}
		rc.Close()
	}

	// Add new ComicInfo.xml
	xmlData, err := MarshalComicInfo(comicInfo)
	if err != nil {
		return fmt.Errorf("marshal ComicInfo: %w", err)
	}
	header := &zip.FileHeader{
		Name:   "ComicInfo.xml",
		Method: zip.Deflate,
	}
	entry, err := w.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create ComicInfo entry: %w", err)
	}
	if _, err := entry.Write(xmlData); err != nil {
		return fmt.Errorf("write ComicInfo: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}
	reader.Close()
	if err := f.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	return os.Rename(tmpPath, cbzPath)
}

// CheckArchive validates a CBZ file and returns its integrity status.
func CheckArchive(path string) types.ArchiveResult {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return types.ArchiveResultNotFound
	}

	r, err := zip.OpenReader(path)
	if err != nil {
		return types.ArchiveResultNotAnArchive
	}
	defer r.Close()

	hasImages := false
	for _, f := range r.File {
		lower := strings.ToLower(f.Name)
		if isImageFile(lower) {
			hasImages = true
			break
		}
	}

	if !hasImages {
		return types.ArchiveResultNoImages
	}

	return types.ArchiveResultFine
}

// CountCBZPages counts the number of image files inside a CBZ archive.
// Returns 0 if the file cannot be read.
func CountCBZPages(path string) int {
	r, err := zip.OpenReader(path)
	if err != nil {
		return 0
	}
	defer r.Close()

	count := 0
	for _, f := range r.File {
		if isImageFile(strings.ToLower(f.Name)) {
			count++
		}
	}
	return count
}

// isImageFile checks if a filename has an image extension.
func isImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".avif", ".jxl":
		return true
	}
	return false
}

// ReadComicInfoFromCBZ reads and parses the ComicInfo.xml from inside a CBZ archive.
// Returns nil if no ComicInfo.xml is found.
func ReadComicInfoFromCBZ(path string) (*ComicInfo, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open cbz: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.EqualFold(f.Name, "ComicInfo.xml") {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open ComicInfo.xml: %w", err)
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("read ComicInfo.xml: %w", err)
			}

			var ci ComicInfo
			if err := xml.Unmarshal(data, &ci); err != nil {
				return nil, fmt.Errorf("unmarshal ComicInfo.xml: %w", err)
			}
			return &ci, nil
		}
	}

	return nil, nil // No ComicInfo.xml found
}

// SaveKaizokuJSON writes a KaizokuInfo struct as kaizoku.json to the given directory.
func SaveKaizokuJSON(dir string, info *types.KaizokuInfo) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal kaizoku info: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "kaizoku.json"), data, 0o644)
}
