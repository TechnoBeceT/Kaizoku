package types

// Settings is the full settings DTO returned by GET /api/settings.
type Settings struct {
	StorageFolder                            string   `json:"storageFolder"`
	PreferredLanguages                       []string `json:"preferredLanguages"`
	MihonRepositories                        []string `json:"mihonRepositories"`
	NumberOfSimultaneousDownloads            int      `json:"numberOfSimultaneousDownloads"`
	NumberOfSimultaneousSearches             int      `json:"numberOfSimultaneousSearches"`
	NumberOfSimultaneousDownloadsPerProvider int      `json:"numberOfSimultaneousDownloadsPerProvider"`
	ChapterDownloadFailRetryTime             string   `json:"chapterDownloadFailRetryTime"`
	ChapterDownloadFailRetries               int      `json:"chapterDownloadFailRetries"`
	PerTitleUpdateSchedule                   string   `json:"perTitleUpdateSchedule"`
	PerSourceUpdateSchedule                  string   `json:"perSourceUpdateSchedule"`
	ExtensionsCheckForUpdateSchedule         string   `json:"extensionsCheckForUpdateSchedule"`
	CategorizedFolders                       bool     `json:"categorizedFolders"`
	Categories                               []string `json:"categories"`
	FlareSolverrEnabled                      bool     `json:"flareSolverrEnabled"`
	FlareSolverrURL                          string   `json:"flareSolverrUrl"`
	FlareSolverrTimeout                      string   `json:"flareSolverrTimeout"`
	FlareSolverrSessionTTL                   string   `json:"flareSolverrSessionTtl"`
	FlareSolverrAsResponseFallback           bool     `json:"flareSolverrAsResponseFallback"`
	IsWizardSetupComplete                    bool     `json:"isWizardSetupComplete"`
	WizardSetupStepCompleted                 int      `json:"wizardSetupStepCompleted"`
}

// DefaultSettings returns the default settings matching .NET FirstTimeSettings.
func DefaultSettings() Settings {
	return Settings{
		PreferredLanguages:                       []string{"en"},
		MihonRepositories:                        []string{"https://raw.githubusercontent.com/keiyoushi/extensions/repo"},
		NumberOfSimultaneousDownloads:            10,
		NumberOfSimultaneousSearches:             10,
		NumberOfSimultaneousDownloadsPerProvider: 3,
		ChapterDownloadFailRetryTime:             "00:30:00",
		ChapterDownloadFailRetries:               144,
		PerTitleUpdateSchedule:                   "02:00:00",
		PerSourceUpdateSchedule:                  "00:30:00",
		ExtensionsCheckForUpdateSchedule:         "01:00:00",
		CategorizedFolders:                       true,
		Categories:                               []string{"Manga", "Manhwa", "Manhua", "Comic", "Other"},
		FlareSolverrEnabled:                      false,
		FlareSolverrURL:                          "http://localhost:8191",
		FlareSolverrTimeout:                      "00:00:30",
		FlareSolverrSessionTTL:                   "00:15:00",
		FlareSolverrAsResponseFallback:           false,
		IsWizardSetupComplete:                    false,
		WizardSetupStepCompleted:                 0,
	}
}

// DownloadInfo represents a download entry for the queue.
type DownloadInfo struct {
	ID               string      `json:"id"`
	Title            string      `json:"title"`
	Provider         string      `json:"provider"`
	Scanlator        *string     `json:"scanlator"`
	Language         string      `json:"language"`
	Chapter          *float64    `json:"chapter"`
	ChapterTitle     *string     `json:"chapterTitle"`
	DownloadDateUTC  *string     `json:"downloadDateUTC"`
	Status           QueueStatus `json:"status"`
	ScheduledDateUTC string      `json:"scheduledDateUTC"`
	Retries          int         `json:"retries"`
	ThumbnailURL     *string     `json:"thumbnailUrl"`
	URL              *string     `json:"url"`
}

// DownloadInfoList wraps downloads with total count.
type DownloadInfoList struct {
	TotalCount int            `json:"totalCount"`
	Downloads  []DownloadInfo `json:"downloads"`
}

