package handler

import (
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/job"
	settingssvc "github.com/technobecet/kaizoku-go/internal/service/settings"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
)

// Handler aggregates all domain handlers.
type Handler struct {
	Series    *SeriesHandler
	Search    *SearchHandler
	Downloads *DownloadsHandler
	Provider  *ProviderHandler
	Settings  *SettingsHandler
	Setup     *SetupHandler
	Reporting *ReportingHandler
}

func New(cfg *config.Config, db *ent.Client, sw *suwayomi.Client, jobMgr *job.Manager) *Handler {
	ss := settingssvc.NewService(db, cfg, sw)
	rc := jobMgr.Client

	return &Handler{
		Series:    &SeriesHandler{config: cfg, db: db, suwayomi: sw, settings: ss, river: rc, downloads: jobMgr.Downloads, jobDeps: jobMgr.JobDeps},
		Search:    &SearchHandler{config: cfg, db: db, suwayomi: sw, settings: ss},
		Downloads: &DownloadsHandler{config: cfg, db: db, downloads: jobMgr.Downloads},
		Provider:  &ProviderHandler{config: cfg, db: db, suwayomi: sw},
		Settings:  NewSettingsHandler(cfg, db, sw),
		Setup:     &SetupHandler{config: cfg, db: db, suwayomi: sw, river: rc},
		Reporting: &ReportingHandler{db: db},
	}
}

// riverClient is a type alias for the River client used across handlers.
type riverClient = *river.Client[pgx.Tx]
