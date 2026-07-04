package cmd

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jackiabishop/mileminder/internal/storage/yamlstore"
)

var backupOutput string

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Archive the MileMinder data directory",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := yamlstore.DefaultDir()
		if err != nil {
			return fmt.Errorf("locate data directory: %w", err)
		}

		outPath := backupOutput
		if outPath == "" {
			outPath = defaultBackupPath(time.Now())
		}

		count, err := writeBackup(dir, outPath)
		if err != nil {
			return err
		}
		fmt.Printf("Wrote backup to %s (%d files)\n", outPath, count)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "Output archive path")
}

func defaultBackupPath(now time.Time) string {
	return "./mileminder-backup-" + now.Format("20060102-150405") + ".tar.gz"
}

func writeBackup(srcDir, outPath string) (int, error) {
	if err := ensureOutputOutsideSource(srcDir, outPath); err != nil {
		return 0, err
	}

	files, err := backupFiles(srcDir)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("no MileMinder data files found in %s", srcDir)
	}

	out, err := os.Create(outPath)
	if err != nil {
		return 0, fmt.Errorf("create backup archive %q: %w", outPath, err)
	}
	outClosed := false
	defer func() {
		if !outClosed {
			_ = out.Close()
		}
	}()

	gz := gzip.NewWriter(out)
	gzClosed := false
	defer func() {
		if !gzClosed {
			_ = gz.Close()
		}
	}()

	tw := tar.NewWriter(gz)
	tarClosed := false
	defer func() {
		if !tarClosed {
			_ = tw.Close()
		}
	}()

	for _, file := range files {
		if err := addBackupFile(tw, srcDir, file); err != nil {
			return 0, err
		}
	}
	if err := tw.Close(); err != nil {
		return 0, fmt.Errorf("close tar archive %q: %w", outPath, err)
	}
	tarClosed = true
	if err := gz.Close(); err != nil {
		return 0, fmt.Errorf("close gzip archive %q: %w", outPath, err)
	}
	gzClosed = true
	if err := out.Close(); err != nil {
		return 0, fmt.Errorf("close backup archive %q: %w", outPath, err)
	}
	outClosed = true
	return len(files), nil
}

func backupFiles(srcDir string) ([]string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("data directory %s does not exist", srcDir)
		}
		return nil, fmt.Errorf("read data directory %s: %w", srcDir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat data file %q: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		name := entry.Name()
		// "current" and "settings" are the two extensionless store documents
		// (default-vehicle pointer, user preferences); everything else is a
		// per-vehicle <id>.yml.
		if name == "current" || name == "settings" || filepath.Ext(name) == ".yml" {
			files = append(files, name)
		}
	}
	return files, nil
}

func addBackupFile(tw *tar.Writer, srcDir, name string) error {
	srcPath := filepath.Join(srcDir, name)
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat data file %q: %w", srcPath, err)
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("create tar header for %q: %w", srcPath, err)
	}
	header.Name = path.Join("mileminder", name)

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header for %q: %w", srcPath, err)
	}

	in, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open data file %q: %w", srcPath, err)
	}
	defer in.Close()

	if _, err := io.Copy(tw, in); err != nil {
		return fmt.Errorf("write data file %q to archive: %w", srcPath, err)
	}
	return nil
}

func ensureOutputOutsideSource(srcDir, outPath string) error {
	srcAbs, err := filepath.Abs(srcDir)
	if err != nil {
		return fmt.Errorf("resolve data directory %q: %w", srcDir, err)
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return fmt.Errorf("resolve backup archive path %q: %w", outPath, err)
	}

	srcAbs = filepath.Clean(srcAbs)
	outAbs = filepath.Clean(outAbs)
	if outAbs == srcAbs || strings.HasPrefix(outAbs, srcAbs+string(os.PathSeparator)) {
		return fmt.Errorf("backup archive path %s is inside the data directory %s", outPath, srcDir)
	}
	return nil
}
