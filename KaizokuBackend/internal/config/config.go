package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/rs/zerolog/log"
)

type Config struct {
	Server   ServerConfig   `koanf:"server"`
	Database DatabaseConfig `koanf:"database"`
	Storage  StorageConfig  `koanf:"storage"`
	Suwayomi SuwayomiConfig `koanf:"suwayomi"`
	Settings SettingsConfig `koanf:"settings"`
}

type ServerConfig struct {
	Port int `koanf:"port"`
}

type DatabaseConfig struct {
	Host     string `koanf:"host"`
	Port     int    `koanf:"port"`
	User     string `koanf:"user"`
	Password string `koanf:"password"`
	DBName   string `koanf:"dbname"`
	SSLMode  string `koanf:"sslmode"`
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

type StorageConfig struct {
	Folder string `koanf:"folder"`
}

type SuwayomiConfig struct {
	UsePreview     bool   `koanf:"use_preview"`
	Version        string `koanf:"version"`
	UseCustomAPI   bool   `koanf:"use_custom_api"`
	CustomEndpoint string `koanf:"custom_endpoint"`
	Port           int    `koanf:"port"`
}

func (s SuwayomiConfig) BaseURL() string {
	if s.UseCustomAPI && s.CustomEndpoint != "" {
		return s.CustomEndpoint
	}
	return fmt.Sprintf("http://127.0.0.1:%d/api/v1", s.Port)
}

type SettingsConfig struct {
	PreferredLanguages      []string `koanf:"preferred_languages"`
	MihonRepositories       []string `koanf:"mihon_repositories"`
	SimultaneousDownloads   int      `koanf:"simultaneous_downloads"`
	SimultaneousSearches    int      `koanf:"simultaneous_searches"`
	DownloadsPerProvider    int      `koanf:"downloads_per_provider"`
	PerTitleUpdateSchedule  string   `koanf:"per_title_update_schedule"`
	PerSourceUpdateSchedule string   `koanf:"per_source_update_schedule"`
	ExtensionsUpdateSchedule string  `koanf:"extensions_update_schedule"`
	ChapterFailRetries      int      `koanf:"chapter_fail_retries"`
	ChapterFailRetryTime    string   `koanf:"chapter_fail_retry_time"`
	CategorizedFolders      bool     `koanf:"categorized_folders"`
	Categories              []string `koanf:"categories"`
}

func Load() (*Config, error) {
	k := koanf.New(".")

	// Defaults
	defaults := map[string]interface{}{
		"server.port":                       9833,
		"database.host":                     "localhost",
		"database.port":                     5432,
		"database.user":                     "kaizoku",
		"database.password":                 "kaizoku",
		"database.dbname":                   "kaizoku",
		"database.sslmode":                  "disable",
		"storage.folder":                    "",
		"suwayomi.use_preview":              true,
		"suwayomi.version":                  "v2.0.1833",
		"suwayomi.use_custom_api":           false,
		"suwayomi.custom_endpoint":          "http://127.0.0.1:4567/api/v1",
		"suwayomi.port":                     4567,
		"settings.preferred_languages":      []string{"en"},
		"settings.mihon_repositories":       []string{},
		"settings.simultaneous_downloads":   10,
		"settings.simultaneous_searches":    10,
		"settings.downloads_per_provider":   3,
		"settings.per_title_update_schedule":  "2h",
		"settings.per_source_update_schedule": "30m",
		"settings.extensions_update_schedule": "1h",
		"settings.chapter_fail_retries":     144,
		"settings.chapter_fail_retry_time":  "30m",
		"settings.categorized_folders":      true,
		"settings.categories":               []string{"Manga", "Manhwa", "Manhua", "Comic", "Other"},
	}
	for key, val := range defaults {
		_ = k.Set(key, val)
	}

	// Load YAML config from runtime directory
	configDir := resolveConfigDir()
	configPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
			log.Warn().Err(err).Str("path", configPath).Msg("failed to load config file, using defaults")
		} else {
			log.Info().Str("path", configPath).Msg("loaded config file")
		}
	} else {
		log.Info().Str("path", configPath).Msg("no config file found, using defaults")
	}

	// Environment variable overrides: KAIZOKU_SERVER_PORT -> server.port
	if err := k.Load(env.Provider("KAIZOKU_", ".", func(s string) string {
		return strings.Replace(
			strings.ToLower(strings.TrimPrefix(s, "KAIZOKU_")),
			"_", ".", -1,
		)
	}), nil); err != nil {
		log.Warn().Err(err).Msg("failed to load env overrides")
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Resolve storage folder
	if cfg.Storage.Folder == "" {
		cfg.Storage.Folder = resolveStorageDir()
	}

	return &cfg, nil
}

func IsDocker() bool {
	return os.Getenv("KAIZOKU_DOCKER") == "true"
}

// ConfigDir returns the resolved configuration directory.
func ConfigDir() string {
	return resolveConfigDir()
}

func resolveConfigDir() string {
	if IsDocker() {
		return "/config"
	}

	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			return filepath.Join(localAppData, "KaizokuNET")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "AppData", "Local", "KaizokuNET")
	default: // linux, darwin
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "KaizokuNET")
	}
}

func resolveStorageDir() string {
	if envDir := os.Getenv("KAIZOKU_STORAGE_DIR"); envDir != "" {
		return envDir
	}
	if IsDocker() {
		return "/series"
	}
	return filepath.Join(resolveConfigDir(), "series")
}
