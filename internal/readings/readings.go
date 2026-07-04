// Package readings holds the shared, store-agnostic rules for odometer
// readings: parsing the CSV interchange format, merging imported rows into an
// existing readings map, and the validation both the CLI and the HTTP layer
// delegate to so the rules cannot drift between surfaces (the cmd/api
// divergence class tracked in #29). Persistence stays with the caller.
//
// The CSV format is exactly what the export endpoint writes: a "date,miles"
// header, then one "YYYY-MM-DD,<int>" row per reading. Export → import must
// reproduce an identical readings map.
package readings

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reading is one parsed CSV row.
type Reading struct {
	Date  string // YYYY-MM-DD
	Miles int
}

// RowError describes one invalid CSV row. Line 0 means the error applies to
// the file as a whole (unreadable input) rather than a specific row.
type RowError struct {
	Line int    `json:"line"`
	Msg  string `json:"message"`
}

func (e RowError) Error() string {
	if e.Line == 0 {
		return e.Msg
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

// Report summarises what a Merge did.
type Report struct {
	Added       int `json:"added"`
	Skipped     int `json:"skipped"`
	Overwritten int `json:"overwritten"`
}

// ParseCSV reads the export-format CSV and returns the parsed rows plus every
// row-level problem found — it keeps going after an error so the caller can
// report the whole file at once (all-or-nothing imports reject on any error).
// Blank lines are tolerated; a date appearing twice within the file is an
// error because the import would be order-dependent.
func ParseCSV(r io.Reader) ([]Reading, []RowError) {
	rd := csv.NewReader(r)
	rd.FieldsPerRecord = -1 // field-count problems become per-row errors below
	rd.TrimLeadingSpace = true

	var rows []Reading
	var errs []RowError
	seen := map[string]int{} // date -> first line it appeared on
	headerSeen := false

	for {
		record, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			if pe, ok := err.(*csv.ParseError); ok {
				errs = append(errs, RowError{Line: pe.Line, Msg: pe.Err.Error()})
				continue
			}
			errs = append(errs, RowError{Line: 0, Msg: fmt.Sprintf("read error: %v", err)})
			break
		}
		line, _ := rd.FieldPos(0)

		// encoding/csv skips truly empty lines; also tolerate whitespace-only rows.
		if len(record) == 1 && strings.TrimSpace(record[0]) == "" {
			continue
		}

		if !headerSeen {
			headerSeen = true
			if len(record) != 2 ||
				!strings.EqualFold(strings.TrimSpace(record[0]), "date") ||
				!strings.EqualFold(strings.TrimSpace(record[1]), "miles") {
				errs = append(errs, RowError{Line: line, Msg: `expected header "date,miles"`})
			}
			continue
		}

		if len(record) != 2 {
			errs = append(errs, RowError{Line: line, Msg: fmt.Sprintf("expected 2 fields (date,miles), got %d", len(record))})
			continue
		}

		date := strings.TrimSpace(record[0])
		if _, err := time.Parse("2006-01-02", date); err != nil {
			errs = append(errs, RowError{Line: line, Msg: fmt.Sprintf("invalid date %q (want YYYY-MM-DD)", date)})
			continue
		}
		miles, err := strconv.Atoi(strings.TrimSpace(record[1]))
		if err != nil {
			errs = append(errs, RowError{Line: line, Msg: fmt.Sprintf("invalid miles %q (want a whole number)", strings.TrimSpace(record[1]))})
			continue
		}
		if miles < 0 {
			errs = append(errs, RowError{Line: line, Msg: fmt.Sprintf("miles must not be negative, got %d", miles)})
			continue
		}
		if first, dup := seen[date]; dup {
			errs = append(errs, RowError{Line: line, Msg: fmt.Sprintf("duplicate date %s (also on line %d)", date, first)})
			continue
		}
		seen[date] = line
		rows = append(rows, Reading{Date: date, Miles: miles})
	}

	if !headerSeen {
		errs = append(errs, RowError{Line: 0, Msg: `empty file: expected header "date,miles"`})
	}
	return rows, errs
}

// Merge combines imported rows into a copy of existing. A row whose date is
// already present is skipped (existing data wins) unless overwrite is set and
// the value actually differs; identical values count as skipped either way,
// which is what makes export → import into the same vehicle a clean no-op.
func Merge(existing map[string]int, rows []Reading, overwrite bool) (map[string]int, Report) {
	merged := make(map[string]int, len(existing)+len(rows))
	for date, miles := range existing {
		merged[date] = miles
	}

	var rep Report
	for _, row := range rows {
		current, exists := merged[row.Date]
		switch {
		case !exists:
			merged[row.Date] = row.Miles
			rep.Added++
		case overwrite && current != row.Miles:
			merged[row.Date] = row.Miles
			rep.Overwritten++
		default:
			rep.Skipped++
		}
	}
	return merged, rep
}

// CheckMonotonic verifies the odometer never decreases in date order — the
// bulk equivalent of the single-add below-max rule. It reports the first
// offending pair; note it runs over whatever map it is given, so a
// pre-existing (previously forced) decrease also trips it.
func CheckMonotonic(readings map[string]int) error {
	dates := make([]string, 0, len(readings))
	for d := range readings {
		dates = append(dates, d)
	}
	sort.Strings(dates) // YYYY-MM-DD sorts chronologically

	for i := 1; i < len(dates); i++ {
		prev, cur := dates[i-1], dates[i]
		if readings[cur] < readings[prev] {
			return fmt.Errorf("odometer decreases from %d on %s to %d on %s", readings[prev], prev, readings[cur], cur)
		}
	}
	return nil
}

// BelowMax reports the highest existing reading and whether miles is below it
// — the single-add validation rule shared by cmd/add.go and the API's
// HandleAddReading (#29). The force gate and the user-facing message stay
// with each caller deliberately: the surfaces word the override differently.
func BelowMax(readings map[string]int, miles int) (max int, below bool) {
	for _, m := range readings {
		if m > max {
			max = m
		}
	}
	return max, miles < max
}
