package yamlstore_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/storage"
	"github.com/jackiabishop/mileminder/internal/storage/storagetest"
	"github.com/jackiabishop/mileminder/internal/storage/yamlstore"
	"gopkg.in/yaml.v3"
)

func TestYAMLStoreConformance(t *testing.T) {
	storagetest.RunConformance(t, func(t *testing.T) storage.Store {
		return yamlstore.New(t.TempDir())
	})
}

// TestListSkipsNonYAMLAndCorruptFiles asserts ListVehicles ignores non-.yml
// entries and files that fail to parse, rather than failing the whole call.
func TestListSkipsNonYAMLAndCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	st := yamlstore.New(dir)
	ctx := context.Background()

	if err := st.SaveVehicle(ctx, "golf", sample()); err != nil {
		t.Fatalf("SaveVehicle: %v", err)
	}
	// A non-YAML file, a subdirectory, and a corrupt .yml must all be skipped.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yml"), []byte(":\n  bad: [unclosed"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListVehicles(ctx)
	if err != nil {
		t.Fatalf("ListVehicles: %v", err)
	}
	if len(got) != 1 || got[0].ID != "golf" {
		t.Fatalf("want only [golf], got %+v", got)
	}
}

// TestSavedBytesMatchLegacyEncoder is the hard on-disk-compatibility guard: the
// bytes SaveVehicle writes must be byte-identical to what the previous
// yaml.NewEncoder(file).Encode(&data) path produced, so existing ~/.mileminder
// files remain readable and round-trip unchanged.
func TestSavedBytesMatchLegacyEncoder(t *testing.T) {
	dir := t.TempDir()
	st := yamlstore.New(dir)
	data := sample()

	if err := st.SaveVehicle(context.Background(), "golf", data); err != nil {
		t.Fatalf("SaveVehicle: %v", err)
	}
	gotBytes, err := os.ReadFile(filepath.Join(dir, "golf.yml"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}

	// Reproduce the exact legacy encoding.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(data); err != nil {
		t.Fatal(err)
	}
	enc.Close()

	if !bytes.Equal(gotBytes, buf.Bytes()) {
		t.Fatalf("on-disk bytes diverge from legacy encoder\n--- got ---\n%s\n--- want ---\n%s", gotBytes, buf.Bytes())
	}
}

// TestAtomicWriteFilePerms verifies vehicle files land at 0644, matching the
// perms the previous os.Create path produced (rather than os.CreateTemp's 0600).
func TestAtomicWriteFilePerms(t *testing.T) {
	dir := t.TempDir()
	st := yamlstore.New(dir)
	if err := st.SaveVehicle(context.Background(), "golf", sample()); err != nil {
		t.Fatalf("SaveVehicle: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "golf.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0644 {
		t.Fatalf("file perms: want 0644, got %o", got)
	}
}

func sample() *model.VehicleData {
	return &model.VehicleData{
		Vehicle: "Golf",
		Plan: model.Plan{
			Start:           time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			End:             time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
			AnnualAllowance: 10000,
			StartMiles:      5000,
			ExcessRate:      8,
		},
		Readings: map[string]int{"2025-01-01": 5000, "2025-03-01": 5600},
	}
}
