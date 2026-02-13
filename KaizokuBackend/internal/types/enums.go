package types

import (
	"encoding/json"
	"strconv"
)

// SeriesStatus represents the publication status of a series.
// Stored as string internally (DB), serialized as integer in JSON (frontend expects int).
type SeriesStatus string

const (
	SeriesStatusUnknown            SeriesStatus = "UNKNOWN"
	SeriesStatusOngoing            SeriesStatus = "ONGOING"
	SeriesStatusCompleted          SeriesStatus = "COMPLETED"
	SeriesStatusLicensed           SeriesStatus = "LICENSED"
	SeriesStatusPublishingFinished SeriesStatus = "PUBLISHING_FINISHED"
	SeriesStatusCancelled          SeriesStatus = "CANCELLED"
	SeriesStatusOnHiatus           SeriesStatus = "ON_HIATUS"
)

func (SeriesStatus) Values() []string {
	return []string{
		string(SeriesStatusUnknown),
		string(SeriesStatusOngoing),
		string(SeriesStatusCompleted),
		string(SeriesStatusLicensed),
		string(SeriesStatusPublishingFinished),
		string(SeriesStatusCancelled),
		string(SeriesStatusOnHiatus),
	}
}

// seriesStatusToInt maps SeriesStatus string to its integer value.
func seriesStatusToInt(s SeriesStatus) int {
	switch s {
	case SeriesStatusUnknown:
		return 0
	case SeriesStatusOngoing:
		return 1
	case SeriesStatusCompleted:
		return 2
	case SeriesStatusLicensed:
		return 3
	case SeriesStatusPublishingFinished:
		return 4
	case SeriesStatusCancelled:
		return 5
	case SeriesStatusOnHiatus:
		return 6
	default:
		return 0
	}
}

// seriesStatusFromInt maps an integer to its SeriesStatus string.
func seriesStatusFromInt(v int) SeriesStatus {
	switch v {
	case 0:
		return SeriesStatusUnknown
	case 1:
		return SeriesStatusOngoing
	case 2:
		return SeriesStatusCompleted
	case 3:
		return SeriesStatusLicensed
	case 4:
		return SeriesStatusPublishingFinished
	case 5:
		return SeriesStatusCancelled
	case 6:
		return SeriesStatusOnHiatus
	default:
		return SeriesStatusUnknown
	}
}

// MarshalJSON serializes SeriesStatus as an integer for frontend compatibility.
func (s SeriesStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(seriesStatusToInt(s))
}

// UnmarshalJSON deserializes SeriesStatus from either integer or string.
func (s *SeriesStatus) UnmarshalJSON(data []byte) error {
	// Try integer first (from frontend)
	var v int
	if err := json.Unmarshal(data, &v); err == nil {
		*s = seriesStatusFromInt(v)
		return nil
	}
	// Try string (from Suwayomi API or DB)
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	// Could be a numeric string from query params
	if iv, err := strconv.Atoi(str); err == nil {
		*s = seriesStatusFromInt(iv)
	} else {
		*s = SeriesStatus(str)
	}
	return nil
}

// InLibraryStatus represents whether a latest series is in the user's library.
type InLibraryStatus int

const (
	InLibraryNotInLibrary       InLibraryStatus = 0
	InLibraryInLibrary          InLibraryStatus = 1
	InLibraryInLibraryDisabled  InLibraryStatus = 2
)

// ImportStatus represents the status of an import entry.
type ImportStatus int

const (
	ImportStatusImport      ImportStatus = 0
	ImportStatusSkip        ImportStatus = 1
	ImportStatusDoNotChange ImportStatus = 2
	ImportStatusCompleted   ImportStatus = 3
)

// ImportAction represents the action to take on an import entry.
type ImportAction int

const (
	ImportActionAdd  ImportAction = 0
	ImportActionSkip ImportAction = 1
)

// JobType represents the type of background job.
type JobType int

const (
	JobTypeScanLocalFiles             JobType = 0
	JobTypeInstallAdditionalExtensions JobType = 1
	JobTypeSearchProviders            JobType = 2
	JobTypeImportSeries               JobType = 3
	JobTypeGetChapters                JobType = 4
	JobTypeGetLatest                  JobType = 5
	JobTypeDownload                   JobType = 6
	JobTypeUpdateExtensions           JobType = 7
	JobTypeUpdateAllSeries            JobType = 8
	JobTypeDailyUpdate                JobType = 9
	JobTypeVerifyAll                  JobType = 10
)

// QueueStatus represents the status of a queued job.
type QueueStatus int

const (
	QueueStatusWaiting   QueueStatus = 0
	QueueStatusRunning   QueueStatus = 1
	QueueStatusCompleted QueueStatus = 2
	QueueStatusFailed    QueueStatus = 3
)

// Priority represents the priority of a job.
type Priority int

const (
	PriorityLow    Priority = 0
	PriorityNormal Priority = 1
	PriorityHigh   Priority = 2
)

// ErrorDownloadAction represents actions for managing errored downloads.
// Frontend sends as integer (0=Retry, 1=Delete) or string ("Retry", "Delete").
type ErrorDownloadAction string

const (
	ErrorDownloadActionRetry  ErrorDownloadAction = "Retry"
	ErrorDownloadActionDelete ErrorDownloadAction = "Delete"
)

// ParseErrorDownloadAction parses an action from query param (int or string).
func ParseErrorDownloadAction(s string) ErrorDownloadAction {
	switch s {
	case "Retry", "0":
		return ErrorDownloadActionRetry
	case "Delete", "1":
		return ErrorDownloadActionDelete
	default:
		return ErrorDownloadAction(s)
	}
}

// ProgressStatus represents the status of a progress update.
type ProgressStatus int

const (
	ProgressStatusQueued    ProgressStatus = 0
	ProgressStatusRunning   ProgressStatus = 1
	ProgressStatusCompleted ProgressStatus = 2
	ProgressStatusFailed    ProgressStatus = 3
)
