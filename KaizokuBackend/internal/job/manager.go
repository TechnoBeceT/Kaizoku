package job

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
)

// Manager manages the River job queue client and custom download dispatcher.
type Manager struct {
	Client     *river.Client[pgx.Tx]
	Pool       *pgxpool.Pool
	Downloads  *DownloadDispatcher
	JobDeps    *Deps
}

// NewManager creates a River client (for non-download jobs) and a custom DownloadDispatcher.
func NewManager(ctx context.Context, cfg *config.Config, db *ent.Client, sw *suwayomi.Client, progress ProgressBroadcaster, settings SettingsReader) (*Manager, error) {
	dsn := cfg.Database.DSN()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	deps := &Deps{
		DB:       db,
		Suwayomi: sw,
		Progress: progress,
		Config:   cfg,
		Settings: settings,
	}

	// Register River workers (non-download jobs only)
	workers := river.NewWorkers()
	river.AddWorker(workers, &GetChaptersWorker{Deps: deps})
	river.AddWorker(workers, &GetLatestWorker{Deps: deps})
	river.AddWorker(workers, &UpdateExtensionsWorker{Deps: deps})
	river.AddWorker(workers, &UpdateAllSeriesWorker{Deps: deps})
	river.AddWorker(workers, &DailyUpdateWorker{Deps: deps})
	river.AddWorker(workers, &ScanLocalFilesWorker{Deps: deps})
	river.AddWorker(workers, &InstallExtensionsWorker{Deps: deps})
	river.AddWorker(workers, &SearchProvidersWorker{Deps: deps})
	river.AddWorker(workers, &ImportSeriesWorker{Deps: deps})
	river.AddWorker(workers, &RefreshAllChaptersWorker{Deps: deps})
	river.AddWorker(workers, &RefreshAllLatestWorker{Deps: deps})
	river.AddWorker(workers, &VerifyAllSeriesWorker{Deps: deps})

	// Parse schedule intervals from config
	extUpdateInterval, err := time.ParseDuration(cfg.Settings.ExtensionsUpdateSchedule)
	if err != nil {
		extUpdateInterval = 1 * time.Hour
	}
	chapterRefreshInterval, err := time.ParseDuration(cfg.Settings.PerTitleUpdateSchedule)
	if err != nil {
		chapterRefreshInterval = 2 * time.Hour
	}
	latestRefreshInterval, err := time.ParseDuration(cfg.Settings.PerSourceUpdateSchedule)
	if err != nil {
		latestRefreshInterval = 30 * time.Minute
	}

	periodicJobs := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return DailyUpdateArgs{}, nil
			},
			&river.PeriodicJobOpts{ID: "daily_update", RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(extUpdateInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return UpdateExtensionsArgs{}, nil
			},
			&river.PeriodicJobOpts{ID: "update_extensions", RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(chapterRefreshInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return RefreshAllChaptersArgs{}, nil
			},
			&river.PeriodicJobOpts{ID: "refresh_all_chapters"},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(latestRefreshInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return RefreshAllLatestArgs{}, nil
			},
			&river.PeriodicJobOpts{ID: "refresh_all_latest"},
		),
	}

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			QueueDefault: {
				MaxWorkers: 5,
			},
		},
		Workers:      workers,
		PeriodicJobs: periodicJobs,
		ErrorHandler: &logErrorHandler{},
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create river client: %w", err)
	}

	// Create custom download dispatcher (replaces River for download jobs)
	dlDispatcher := NewDownloadDispatcher(
		db, deps,
		cfg.Settings.SimultaneousDownloads,
		cfg.Settings.DownloadsPerProvider,
	)
	// Wire dispatcher and river client back into deps
	deps.DownloadQueue = dlDispatcher
	deps.RiverClient = riverClient

	return &Manager{
		Client:    riverClient,
		Pool:      pool,
		Downloads: dlDispatcher,
		JobDeps:   deps,
	}, nil
}

// Start begins processing River jobs and the download dispatcher.
func (m *Manager) Start(ctx context.Context) error {
	log.Info().Msg("starting River job queue")
	// Start download dispatcher in background
	go m.Downloads.Run(ctx)
	return m.Client.Start(ctx)
}

// Stop gracefully stops job processing.
func (m *Manager) Stop(ctx context.Context) error {
	log.Info().Msg("stopping River job queue and download dispatcher")
	m.Downloads.Stop()
	if err := m.Client.Stop(ctx); err != nil {
		return err
	}
	m.Pool.Close()
	return nil
}

// CreateSchema creates the River schema tables if they don't exist.
func (m *Manager) CreateSchema(ctx context.Context) error {
	migrator, err := rivermigrate.New[pgx.Tx](riverpgxv5.New(m.Pool), nil)
	if err != nil {
		return fmt.Errorf("get river migrator: %w", err)
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("run river migrations: %w", err)
	}
	return nil
}

// logErrorHandler logs River worker errors.
type logErrorHandler struct{}

func (h *logErrorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	log.Error().
		Err(err).
		Str("kind", job.Kind).
		Int64("id", job.ID).
		Int("attempt", job.Attempt).
		Msg("job failed")
	return nil
}

func (h *logErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	log.Error().
		Interface("panic", panicVal).
		Str("kind", job.Kind).
		Int64("id", job.ID).
		Str("trace", trace).
		Msg("job panicked")
	return nil
}
