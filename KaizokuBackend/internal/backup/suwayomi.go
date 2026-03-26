package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	backupSubdir  = "backups/suwayomi"
	retentionDays = 7
)

// h2FileExtensions are the file extensions used by H2 database files.
var h2FileExtensions = []string{".h2.db", ".mv.db", ".trace.db", ".lock.db", ".db"}

// BackupSuwayomiDB creates a compressed tarball of Suwayomi's H2 database files.
// configDir is the root config directory (e.g., /config in Docker).
// tag is appended to the filename (e.g., "startup" or "daily").
// Returns the backup file path and size, or an error.
func BackupSuwayomiDB(configDir, tag string) (string, int64, error) {
	suwayomiDir := filepath.Join(configDir, "Suwayomi")
	backupDir := filepath.Join(configDir, backupSubdir)

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create backup dir: %w", err)
	}

	// Find H2 database files
	dbFiles, err := findH2Files(suwayomiDir)
	if err != nil {
		return "", 0, fmt.Errorf("find H2 files: %w", err)
	}
	if len(dbFiles) == 0 {
		log.Info().Msg("backup: no H2 database files found, skipping")
		return "", 0, nil
	}

	// Create tarball: YYYY-MM-DD_tag.tar.gz
	filename := fmt.Sprintf("%s_%s.tar.gz", time.Now().Format("2006-01-02"), tag)
	backupPath := filepath.Join(backupDir, filename)

	size, err := createTarGz(backupPath, suwayomiDir, dbFiles)
	if err != nil {
		_ = os.Remove(backupPath)
		return "", 0, fmt.Errorf("create backup: %w", err)
	}

	log.Info().
		Str("file", filename).
		Int64("bytes", size).
		Int("dbFiles", len(dbFiles)).
		Msg("backup: Suwayomi DB backup created")

	return backupPath, size, nil
}

// CleanupOldBackups removes backup files older than retentionDays.
func CleanupOldBackups(configDir string) {
	backupDir := filepath.Join(configDir, backupSubdir)

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(backupDir, entry.Name())
			if err := os.Remove(path); err != nil {
				log.Warn().Err(err).Str("file", entry.Name()).Msg("backup: failed to remove old backup")
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		log.Info().Int("count", removed).Msg("backup: cleaned up old backups")
	}
}

// findH2Files returns relative paths of H2 database files in suwayomiDir.
func findH2Files(suwayomiDir string) ([]string, error) {
	entries, err := os.ReadDir(suwayomiDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		for _, ext := range h2FileExtensions {
			if strings.HasSuffix(name, ext) {
				files = append(files, entry.Name())
				break
			}
		}
	}
	return files, nil
}

// createTarGz creates a gzipped tarball at destPath containing the specified files from baseDir.
func createTarGz(destPath, baseDir string, files []string) (int64, error) {
	f, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, name := range files {
		fullPath := filepath.Join(baseDir, name)
		if err := addFileToTar(tw, fullPath, name); err != nil {
			return 0, fmt.Errorf("add %s: %w", name, err)
		}
	}

	// Close writers to flush
	if err := tw.Close(); err != nil {
		return 0, err
	}
	if err := gw.Close(); err != nil {
		return 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// addFileToTar adds a single file to the tar writer.
func addFileToTar(tw *tar.Writer, fullPath, name string) error {
	file, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	return err
}
