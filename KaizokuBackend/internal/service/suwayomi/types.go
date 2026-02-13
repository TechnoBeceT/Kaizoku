package suwayomi

// SuwayomiSeries represents a manga from the Suwayomi API.
type SuwayomiSeries struct {
	ID                       int               `json:"id"`
	SourceID                 string            `json:"sourceId"`
	URL                      string            `json:"url"`
	Title                    string            `json:"title"`
	ThumbnailURL             *string           `json:"thumbnailUrl"`
	ThumbnailURLLastFetched  int64             `json:"thumbnailUrlLastFetched"`
	Initialized              bool              `json:"initialized"`
	Artist                   *string           `json:"artist"`
	Author                   *string           `json:"author"`
	Description              *string           `json:"description"`
	Genre                    []string          `json:"genre"`
	Status                   string            `json:"status"`
	InLibrary                bool              `json:"inLibrary"`
	InLibraryAt              int64             `json:"inLibraryAt"`
	Meta                     map[string]string `json:"meta"`
	RealURL                  *string           `json:"realUrl"`
	LastFetchedAt            *int64            `json:"lastFetchedAt"`
	ChaptersLastFetchedAt    *int64            `json:"chaptersLastFetchedAt"`
	FreshData                bool              `json:"freshData"`
	UnreadCount              *int64            `json:"unreadCount"`
	DownloadCount            *int64            `json:"downloadCount"`
	ChapterCount             *int64            `json:"chapterCount"`
	LastReadAt               *int64            `json:"lastReadAt"`
	Chapters                 []SuwayomiChapter `json:"chapters"`
}

// SuwayomiChapter represents a chapter from the Suwayomi API.
type SuwayomiChapter struct {
	ID            int               `json:"id"`
	URL           string            `json:"url"`
	Name          string            `json:"name"`
	UploadDate    int64             `json:"uploadDate"`
	ChapterNumber *float64          `json:"chapterNumber"`
	Scanlator     *string           `json:"scanlator"`
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

// SuwayomiExtension represents an extension from the Suwayomi API.
type SuwayomiExtension struct {
	Repo        string `json:"repo"`
	Name        string `json:"name"`
	PkgName     string `json:"pkgName"`
	VersionName string `json:"versionName"`
	VersionCode int64  `json:"versionCode"`
	Lang        string `json:"lang"`
	ApkName     string `json:"apkName"`
	IsNsfw      bool   `json:"isNsfw"`
	Installed   bool   `json:"installed"`
	HasUpdate   bool   `json:"hasUpdate"`
	IconURL     string `json:"iconUrl"`
	Obsolete    bool   `json:"obsolete"`
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

// SuwayomiPreference represents a preference entry for a source.
type SuwayomiPreference struct {
	Type   string       `json:"type"`
	Props  SuwayomiProp `json:"props"`
	Source string       `json:"source"`
}

// SuwayomiProp represents the properties of a preference.
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

// SuwayomiSettings represents the Suwayomi server settings.
type SuwayomiSettings struct {
	IP                     string `json:"ip"`
	Port                   int    `json:"port"`
	MaxSourcesInParallel   int    `json:"maxSourcesInParallel"`
	FlareSolverrEnabled    bool   `json:"flareSolverrEnabled"`
	FlareSolverrURL        string `json:"flareSolverrUrl"`
	FlareSolverrTimeout    int    `json:"flareSolverrTimeout"`
	FlareSolverrSessionTTL int    `json:"flareSolverrSessionTtl"`
}

// MangaSearchResult wraps a list of series from a search result.
type MangaSearchResult struct {
	MangaList []SuwayomiSeries `json:"mangaList"`
	HasNextPage bool           `json:"hasNextPage"`
}

// SetPreferenceRequest is the body for setting a source preference.
type SetPreferenceRequest struct {
	Position int         `json:"position"`
	Value    interface{} `json:"value"`
}

// MetadataUpdate is the body for updating manga metadata.
type MetadataUpdate struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ChapterUpdate is the body for updating a chapter.
type ChapterUpdate struct {
	Read       *bool  `json:"read,omitempty"`
	Bookmarked *bool  `json:"bookmarked,omitempty"`
	MarkPrevAsRead *bool `json:"markPrevAsRead,omitempty"`
	LastPageRead *int64 `json:"lastPageRead,omitempty"`
}
