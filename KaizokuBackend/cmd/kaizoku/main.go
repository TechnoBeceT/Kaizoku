package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/database"
	"github.com/technobecet/kaizoku-go/internal/job"
	"github.com/technobecet/kaizoku-go/internal/server"
	settingssvc "github.com/technobecet/kaizoku-go/internal/service/settings"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
	"github.com/technobecet/kaizoku-go/internal/ws"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	log.Info().
		Int("port", cfg.Server.Port).
		Str("storage", cfg.Storage.Folder).
		Msg("starting Kaizoku.GO")

	// Setup Suwayomi: check Java, download JAR if needed, write initial config.
	// Skipped entirely when UseCustomAPI is true.
	runtimeDir := config.ConfigDir()
	if err := suwayomi.Setup(context.Background(), cfg.Suwayomi, runtimeDir); err != nil {
		log.Warn().Err(err).Msg("Suwayomi setup failed (continuing without embedded Suwayomi)")
	}

	// Start embedded Suwayomi process (unless using custom API)
	var swProcess *suwayomi.ProcessManager
	if !cfg.Suwayomi.UseCustomAPI {
		swProcess = suwayomi.NewProcessManager(runtimeDir, cfg.Suwayomi.Port)

		startCtx, startCancel := context.WithCancel(context.Background())
		defer startCancel()

		go func() {
			if err := swProcess.Start(startCtx); err != nil {
				log.Warn().Err(err).Msg("Suwayomi process failed to start (continuing without embedded Suwayomi)")
			}
		}()
	} else {
		log.Info().Str("endpoint", cfg.Suwayomi.CustomEndpoint).Msg("using custom Suwayomi API endpoint")
	}

	// Open database
	db, err := database.Open(cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	// Run Ent migrations
	if err := database.Migrate(context.Background(), db); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}

	// Ensure storage folder exists
	if err := os.MkdirAll(cfg.Storage.Folder, 0o755); err != nil {
		log.Fatal().Err(err).Str("path", cfg.Storage.Folder).Msg("failed to create storage directory")
	}

	// Create shared Suwayomi client
	sw := suwayomi.NewClient(cfg.Suwayomi.BaseURL())

	// Create progress hub (shared between job manager and server)
	hub := ws.NewHub()

	// Create River job manager
	ctx := context.Background()
	jobMgr, err := job.NewManager(ctx, cfg, db, sw, hub)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create job manager")
	}

	// Run River schema migrations
	if err := jobMgr.CreateSchema(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to create River schema")
	}

	srv := server.New(cfg, db, sw, jobMgr, hub)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Sync settings to Suwayomi on startup (matches .NET behavior).
	// Runs in background because Suwayomi may still be starting up.
	go func() {
		ss := settingssvc.NewService(db, cfg, sw)
		// Wait for Suwayomi to become responsive before syncing
		for i := 0; i < 30; i++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			if _, err := sw.GetServerSettings(ctx); err == nil {
				ss.SyncOnStartup(ctx)
				return
			}
		}
		log.Warn().Msg("Suwayomi not responsive after 60s, skipping startup settings sync")
	}()

	// Start River job queue
	go func() {
		if err := jobMgr.Start(ctx); err != nil {
			log.Error().Err(err).Msg("job queue error")
			cancel()
		}
	}()

	// Start HTTP server
	go func() {
		if err := srv.Start(); err != nil {
			log.Error().Err(err).Msg("server error")
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutting down...")

	shutdownCtx := context.Background()

	if err := jobMgr.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("job queue shutdown error")
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
	}

	// Stop Suwayomi process
	if swProcess != nil {
		swProcess.Stop()
	}
}
