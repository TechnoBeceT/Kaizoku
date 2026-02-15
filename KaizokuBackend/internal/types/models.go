package types

import "time"

// Chapter represents a chapter within a series provider (stored as JSON).
type Chapter struct {
	Name               string   `json:"name"`
	Number             *float64 `json:"number"`
	ProviderUploadDate *time.Time `json:"providerUploadDate"`
	URL                string   `json:"url"`
	ProviderIndex      int      `json:"providerIndex"`
	DownloadDate       *time.Time `json:"downloadDate"`
	ShouldDownload      bool     `json:"shouldDownload"`
	IsDeleted           bool     `json:"isDeleted"`
	IsPermanentlyFailed bool     `json:"isPermanentlyFailed"`
	PageCount           *int     `json:"pageCount"`
	Filename            string   `json:"filename"`
}

// SuwayomiChapter represents a chapter from the Suwayomi API.
type SuwayomiChapter struct {
	ID            int               `json:"id"`
	URL           string            `json:"url"`
	Name          string            `json:"name"`
	UploadDate    int64             `json:"uploadDate"`
	ChapterNumber *float64          `json:"chapterNumber"`
	Scanlator     string            `json:"scanlator"`
	MangaID       int               `json:"mangaId"`
	Read          bool              `json:"read"`
	Bookmarked    bool              `json:"bookmarked"`
	LastPageRead  int64             `json:"lastPageRead"`
	LastReadAt    int64             `json:"lastReadAt"`
	Index         int               `json:"index"`
	FetchedAt     int64             `json:"fetchedAt"`
	RealURL       string            `json:"realUrl"`
	Downloaded    bool              `json:"downloaded"`
	PageCount     int               `json:"pageCount"`
	ChapterCount  int               `json:"chapterCount"`
	Meta          map[string]string `json:"meta"`
}

// SuwayomiSource represents a source from the Suwayomi API.
type SuwayomiSource struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Lang           string `json:"lang"`
	IconURL        string `json:"iconUrl"`
	SupportsLatest bool   `json:"supportsLatest"`
	IsConfigurable bool   `json:"isConfigurable"`
	IsNsfw         bool   `json:"isNsfw"`
	DisplayName    string `json:"displayName"`
}

// SuwayomiPreference represents a preference for a Suwayomi source.
type SuwayomiPreference struct {
	Type   string       `json:"type"`
	Props  SuwayomiProp `json:"props"`
	Source string       `json:"source"`
}

// SuwayomiProp represents the properties of a Suwayomi preference.
type SuwayomiProp struct {
	Key              string      `json:"key"`
	Title            string      `json:"title"`
	Summary          string      `json:"summary"`
	DefaultValue     interface{} `json:"defaultValue"`
	Entries          []string    `json:"entries"`
	EntryValues      []string    `json:"entryValues"`
	DefaultValueType string      `json:"defaultValueType"`
	CurrentValue     interface{} `json:"currentValue"`
	Visible          bool        `json:"visible"`
	DialogTitle      string      `json:"dialogTitle"`
	DialogMessage    string      `json:"dialogMessage"`
	Text             string      `json:"text"`
}

// ProviderMapping stores source and preference info for a provider (stored as JSON).
type ProviderMapping struct {
	Source      *SuwayomiSource      `json:"source"`
	Preferences []SuwayomiPreference `json:"preferences"`
}

// KaizokuInfo represents the metadata stored in kaizoku.json files.
type KaizokuInfo struct {
	Title          string         `json:"title"`
	Status         SeriesStatus   `json:"status"`
	Artist         string         `json:"artist"`
	Author         string         `json:"author"`
	Description    string         `json:"description"`
	Genre          []string       `json:"genre"`
	Type           string         `json:"type"`
	ChapterCount   int            `json:"chapterCount"`
	LastUpdatedUTC *time.Time     `json:"lastUpdatedUTC"`
	Providers      []ProviderInfo `json:"providers"`
	IsDisabled     bool           `json:"isDisabled"`
	KaizokuVersion int            `json:"kaizokuVersion"`
	Path           string         `json:"path"`
}

// ProviderInfo represents provider metadata in kaizoku.json.
type ProviderInfo struct {
	Provider     string        `json:"provider"`
	Language     string        `json:"language"`
	Scanlator    string        `json:"scanlator"`
	Title        string        `json:"title"`
	ThumbnailURL string        `json:"thumbnailUrl"`
	Status       SeriesStatus  `json:"status"`
	Importance   int           `json:"importance"`
	ChapterCount int           `json:"chapterCount"`
	ChapterList  []StartStop   `json:"chapterList"`
	IsDisabled   bool          `json:"isDisabled"`
	Archives     []ArchiveInfo `json:"archives"`
}

// StartStop represents a chapter range.
type StartStop struct {
	Start float64 `json:"start"`
	Stop  float64 `json:"stop"`
}

// ArchiveInfo represents info about a downloaded archive.
type ArchiveInfo struct {
	Filename      string   `json:"filename"`
	ChapterName   string   `json:"chapterName"`
	ChapterNumber *float64 `json:"chapterNumber"`
}