// DownloadsMetrics contains download queue counts.
type DownloadsMetrics struct {
	Downloads int `json:"downloads"`
	Queued    int `json:"queued"`
	Failed    int `json:"failed"`
}

// SeriesInfo is the library list item with provider summaries.
type SeriesInfo struct {
	ID                 string              `json:"id"`
	Title              string              `json:"title"`
	ThumbnailURL       string              `json:"thumbnailUrl"`
	Artist             string              `json:"artist"`
	Author             string              `json:"author"`
	Description        string              `json:"description"`
	Genre              []string            `json:"genre"`
	Status             SeriesStatus        `json:"status"`
	StoragePath        string              `json:"storagePath"`
	Type               *string             `json:"type"`
	ChapterCount       int                 `json:"chapterCount"`
	LastChapter        *float64            `json:"lastChapter"`
	LastChangeUTC      *string             `json:"lastChangeUTC"`
	LastChangeProvider *SmallProviderInfo  `json:"lastChangeProvider"`
	IsActive           bool                `json:"isActive"`
	PausedDownloads    bool                `json:"pausedDownloads"`
	HasUnknown         bool                `json:"hasUnknown"`
	Providers          []SmallProviderInfo `json:"providers"`
}

// SmallProviderInfo is a minimal provider summary.
type SmallProviderInfo struct {
	Provider  string  `json:"provider"`
	Scanlator string  `json:"scanlator"`
	Language  string  `json:"language"`
	URL       *string `json:"url"`
	Importance int    `json:"importance"`
}

// SearchSource represents a searchable source.
type SearchSource struct {
	SourceID       string `json:"sourceId"`
	SourceName     string `json:"sourceName"`
	Language       string `json:"language"`
	SupportsLatest bool   `json:"supportsLatest"`
}

// LinkedSeries is a search result linking multiple sources.
type LinkedSeries struct {
	ID           string   `json:"id"`
	ProviderID   string   `json:"providerId"`
	Provider     string   `json:"provider"`
	Lang         string   `json:"lang"`
	ThumbnailURL *string  `json:"thumbnailUrl"`
	Title        string   `json:"title"`
	LinkedIDs    []string `json:"linkedIds"`
	UseCover     bool     `json:"useCover"`
}

// ImportInfo represents an import entry for the setup wizard.
type ImportInfo struct {
	Path                  string         `json:"path"`
	Title                 string         `json:"title"`
	Status                ImportStatus   `json:"status"`
	ContinueAfterChapter  *float64       `json:"continueAfterChapter"`
	Action                ImportAction   `json:"action"`
	Series                []SmallSeries  `json:"series"`
}

// SmallSeries is a minimal series for import display.
type SmallSeries struct {
	ID           string   `json:"id"`
	ProviderID   string   `json:"providerId"`
	Provider     string   `json:"provider"`
	Scanlator    string   `json:"scanlator"`
	Lang         string   `json:"lang"`
	ThumbnailURL *string  `json:"thumbnailUrl"`
	Title        string   `json:"title"`
	ChapterCount int64    `json:"chapterCount"`
	URL          *string  `json:"url"`
	LastChapter  *float64 `json:"lastChapter"`
	Preferred    bool     `json:"preferred"`
	ChapterList  string   `json:"chapterList"`
	UseCover     bool     `json:"useCover"`
	Importance   int      `json:"importance"`
	UseTitle     bool     `json:"useTitle"`
	IsSelected   bool     `json:"isSelected"`
}

// ImportTotals contains import summary counts.
type ImportTotals struct {
	TotalSeries    int `json:"totalSeries"`
	TotalProviders int `json:"totalProviders"`
	TotalDownloads int `json:"totalDownloads"`
}

