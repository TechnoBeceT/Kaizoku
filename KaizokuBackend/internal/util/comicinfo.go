package util

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// ComicInfo represents the ComicInfo.xml metadata in a CBZ file.
// Compatible with Kavita/Komga readers.
type ComicInfo struct {
	XMLName         xml.Name `xml:"ComicInfo"`
	Title           string   `xml:"Title,omitempty"`
	Series          string   `xml:"Series,omitempty"`
	LocalizedSeries string   `xml:"LocalizedSeries,omitempty"`
	Number          string   `xml:"Number,omitempty"`
	Count           int      `xml:"Count,omitempty"`
	PageCount       int      `xml:"PageCount,omitempty"`
	Format          string   `xml:"Format,omitempty"`
	LanguageISO     string   `xml:"LanguageISO,omitempty"`
	Tags            string   `xml:"Tags,omitempty"`
	AgeRating       string   `xml:"AgeRating,omitempty"`
	Web             string   `xml:"Web,omitempty"`
	Writer          string   `xml:"Writer,omitempty"`
	Publisher       string   `xml:"Publisher,omitempty"`
	Translator      string   `xml:"Translator,omitempty"`
	CoverArtist     string   `xml:"CoverArtist,omitempty"`
	Day             int      `xml:"Day,omitempty"`
	Month           int      `xml:"Month,omitempty"`
	Year            int      `xml:"Year,omitempty"`
	Manga           string   `xml:"Manga,omitempty"`
	Notes           string   `xml:"Notes,omitempty"`
}

// ChapterMeta holds the metadata needed to generate a ComicInfo.xml.
type ChapterMeta struct {
	Title         string
	SeriesTitle   string
	ProviderTitle string
	ChapterNumber *float64
	ChapterName   string
	ChapterCount  int
	PageCount     int
	Language      string
	Provider      string
	Scanlator     string
	Author        string
	Artist        string
	Genre         []string
	Type          string
	URL           string
	UploadDate    *time.Time
}

// NewComicInfo generates a ComicInfo from chapter metadata.
func NewComicInfo(meta ChapterMeta) ComicInfo {
	chapName := strings.TrimSpace(meta.ChapterName)
	if chapName == "" && meta.ChapterNumber != nil {
		chapName = "Chapter " + FormatChapterNumber(*meta.ChapterNumber)
	}

	ci := ComicInfo{
		Title:           chapName,
		Series:          meta.ProviderTitle,
		LocalizedSeries: meta.SeriesTitle,
		Format:          "Web",
		LanguageISO:     strings.ToLower(meta.Language),
		PageCount:       meta.PageCount,
		Writer:          strings.TrimSpace(meta.Author),
		Publisher:       meta.Provider,
		Translator:      meta.Scanlator,
		CoverArtist:     strings.TrimSpace(meta.Artist),
		Notes:           "Created by Kaizoku.GO",
	}

	if meta.ChapterNumber != nil {
		ci.Number = FormatChapterNumber(*meta.ChapterNumber)
	}

	if meta.ChapterCount > 0 {
		ci.Count = meta.ChapterCount
	}

	if len(meta.Genre) > 0 {
		ci.Tags = strings.Join(meta.Genre, ",")
	}

	if meta.URL != "" {
		ci.Web = meta.URL
	}

	if meta.UploadDate != nil {
		ci.Day = meta.UploadDate.Day()
		ci.Month = int(meta.UploadDate.Month())
		ci.Year = meta.UploadDate.Year()
	}

	t := strings.ToLower(strings.TrimSpace(meta.Type))
	if t == "manga" || containsIgnoreCase(meta.Genre, "manga") {
		ci.Manga = "YesAndRightToLeft"
	}

	return ci
}

// MarshalComicInfo serializes a ComicInfo to XML bytes.
func MarshalComicInfo(ci ComicInfo) ([]byte, error) {
	header := []byte(xml.Header)
	body, err := xml.MarshalIndent(ci, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(header, body...), nil
}

// FormatChapterNumber formats a chapter number for display.
func FormatChapterNumber(n float64) string {
	if n == float64(int(n)) {
		return fmt.Sprintf("%d", int(n))
	}
	return fmt.Sprintf("%g", n)
}

func containsIgnoreCase(slice []string, target string) bool {
	t := strings.ToLower(target)
	for _, s := range slice {
		if strings.ToLower(s) == t {
			return true
		}
	}
	return false
}
