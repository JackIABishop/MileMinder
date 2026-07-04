package readings

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestParseCSVValid(t *testing.T) {
	rows, errs := ParseCSV(strings.NewReader("date,miles\n2025-01-01,5000\n2025-02-01,5500\n"))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := []Reading{{"2025-01-01", 5000}, {"2025-02-01", 5500}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
}

func TestParseCSVCRLFAndCaseInsensitiveHeader(t *testing.T) {
	rows, errs := ParseCSV(strings.NewReader("Date,Miles\r\n2025-01-01,5000\r\n"))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(rows) != 1 || rows[0] != (Reading{"2025-01-01", 5000}) {
		t.Fatalf("rows = %v", rows)
	}
}

func TestParseCSVBlankLinesTolerated(t *testing.T) {
	rows, errs := ParseCSV(strings.NewReader("date,miles\n\n2025-01-01,5000\n   \n"))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want 1 row", rows)
	}
}

func TestParseCSVErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLine int
		wantMsg  string // substring
	}{
		{"wrong header", "when,how far\n2025-01-01,5000\n", 1, `expected header "date,miles"`},
		{"missing header", "2025-01-01,5000\n", 1, `expected header "date,miles"`},
		{"bad date", "date,miles\n01/02/2025,5000\n", 2, "invalid date"},
		{"non-numeric miles", "date,miles\n2025-01-01,about 5k\n", 2, "invalid miles"},
		{"decimal miles", "date,miles\n2025-01-01,5000.5\n", 2, "invalid miles"},
		{"negative miles", "date,miles\n2025-01-01,-5\n", 2, "must not be negative"},
		{"too many fields", "date,miles\n2025-01-01,5000,extra\n", 2, "expected 2 fields"},
		{"duplicate date", "date,miles\n2025-01-01,5000\n2025-01-01,5100\n", 3, "duplicate date 2025-01-01 (also on line 2)"},
		{"empty file", "", 0, "empty file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := ParseCSV(strings.NewReader(tt.input))
			if len(errs) == 0 {
				t.Fatal("want errors, got none")
			}
			found := false
			for _, e := range errs {
				if e.Line == tt.wantLine && strings.Contains(e.Msg, tt.wantMsg) {
					found = true
				}
			}
			if !found {
				t.Fatalf("no error on line %d containing %q; got %v", tt.wantLine, tt.wantMsg, errs)
			}
		})
	}
}

func TestParseCSVCollectsAllErrors(t *testing.T) {
	_, errs := ParseCSV(strings.NewReader("date,miles\nbad,5000\n2025-01-01,worse\n"))
	if len(errs) != 2 {
		t.Fatalf("want 2 errors (whole file reported), got %v", errs)
	}
}

func TestMerge(t *testing.T) {
	existing := map[string]int{"2025-01-01": 5000, "2025-02-01": 5500}
	rows := []Reading{
		{"2025-01-01", 5000}, // identical -> skipped even with overwrite
		{"2025-02-01", 5600}, // conflict
		{"2025-03-01", 6000}, // new
	}

	t.Run("skip by default", func(t *testing.T) {
		merged, rep := Merge(existing, rows, false)
		if rep != (Report{Added: 1, Skipped: 2}) {
			t.Fatalf("report = %+v", rep)
		}
		if merged["2025-02-01"] != 5500 {
			t.Fatal("existing value should win without overwrite")
		}
		if merged["2025-03-01"] != 6000 {
			t.Fatal("new row not added")
		}
		if existing["2025-03-01"] != 0 {
			t.Fatal("Merge mutated its input map")
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		merged, rep := Merge(existing, rows, true)
		if rep != (Report{Added: 1, Skipped: 1, Overwritten: 1}) {
			t.Fatalf("report = %+v", rep)
		}
		if merged["2025-02-01"] != 5600 {
			t.Fatal("overwrite did not replace conflicting value")
		}
	})
}

func TestCheckMonotonic(t *testing.T) {
	if err := CheckMonotonic(map[string]int{"2025-01-01": 5000, "2025-02-01": 5500, "2025-03-01": 5500}); err != nil {
		t.Fatalf("non-decreasing readings flagged: %v", err)
	}
	err := CheckMonotonic(map[string]int{"2025-01-01": 5000, "2025-02-01": 4000})
	if err == nil {
		t.Fatal("decrease not flagged")
	}
	for _, want := range []string{"5000", "2025-01-01", "4000", "2025-02-01"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name offending pair (%s)", err, want)
		}
	}
}

// A pre-existing (previously forced) decrease in the vehicle blocks an
// otherwise-clean import until force — accepted behaviour, pinned here.
func TestCheckMonotonicTripsOnPreexistingViolation(t *testing.T) {
	existing := map[string]int{"2025-01-01": 5000, "2025-02-01": 4000} // forced dip
	merged, _ := Merge(existing, []Reading{{"2025-03-01", 6000}}, false)
	if err := CheckMonotonic(merged); err == nil {
		t.Fatal("pre-existing decrease should still fail the merged check")
	}
}

func TestBelowMax(t *testing.T) {
	readings := map[string]int{"2025-01-01": 5000, "2025-02-01": 5500}
	if max, below := BelowMax(readings, 5400); max != 5500 || !below {
		t.Fatalf("got max=%d below=%v, want 5500/true", max, below)
	}
	if _, below := BelowMax(readings, 5500); below {
		t.Fatal("equal to max should not be below")
	}
	if max, below := BelowMax(nil, 0); max != 0 || below {
		t.Fatalf("empty readings: got max=%d below=%v", max, below)
	}
}

// Round-trip: a readings map written in the exact export format (header,
// sorted dates, %s,%d rows — see HandleExportCSV) parses and merges into an
// empty vehicle as an identical map.
func TestRoundTripWithExportFormat(t *testing.T) {
	original := map[string]int{
		"2023-06-15": 12000,
		"2024-01-02": 15321,
		"2025-01-01": 20000,
		"2025-07-01": 24500,
	}

	dates := make([]string, 0, len(original))
	for d := range original {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	var sb strings.Builder
	fmt.Fprintln(&sb, "date,miles")
	for _, d := range dates {
		fmt.Fprintf(&sb, "%s,%d\n", d, original[d])
	}

	rows, errs := ParseCSV(strings.NewReader(sb.String()))
	if len(errs) != 0 {
		t.Fatalf("export-format CSV did not parse cleanly: %v", errs)
	}
	merged, rep := Merge(map[string]int{}, rows, false)
	if !reflect.DeepEqual(merged, original) {
		t.Fatalf("round-trip mismatch: got %v, want %v", merged, original)
	}
	if rep.Added != len(original) || rep.Skipped != 0 || rep.Overwritten != 0 {
		t.Fatalf("round-trip report = %+v", rep)
	}
	if err := CheckMonotonic(merged); err != nil {
		t.Fatalf("round-trip data failed monotonic check: %v", err)
	}

	// Importing the same file into the same vehicle is a clean no-op.
	again, rep2 := Merge(merged, rows, false)
	if !reflect.DeepEqual(again, original) || rep2.Skipped != len(original) || rep2.Added != 0 {
		t.Fatalf("re-import not a no-op: map=%v report=%+v", again, rep2)
	}
}