// SeriesExtendedInfo is the detailed series view with full provider info.
type SeriesExtendedInfo struct {
	ID                 string                 `json:"id"`
	Title              string                 `json:"title"`
	ThumbnailURL       string                 `json:"thumbnailUrl"`
	Artist             string                 `json:"artist"`
	Author             string                 `json:"author"`
	Description        string                 `json:"description"`
	Genre              []string               `json:"genre"`
	Status             SeriesStatus           `json:"status"`
	StoragePath        string                 `json:"storagePath"`
	Type               *string                `json:"type"`
	ChapterCount       int                    `json:"chapterCount"`
	LastChapter        *float64               `json:"lastChapter"`
	LastChangeUTC      *string                `json:"lastChangeUTC"`
	LastChangeProvider *SmallProviderInfo     `json:"lastChangeProvider"`
	IsActive           bool                   `json:"isActive"`
	PausedDownloads    bool                   `json:"pausedDownloads"`
	HasUnknown         bool                   `json:"hasUnknown"`
	Providers          []ProviderExtendedInfo `json:"providers"`
	ChapterList        string                 `json:"chapterList"`
	Path               string                 `json:"path"`
}

// ProviderExtendedInfo is the detailed provider view within a series.
type ProviderExtendedInfo struct {
	ID                   string            `json:"id"`
	Provider             string            `json:"provider"`
	Scanlator            string            `json:"scanlator"`
	Lang                 string            `json:"lang"`
	ThumbnailURL         *string           `json:"thumbnailUrl"`
	Title                string            `json:"title"`
	Artist               string            `json:"artist"`
	Author               string            `json:"author"`
	Description          string            `json:"description"`
	Genre                []string          `json:"genre"`
	Type                 *string           `json:"type"`
	ChapterCount         int64             `json:"chapterCount"`
	ContinueAfterChapter *float64          `json:"fromChapter"`
	URL                  *string           `json:"url"`
	Meta                 map[string]string `json:"meta"`
	UseCover             bool              `json:"useCover"`
	Importance           int               `json:"importance"`
	IsUnknown            bool              `json:"isUnknown"`
	UseTitle             bool              `json:"useTitle"`
	IsDisabled           bool              `json:"isDisabled"`
	IsUninstalled        bool              `json:"isUninstalled"`
	IsDeleted            bool              `json:"isDeleted"`
	DeleteFiles          bool              `json:"deleteFiles"`
	LastUpdatedUTC       string            `json:"lastUpdatedUTC"`
	Status               SeriesStatus      `json:"status"`
	LastChapter          *float64          `json:"lastChapter"`
	LastChangeUTC        string            `json:"lastChangeUTC"`
	ChapterList          string            `json:"chapterList"`
	MatchID              string            `json:"matchId"`
}

// LatestSeriesInfo is the DTO for cloud latest series.
type LatestSeriesInfo struct {
	ID                 string          `json:"id"`
	SuwayomiSourceID   string          `json:"suwayomiSourceId"`
	Provider           string          `json:"provider"`
	Language           string          `json:"language"`
	URL                *string         `json:"url"`
	Title              string          `json:"title"`
	ThumbnailURL       *string         `json:"thumbnailUrl"`
	Artist             *string         `json:"artist"`
	Author             *string         `json:"author"`
	Description        *string         `json:"description"`
	Genre              []string        `json:"genre"`
	FetchDate          string          `json:"fetchDate"`
	ChapterCount       *int64          `json:"chapterCount"`
	LatestChapter      *float64        `json:"latestChapter"`
	LatestChapterTitle string          `json:"latestChapterTitle"`
	Status             SeriesStatus    `json:"status"`
	InLibrary          InLibraryStatus `json:"inLibrary"`
	SeriesID           *string         `json:"seriesId"`
}

// ProviderMatch represents a match between unknown and known providers.
type ProviderMatch struct {
	ID         string                `json:"id"`
	MatchInfos []MatchInfo          `json:"matchInfos"`
	Chapters   []ProviderMatchChapter `json:"chapters"`
}

// MatchInfo represents a known provider in a match.
type MatchInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Scanlator string `json:"scanlator"`
	Language string `json:"language"`
}

// ProviderMatchChapter represents a chapter in a provider match.
type ProviderMatchChapter struct {
	Filename      string   `json:"filename"`
	ChapterName   string   `json:"chapterName"`
	ChapterNumber *float64 `json:"chapterNumber"`
	MatchInfoID   *string  `json:"matchInfoId"`
	LocalPages    int      `json:"localPages,omitempty"`
	SourcePages   int      `json:"sourcePages,omitempty"`
	PageMismatch  bool     `json:"pageMismatch,omitempty"`
}

