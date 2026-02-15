package types

import "github.com/google/uuid"

// FallbackSource represents a fallback provider for cascade downloads.
type FallbackSource struct {
	ProviderID uuid.UUID `json:"providerId"`
	SuwayomiID int       `json:"suwayomiId"`
	Importance int       `json:"importance"`
}

// DownloadChapterArgs represents the parameters for a chapter download job.
type DownloadChapterArgs struct {
	SeriesID      uuid.UUID `json:"seriesId"`
	ProviderID    uuid.UUID `json:"providerId"`
	SuwayomiID    int       `json:"suwayomiId"`
	ChapterIndex  int       `json:"chapterIndex"`
	ChapterNumber *float64  `json:"chapterNumber"`
	ChapterName   string    `json:"chapterName"`
	ProviderName  string    `json:"providerName"`
	Scanlator     string    `json:"scanlator"`
	Language      string    `json:"language"`
	Title         string    `json:"title"`
	StoragePath   string    `json:"storagePath"`
	ThumbnailURL  string    `json:"thumbnailUrl"`
	URL           string    `json:"url"`
	PageCount     int       `json:"pageCount"`
	UploadDate    int64     `json:"uploadDate"`

	// Cascade fields — providers to try on failure, sorted by importance.
	FallbackProviders []FallbackSource `json:"fallbackProviders,omitempty"`
	CascadeRetries    int              `json:"cascadeRetries"`
	OriginalItemID    uuid.UUID        `json:"originalItemId,omitempty"` // Queue item ID for cleanup on cascade

	// Replacement fields — set when this download replaces an inferior copy.
	IsReplacement       bool      `json:"isReplacement,omitempty"`
	ReplacingProviderID uuid.UUID `json:"replacingProviderId,omitempty"`
	ReplacingFilename   string    `json:"replacingFilename,omitempty"`
	ReplacementRetry    int       `json:"replacementRetry,omitempty"`
}

// Download queue status constants.
const (
	DLStatusWaiting   = 0
	DLStatusRunning   = 1
	DLStatusCompleted = 2
	DLStatusFailed    = 3
)
