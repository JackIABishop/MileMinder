package cmd

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteBackupArchivesDataFiles(t *testing.T) {
	srcDir := t.TempDir()
	outPath := filepath.Join(t.TempDir(), "backup.tar.gz")

	writeFile(t, filepath.Join(srcDir, "golf.yml"), "vehicle: Golf\n")
	writeFile(t, filepath.Join(srcDir, "mini.yml"), "vehicle: Mini\n")
	writeFile(t, filepath.Join(srcDir, "current"), "golf")
	writeFile(t, filepath.Join(srcDir, "settings"), "currency: EUR\ndistance_unit: mi\n")
	writeFile(t, filepath.Join(srcDir, "notes.txt"), "ignore me")
	if err := os.Mkdir(filepath.Join(srcDir, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "nested", "ignored.yml"), "ignore me too")

	count, err := writeBackup(srcDir, outPath)
	if err != nil {
		t.Fatalf("writeBackup: %v", err)
	}
	if count != 4 {
		t.Fatalf("file count: want 4, got %d", count)
	}

	got := readArchive(t, outPath)
	want := map[string]string{
		"mileminder/current":  "golf",
		"mileminder/settings": "currency: EUR\ndistance_unit: mi\n",
		"mileminder/golf.yml": "vehicle: Golf\n",
		"mileminder/mini.yml": "vehicle: Mini\n",
	}
	if len(got) != len(want) {
		t.Fatalf("archive entries: want %d, got %d: %#v", len(want), len(got), got)
	}
	for name, wantBody := range want {
		if gotBody, ok := got[name]; !ok {
			t.Fatalf("archive missing %s; got %#v", name, got)
		} else if gotBody != wantBody {
			t.Fatalf("%s contents: want %q, got %q", name, wantBody, gotBody)
		}
	}
}

func TestWriteBackupErrorsWhenDataDirMissing(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "backup.tar.gz")

	if _, err := writeBackup(filepath.Join(t.TempDir(), "missing"), outPath); err == nil {
		t.Fatal("writeBackup: want missing directory error")
	}
	if _, err := os.Stat(outPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive should not be created, stat error = %v", err)
	}
}

func TestWriteBackupErrorsWhenDataDirEmpty(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "backup.tar.gz")

	if _, err := writeBackup(t.TempDir(), outPath); err == nil {
		t.Fatal("writeBackup: want empty directory error")
	}
	if _, err := os.Stat(outPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive should not be created, stat error = %v", err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func readArchive(t *testing.T, path string) map[string]string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	entries := map[string]string{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = string(body)
	}
	return entries
}