// MatchResult is the response from SetProviderMatch.
type MatchResult struct {
	Success       bool                    `json:"success"`
	Matched       int                     `json:"matched"`
	Redownloads   int                     `json:"redownloads"`
	MismatchFiles []ProviderMatchChapter  `json:"mismatchFiles,omitempty"`
}

// AugmentedResponse is the request body for AddSeries.
type AugmentedResponse struct {
	StorageFolderPath    string       `json:"storageFolderPath"`
	UseCategoriesForPath bool         `json:"useCategoriesForPath"`
	ExistingSeries       bool         `json:"existingSeries"`
	ExistingSeriesID     *string      `json:"existingSeriesId"`
	Categories           []string     `json:"categories"`
	Series               []FullSeries `json:"series"`
	PreferredLanguages   []string     `json:"preferredLanguages"`
	DisableJobs          bool         `json:"disableJobs"`
}

// FullSeries represents a complete series from search/augment with chapters.
type FullSeries struct {
	ID                   string            `json:"id"`
	ProviderID           string            `json:"providerId"`
	Provider             string            `json:"provider"`
	Scanlator            string            `json:"scanlator"`
	Lang                 string            `json:"lang"`
	ThumbnailURL         *string           `json:"thumbnailUrl"`
	Title                string            `json:"title"`
	Artist               string            `json:"artist"`
	Author               string            `json:"author"`
	Description          string            `json:"description"`
	Genre                []string          `json:"genre"`
	Type                 *string           `json:"type"`
	ChapterCount         int               `json:"chapterCount"`
	ContinueAfterChapter *float64          `json:"fromChapter"`
	URL                  *string           `json:"url"`
	Meta                 map[string]string `json:"meta"`
	UseCover             bool              `json:"useCover"`
	Importance           int               `json:"importance"`
	IsUnknown            bool              `json:"isUnknown"`
	UseTitle             bool              `json:"useTitle"`
	ExistingProvider     bool              `json:"existingProvider"`
	LastUpdatedUTC       string            `json:"lastUpdatedUTC"`
	SuggestedFilename    string            `json:"suggestedFilename"`
	Chapters             []Chapter         `json:"chapters"`
	Status               SeriesStatus      `json:"status"`
	ChapterList          string            `json:"chapterList"`
	IsSelected           bool              `json:"isSelected"`
}

// SeriesIntegrityResult is the result of a series integrity verification.
type SeriesIntegrityResult struct {
	Success          bool                     `json:"success"`
	BadFiles         []ArchiveIntegrityResult `json:"badFiles"`
	MissingFiles     int                      `json:"missingFiles"`     // DB said downloaded but file gone
	OrphanFiles      []string                 `json:"orphanFiles"`      // On disk but not in DB
	FixedCount       int                      `json:"fixedCount"`       // DB records corrected
	RedownloadQueued int                      `json:"redownloadQueued"` // Providers queued for re-download
}

// ArchiveIntegrityResult describes the integrity status of a single archive.
type ArchiveIntegrityResult struct {
	Filename string        `json:"filename"`
	Result   ArchiveResult `json:"result"`
}

// ArchiveResult enumerates archive integrity outcomes.
type ArchiveResult string

const (
	ArchiveResultFine         ArchiveResult = "Fine"
	ArchiveResultNotAnArchive ArchiveResult = "NotAnArchive"
	ArchiveResultNoImages     ArchiveResult = "NoImages"
	ArchiveResultNotFound     ArchiveResult = "NotFound"
	ArchiveResultTruncated    ArchiveResult = "Truncated"
)

// DeepVerifyResult is the result of a deep content verification.
type DeepVerifyResult struct {
	Success         bool             `json:"success"`
	SuspiciousFiles []SuspiciousFile `json:"suspiciousFiles"`
	SourceIssues    []SourceIssue    `json:"sourceIssues"`
}

// SuspiciousFile represents a CBZ file with mismatched content metadata.
type SuspiciousFile struct {
	Filename      string `json:"filename"`
	Provider      string `json:"provider"`
	ExpectedTitle string `json:"expectedTitle"`
	ActualTitle   string `json:"actualTitle"`
	ChapterNumber string `json:"chapterNumber"`
	Reason        string `json:"reason"` // "title_mismatch", "chapter_mismatch"
}

