package job

import (
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

// Queue names used by River (downloads are handled by custom DownloadDispatcher, not River).
const (
	QueueDefault = "default"
)

// --- River job argument types (non-download jobs only) ---

// GetChaptersArgs represents a job to fetch chapters for a provider.
type GetChaptersArgs struct {
	ProviderID uuid.UUID `json:"providerId"`
}

func (GetChaptersArgs) Kind() string { return "get_chapters" }

func (GetChaptersArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}

// GetLatestArgs represents a job to fetch latest series from a source.
type GetLatestArgs struct {
	SourceID string `json:"sourceId"`
}

func (GetLatestArgs) Kind() string { return "get_latest" }

func (GetLatestArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}

// UpdateExtensionsArgs represents a job to check for extension updates.
type UpdateExtensionsArgs struct{}

func (UpdateExtensionsArgs) Kind() string { return "update_extensions" }

func (UpdateExtensionsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: 1 * time.Hour,
		},
	}
}

// UpdateAllSeriesArgs represents a job to update all series.
type UpdateAllSeriesArgs struct{}

func (UpdateAllSeriesArgs) Kind() string { return "update_all_series" }

func (UpdateAllSeriesArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: 1 * time.Hour,
		},
	}
}

// DailyUpdateArgs represents the daily maintenance job.
type DailyUpdateArgs struct{}

func (DailyUpdateArgs) Kind() string { return "daily_update" }

func (DailyUpdateArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
	}
}

// ScanLocalFilesArgs represents a job to scan local directories.
type ScanLocalFilesArgs struct {
	Path string `json:"path"`
}

func (ScanLocalFilesArgs) Kind() string { return "scan_local_files" }

func (ScanLocalFilesArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
	}
}

// InstallExtensionsArgs represents a job to install required extensions.
type InstallExtensionsArgs struct {
	Packages []string `json:"packages"`
}

func (InstallExtensionsArgs) Kind() string { return "install_extensions" }

func (InstallExtensionsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
	}
}

// SearchProvidersArgs represents a job to search for imported series.
type SearchProvidersArgs struct{}

func (SearchProvidersArgs) Kind() string { return "search_providers" }

func (SearchProvidersArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
	}
}

// RefreshAllChaptersArgs dispatches GetChapters for every active provider.
type RefreshAllChaptersArgs struct{}

func (RefreshAllChaptersArgs) Kind() string { return "refresh_all_chapters" }

func (RefreshAllChaptersArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: 10 * time.Minute,
		},
	}
}

// RefreshAllLatestArgs dispatches GetLatest for every active source.
type RefreshAllLatestArgs struct{}

func (RefreshAllLatestArgs) Kind() string { return "refresh_all_latest" }

func (RefreshAllLatestArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: 5 * time.Minute,
		},
	}
}

// ImportSeriesArgs represents a job to import scanned series.
type ImportSeriesArgs struct {
	DisableDownloads bool `json:"disableDownloads"`
}

func (ImportSeriesArgs) Kind() string { return "import_series" }

func (ImportSeriesArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
	}
}

// VerifyAllSeriesArgs represents a job to verify all series in the library.
type VerifyAllSeriesArgs struct{}

func (VerifyAllSeriesArgs) Kind() string { return "verify_all_series" }

func (VerifyAllSeriesArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueDefault,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: 5 * time.Minute,
		},
	}
}
