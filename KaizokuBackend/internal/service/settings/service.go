package settings

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
	"github.com/technobecet/kaizoku-go/internal/types"
)

// Service manages application settings with DB persistence and Suwayomi sync.
type Service struct {
	db       *ent.Client
	cfg      *config.Config
	suwayomi *suwayomi.Client
	mu       sync.RWMutex
	cached   *types.Settings
}

// NewService creates a new settings service.
func NewService(db *ent.Client, cfg *config.Config, sw *suwayomi.Client) *Service {
	return &Service{
		db:       db,
		cfg:      cfg,
		suwayomi: sw,
	}
}

// Get loads settings from DB, falling back to defaults.
func (s *Service) Get(ctx context.Context) (*types.Settings, error) {
	s.mu.RLock()
	if s.cached != nil {
		c := *s.cached
		s.mu.RUnlock()
		return &c, nil
	}
	s.mu.RUnlock()

	return s.loadFromDB(ctx)
}

func (s *Service) loadFromDB(ctx context.Context) (*types.Settings, error) {
	records, err := s.db.Setting.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}

	settings := types.DefaultSettings()
	settings.StorageFolder = s.cfg.Storage.Folder

	if len(records) == 0 {
		// First run — save defaults to DB
		if err := s.saveToDB(ctx, &settings); err != nil {
			log.Warn().Err(err).Msg("failed to save default settings")
		}
	} else {
		kv := make(map[string]string, len(records))
		for _, r := range records {
			kv[r.ID] = r.Value
		}
		deserialize(kv, &settings)
		settings.StorageFolder = s.cfg.Storage.Folder
	}

	s.mu.Lock()
	s.cached = &settings
	s.mu.Unlock()

	return &settings, nil
}

// SyncOnStartup loads settings from DB and force-syncs them to Suwayomi.
// This matches .NET behavior where settings are synced on every startup
// to ensure Suwayomi has FlareSolverr, repos, and download limits configured.
func (s *Service) SyncOnStartup(ctx context.Context) {
	settings, err := s.Get(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to load settings for startup sync")
		return
	}
	if err := s.syncToSuwayomi(ctx, settings); err != nil {
		log.Warn().Err(err).Msg("failed to sync settings to Suwayomi on startup")
	} else {
		log.Info().Msg("settings synced to Suwayomi")
	}
}

// Save persists settings to DB and syncs relevant ones to Suwayomi.
func (s *Service) Save(ctx context.Context, settings *types.Settings) error {
	settings.StorageFolder = s.cfg.Storage.Folder

	if err := s.saveToDB(ctx, settings); err != nil {
		return err
	}

	// Sync to Suwayomi
	if err := s.syncToSuwayomi(ctx, settings); err != nil {
		log.Warn().Err(err).Msg("failed to sync settings to Suwayomi")
	}

	s.mu.Lock()
	s.cached = settings
	s.mu.Unlock()

	return nil
}

func (s *Service) saveToDB(ctx context.Context, settings *types.Settings) error {
	kv := serialize(settings)

	for name, value := range kv {
		err := s.db.Setting.Create().
			SetID(name).
			SetValue(value).
			OnConflictColumns("id").
			SetValue(value).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("upsert setting %s: %w", name, err)
		}
	}
	return nil
}

func (s *Service) syncToSuwayomi(ctx context.Context, settings *types.Settings) error {
	if s.suwayomi == nil {
		return nil
	}

	// Detect if extension repos changed so we can wait for Suwayomi to process them.
	s.mu.RLock()
	var oldRepos string
	if s.cached != nil {
		oldRepos = joinAndSort(s.cached.MihonRepositories)
	}
	s.mu.RUnlock()
	newRepos := joinAndSort(settings.MihonRepositories)
	reposChanged := oldRepos != newRepos && len(settings.MihonRepositories) > 0

	timeout := parseDuration(settings.FlareSolverrTimeout)
	sessionTTL := parseDuration(settings.FlareSolverrSessionTTL)

	payload := map[string]interface{}{
		"maxSourcesInParallel":           settings.NumberOfSimultaneousDownloads,
		"extensionRepos":                 settings.MihonRepositories,
		"flareSolverrEnabled":            settings.FlareSolverrEnabled,
		"flareSolverrUrl":                settings.FlareSolverrURL,
		"flareSolverrTimeout":            int(timeout.Seconds()),
		"flareSolverrSessionTtl":         int(sessionTTL.Minutes()),
		"flareSolverrAsResponseFallback": settings.FlareSolverrAsResponseFallback,
	}

	// Use GraphQL API — required for mihonRepositories/extensionRepos sync.
	if err := s.suwayomi.SetServerSettingsGraphQL(ctx, payload); err != nil {
		return err
	}

	// After setting new repos, Suwayomi needs time to fetch the extension list
	// from the remote repositories. Poll until extensions become available.
	if reposChanged {
		s.waitForExtensions(ctx)
	}

	return nil
}