// SourceIssue represents a provider whose Suwayomi source no longer matches.
type SourceIssue struct {
	ProviderID    string `json:"providerId"`
	Provider      string `json:"provider"`
	ExpectedTitle string `json:"expectedTitle"`
	CurrentTitle  string `json:"currentTitle"`
	SuwayomiURL   string `json:"suwayomiUrl"`
	Reason        string `json:"reason"` // "source_changed"
}

// ProgressState is the WebSocket message for real-time job progress.
type ProgressState struct {
	ID             string         `json:"id"`
	JobType        JobType        `json:"jobType"`
	ProgressStatus ProgressStatus `json:"progressStatus"`
	Percentage     float64        `json:"percentage"`
	Message        string         `json:"message"`
	ErrorMessage   string         `json:"errorMessage"`
	Parameter      interface{}    `json:"parameter"`
}

// --- Reporting DTOs ---

// ReportingOverview is the dashboard summary.
type ReportingOverview struct {
	TotalEvents    int                  `json:"totalEvents"`
	SuccessRate    float64              `json:"successRate"`
	AvgDurationMs  float64             `json:"avgDurationMs"`
	ActiveSources  int                  `json:"activeSources"`
	SlowestSources []SourceStatsSummary `json:"slowestSources"`
	FailingSources []SourceStatsSummary `json:"failingSources"`
	RecentErrors   []SourceEventDTO     `json:"recentErrors"`
}

// SourceStatsSummary is a brief per-source stat for overview lists.
type SourceStatsSummary struct {
	SourceID      string  `json:"sourceId"`
	SourceName    string  `json:"sourceName"`
	Language      string  `json:"language"`
	AvgDurationMs float64 `json:"avgDurationMs"`
	EventCount    int     `json:"eventCount"`
	FailureCount  int     `json:"failureCount"`
	FailureRate   float64 `json:"failureRate"`
}

// SourceStats is the full per-source aggregation.
type SourceStats struct {
	SourceID         string                        `json:"sourceId"`
	SourceName       string                        `json:"sourceName"`
	Language         string                        `json:"language"`
	TotalEvents      int                           `json:"totalEvents"`
	SuccessCount     int                           `json:"successCount"`
	FailureCount     int                           `json:"failureCount"`
	PartialCount     int                           `json:"partialCount"`
	SuccessRate      float64                       `json:"successRate"`
	AvgDurationMs    float64                       `json:"avgDurationMs"`
	MaxDurationMs    int64                         `json:"maxDurationMs"`
	LastEventAt      *string                       `json:"lastEventAt"`
	LastErrorAt      *string                       `json:"lastErrorAt"`
	LastErrorMessage *string                       `json:"lastErrorMessage"`
	Breakdown        map[string]EventTypeBreakdown `json:"breakdown"`
}

// EventTypeBreakdown is per-event-type stats within a source.
type EventTypeBreakdown struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

// SourceEventDTO is a single event for the event log.
type SourceEventDTO struct {
	ID            string            `json:"id"`
	SourceID      string            `json:"sourceId"`
	SourceName    string            `json:"sourceName"`
	Language      string            `json:"language"`
	EventType     string            `json:"eventType"`
	Status        string            `json:"status"`
	DurationMs    int64             `json:"durationMs"`
	ErrorMessage  *string           `json:"errorMessage"`
	ErrorCategory *string           `json:"errorCategory"`
	ItemsCount    *int              `json:"itemsCount"`
	Metadata      map[string]string `json:"metadata"`
	CreatedAt     string            `json:"createdAt"`
}

// SourceEventList wraps events with total count for pagination.
type SourceEventList struct {
	Total  int              `json:"total"`
	Events []SourceEventDTO `json:"events"`
}

// TimelineBucket is a time-bucketed aggregation point.
type TimelineBucket struct {
	Timestamp     string  `json:"timestamp"`
	SuccessCount  int     `json:"successCount"`
	FailureCount  int     `json:"failureCount"`
	AvgDurationMs float64 `json:"avgDurationMs"`
	TotalEvents   int     `json:"totalEvents"`
}
