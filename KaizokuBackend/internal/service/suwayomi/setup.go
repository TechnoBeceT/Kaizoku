package suwayomi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
)

const (
	suwayomiJarPattern    = "Suwayomi-Server-%s.jar"
	suwayomiStableURL     = "https://github.com/Suwayomi/Suwayomi-Server/releases/download/%s/%s"
	suwayomiPreviewURL    = "https://github.com/Suwayomi/Suwayomi-Server-preview/releases/download/%s/%s"
	minJavaVersion        = 21
)

// defaultServerConf is the initial Suwayomi server.conf that matches the .NET version.
const defaultServerConf = `# Server ip and port bindings
server.ip = "0.0.0.0"
server.port = 4567

# Socks5 proxy
server.socksProxyEnabled = false
server.socksProxyVersion = 5
server.socksProxyHost = ""
server.socksProxyPort = ""
server.socksProxyUsername = ""
server.socksProxyPassword = ""

# webUI
server.webUIEnabled = true
server.webUIFlavor = "WebUI"
server.initialOpenInBrowserEnabled = false
server.webUIInterface = "browser"
server.electronPath = ""
server.webUIChannel = "bundled"
server.webUIUpdateCheckInterval = 0

# downloader
server.downloadAsCbz = true
server.downloadsPath = ""
server.autoDownloadNewChapters = false
server.excludeEntryWithUnreadChapters = true
server.autoDownloadNewChaptersLimit = 0
server.autoDownloadIgnoreReUploads = false

# extension repos
server.extensionRepos = [
    "https://raw.githubusercontent.com/keiyoushi/extensions/repo"
]

# requests
server.maxSourcesInParallel = 6

# updater
server.excludeUnreadChapters = true
server.excludeNotStarted = true
server.excludeCompleted = true
server.globalUpdateInterval = 0
server.updateMangas = false

# Authentication
server.authMode = "none"
server.authUsername = ""
server.authPassword = ""

# misc
server.debugLogsEnabled = false
server.systemTrayEnabled = false
server.maxLogFiles = 31
server.maxLogFileSize = "10mb"
server.maxLogFolderSize = "100mb"

# backup
server.backupPath = ""
server.backupTime = "00:00"
server.backupInterval = 1
server.backupTTL = 14

# local source
server.localSourcePath = ""

# Cloudflare bypass
server.flareSolverrEnabled = false
server.flareSolverrUrl = "http://localhost:8191"
server.flareSolverrTimeout = 60
server.flareSolverrSessionName = "suwayomi"
server.flareSolverrSessionTtl = 15
server.flareSolverrAsResponseFallback = false
`

// Setup prepares the Suwayomi environment: checks Java, downloads JAR if needed,
// writes initial server.conf. Returns nil if UseCustomAPI is true (skip everything).
func Setup(ctx context.Context, cfg config.SuwayomiConfig, runtimeDir string) error {
	if cfg.UseCustomAPI {
		log.Info().Str("endpoint", cfg.CustomEndpoint).Msg("using custom Suwayomi API, skipping setup")
		return nil
	}

	// Check Java version
	if err := checkJavaVersion(); err != nil {
		return fmt.Errorf("java check: %w", err)
	}

	suwayomiDir := filepath.Join(runtimeDir, "Suwayomi")
	if err := os.MkdirAll(suwayomiDir, 0o755); err != nil {
		return fmt.Errorf("create Suwayomi directory: %w", err)
	}

	// Write initial server.conf if it doesn't exist
	if err := writeInitialConfig(suwayomiDir); err != nil {
		log.Warn().Err(err).Msg("failed to write initial Suwayomi config")
	}

	// Download JAR if needed
	if err := downloadJarIfNeeded(ctx, cfg, suwayomiDir); err != nil {
		return fmt.Errorf("download Suwayomi: %w", err)
	}

	return nil
}

// checkJavaVersion runs `java -version` and verifies the version is >= 21.
func checkJavaVersion() error {
	log.Info().Msg("checking Java version")

	cmd := exec.Command("java", "-version")
	// Java outputs version to stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("java not found or failed to run: %w (install JRE %d+)", err, minJavaVersion)
	}

	version := parseJavaVersion(string(output))
	if version == 0 {
		return fmt.Errorf("unable to parse Java version from: %s", string(output))
	}

	log.Info().Int("version", version).Msg("found Java")

	if version < minJavaVersion {
		return fmt.Errorf("Java %d required, found %d", minJavaVersion, version)
	}

	return nil
}

// parseJavaVersion extracts the major version number from `java -version` output.
// Handles formats like: "21.0.1", "openjdk version \"21.0.1\"", "1.8.0_292"
var javaVersionRegex = regexp.MustCompile(`(?:version\s+")?([\d]+)(?:\.([\d]+))?`)

func parseJavaVersion(output string) int {
	matches := javaVersionRegex.FindStringSubmatch(output)
	if len(matches) < 2 {
		return 0
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	// Old-style versioning: "1.8.0" means Java 8
	if major == 1 && len(matches) >= 3 {
		minor, _ := strconv.Atoi(matches[2])
		return minor
	}
	return major
}

// writeInitialConfig writes the default server.conf if it doesn't already exist.
func writeInitialConfig(suwayomiDir string) error {
	confPath := filepath.Join(suwayomiDir, "server.conf")
	if _, err := os.Stat(confPath); err == nil {
		return nil // already exists
	}

	log.Info().Str("path", confPath).Msg("writing initial Suwayomi server.conf")
	return os.WriteFile(confPath, []byte(defaultServerConf), 0o644)
}

// downloadJarIfNeeded checks if the expected JAR version exists, and downloads it if not.
// Old JAR versions are cleaned up before downloading.
func downloadJarIfNeeded(ctx context.Context, cfg config.SuwayomiConfig, suwayomiDir string) error {
	version := cfg.Version
	if version == "" {
		version = "v2.0.1833"
	}

	expectedJar := fmt.Sprintf(suwayomiJarPattern, version)
	expectedPath := filepath.Join(suwayomiDir, expectedJar)

	// Check if already downloaded
	if _, err := os.Stat(expectedPath); err == nil {
		log.Info().Str("jar", expectedJar).Msg("Suwayomi JAR already present")
		return nil
	}

	// Clean up old JAR versions
	cleanOldJars(suwayomiDir)

	// Build download URL
	var downloadURL string
	if cfg.UsePreview {
		downloadURL = fmt.Sprintf(suwayomiPreviewURL, version, expectedJar)
	} else {
		downloadURL = fmt.Sprintf(suwayomiStableURL, version, expectedJar)
	}

	log.Info().
		Str("version", version).
		Bool("preview", cfg.UsePreview).
		Str("url", downloadURL).
		Msg("downloading Suwayomi JAR")

	if err := downloadFile(ctx, downloadURL, expectedPath); err != nil {
		return fmt.Errorf("download %s: %w", expectedJar, err)
	}

	log.Info().Str("jar", expectedJar).Msg("Suwayomi JAR downloaded successfully")
	return nil
}

// cleanOldJars removes all .jar files from the Suwayomi directory.
func cleanOldJars(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			path := filepath.Join(dir, entry.Name())
			log.Info().Str("file", entry.Name()).Msg("removing old Suwayomi JAR")
			_ = os.Remove(path)
		}
	}
}

// downloadFile downloads a file from url to destPath with progress logging.
func downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	// Write to temp file first, then rename for atomicity
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	written, err := io.Copy(f, resp.Body)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write file: %w", err)
	}

	log.Info().Int64("bytes", written).Msg("download complete")

	return os.Rename(tmpPath, destPath)
}