// waitForExtensions polls Suwayomi's extension list until it returns results,
// giving Suwayomi time to fetch extensions from newly added repositories.
func (s *Service) waitForExtensions(ctx context.Context) {
	const maxWait = 30 * time.Second
	const pollInterval = time.Second

	log.Info().Msg("extension repos changed, waiting for Suwayomi to fetch extensions...")
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		extensions, err := s.suwayomi.GetExtensions(ctx)
		if err == nil && len(extensions) > 0 {
			log.Info().Int("count", len(extensions)).Msg("extensions available after repo sync")
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
	log.Warn().Msg("timed out waiting for extensions after repo sync")
}

// joinAndSort returns a deterministic string for comparing repo lists.
func joinAndSort(repos []string) string {
	sorted := make([]string, len(repos))
	copy(sorted, repos)
	sort.Strings(sorted)
	return strings.Join(sorted, "|")
}

// GetLanguages returns distinct languages from all available Suwayomi extensions.
// Falls back to provider storage mappings if Suwayomi is unreachable.
func (s *Service) GetLanguages(ctx context.Context) ([]string, error) {
	langSet := make(map[string]struct{})

	// Primary: fetch from Suwayomi's full extension list (includes all available, not just installed)
	if s.suwayomi != nil {
		extensions, err := s.suwayomi.GetExtensions(ctx)
		if err == nil {
			for _, ext := range extensions {
				lang := strings.ToLower(ext.Lang)
				if lang != "" && lang != "all" {
					langSet[lang] = struct{}{}
				}
			}
		}
	}

	// Fallback: if Suwayomi returned nothing, use provider storage
	if len(langSet) == 0 {
		providers, err := s.db.ProviderStorage.Query().All(ctx)
		if err != nil {
			return nil, fmt.Errorf("query providers: %w", err)
		}
		for _, p := range providers {
			for _, m := range p.Mappings {
				if m.Source != nil && m.Source.Lang != "" {
					lang := strings.ToLower(m.Source.Lang)
					if lang != "all" {
						langSet[lang] = struct{}{}
					}
				}
			}
		}
	}

	langs := make([]string, 0, len(langSet))
	for l := range langSet {
		langs = append(langs, l)
	}
	// Sort for deterministic output
	sortStrings(langs)
	return langs, nil
}

// --- Serialization helpers ---
// Settings are stored as PascalCase key-value pairs in the DB,
// matching the .NET convention.

func serialize(s *types.Settings) map[string]string {
	kv := map[string]string{
		"PreferredLanguages":                       joinPipe(s.PreferredLanguages),
		"MihonRepositories":                        joinPipe(s.MihonRepositories),
		"NumberOfSimultaneousDownloads":             strconv.Itoa(s.NumberOfSimultaneousDownloads),
		"NumberOfSimultaneousSearches":              strconv.Itoa(s.NumberOfSimultaneousSearches),
		"NumberOfSimultaneousDownloadsPerProvider":  strconv.Itoa(s.NumberOfSimultaneousDownloadsPerProvider),
		"ChapterDownloadFailRetryTime":              s.ChapterDownloadFailRetryTime,
		"ChapterDownloadFailRetries":                strconv.Itoa(s.ChapterDownloadFailRetries),
		"PerTitleUpdateSchedule":                    s.PerTitleUpdateSchedule,
		"PerSourceUpdateSchedule":                   s.PerSourceUpdateSchedule,
		"ExtensionsCheckForUpdateSchedule":          s.ExtensionsCheckForUpdateSchedule,
		"CategorizedFolders":                        strconv.FormatBool(s.CategorizedFolders),
		"Categories":                                joinPipe(s.Categories),
		"FlareSolverrEnabled":                       strconv.FormatBool(s.FlareSolverrEnabled),
		"FlareSolverrUrl":                           s.FlareSolverrURL,
		"FlareSolverrTimeout":                       s.FlareSolverrTimeout,
		"FlareSolverrSessionTtl":                    s.FlareSolverrSessionTTL,
		"FlareSolverrAsResponseFallback":            strconv.FormatBool(s.FlareSolverrAsResponseFallback),
		"IsWizardSetupComplete":                     strconv.FormatBool(s.IsWizardSetupComplete),
		"WizardSetupStepCompleted":                  strconv.Itoa(s.WizardSetupStepCompleted),
	}
	return kv
}

func deserialize(kv map[string]string, s *types.Settings) {
	if v, ok := kv["PreferredLanguages"]; ok {
		s.PreferredLanguages = splitPipe(v)
	}
	if v, ok := kv["MihonRepositories"]; ok {
		s.MihonRepositories = splitPipe(v)
	}
	if v, ok := kv["NumberOfSimultaneousDownloads"]; ok {
		s.NumberOfSimultaneousDownloads, _ = strconv.Atoi(v)
	}
	if v, ok := kv["NumberOfSimultaneousSearches"]; ok {
		s.NumberOfSimultaneousSearches, _ = strconv.Atoi(v)
	}
	if v, ok := kv["NumberOfSimultaneousDownloadsPerProvider"]; ok {
		s.NumberOfSimultaneousDownloadsPerProvider, _ = strconv.Atoi(v)
	}
	if v, ok := kv["ChapterDownloadFailRetryTime"]; ok {
		s.ChapterDownloadFailRetryTime = v
	}
	if v, ok := kv["ChapterDownloadFailRetries"]; ok {
		s.ChapterDownloadFailRetries, _ = strconv.Atoi(v)
	}
	if v, ok := kv["PerTitleUpdateSchedule"]; ok {
		s.PerTitleUpdateSchedule = v
	}
	if v, ok := kv["PerSourceUpdateSchedule"]; ok {
		s.PerSourceUpdateSchedule = v
	}
	if v, ok := kv["ExtensionsCheckForUpdateSchedule"]; ok {
		s.ExtensionsCheckForUpdateSchedule = v
	}
	if v, ok := kv["CategorizedFolders"]; ok {
		s.CategorizedFolders, _ = strconv.ParseBool(v)
	}
	if v, ok := kv["Categories"]; ok {
		s.Categories = splitPipe(v)
	}
	if v, ok := kv["FlareSolverrEnabled"]; ok {
		s.FlareSolverrEnabled, _ = strconv.ParseBool(v)
	}
	if v, ok := kv["FlareSolverrUrl"]; ok {
		s.FlareSolverrURL = v
	}
	if v, ok := kv["FlareSolverrTimeout"]; ok {
		s.FlareSolverrTimeout = v
	}
	if v, ok := kv["FlareSolverrSessionTtl"]; ok {
		s.FlareSolverrSessionTTL = v
	}
	if v, ok := kv["FlareSolverrAsResponseFallback"]; ok {
		s.FlareSolverrAsResponseFallback, _ = strconv.ParseBool(v)
	}
	if v, ok := kv["IsWizardSetupComplete"]; ok {
		s.IsWizardSetupComplete, _ = strconv.ParseBool(v)
	}
	if v, ok := kv["WizardSetupStepCompleted"]; ok {
		s.WizardSetupStepCompleted, _ = strconv.Atoi(v)
	}
}

func joinPipe(ss []string) string {
	return strings.Join(ss, "|")
}

func splitPipe(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "|")
}

func parseDuration(s string) time.Duration {
	// Parse "HH:MM:SS" format used by .NET TimeSpan
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])
	return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
}

func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
