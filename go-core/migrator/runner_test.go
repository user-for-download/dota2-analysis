package migrator

import (
	"embed"
	"io/fs"
	"strconv"
	"testing"
)

//go:embed testdata
var testdataFS embed.FS

func TestLoadMigrations(t *testing.T) {
	// testdataFS contains files at path "testdata/*.sql".
	// Sub to get an fs.FS rooted at the testdata directory.
	sub, err := fs.Sub(testdataFS, "testdata")
	if err != nil {
		t.Fatalf("sub testdata: %v", err)
	}
	migs, err := loadMigrations(sub)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}

	if len(migs) == 0 {
		t.Fatal("expected at least one migration")
	}

	// Verify ordering
	for i := 1; i < len(migs); i++ {
		if migs[i].version <= migs[i-1].version {
			t.Errorf("migrations not sorted: %d (%s) after %d (%s)",
				migs[i].version, migs[i].filename,
				migs[i-1].version, migs[i-1].filename)
		}
	}

	t.Logf("loaded %d migrations:", len(migs))
	for _, m := range migs {
		t.Logf("  version=%d file=%s sql_len=%d", m.version, m.filename, len(m.sql))
	}
}

func TestFilenameRegex(t *testing.T) {
	tests := []struct {
		name     string
		match    bool
		expected int
	}{
		{"001_init.sql", true, 1},
		{"002_analytics.sql", true, 2},
		{"003_add_indexes.sql", true, 3},
		{"004_reconcile.sql", true, 4},
		{"init.sql", false, 0},
		{"001", false, 0},
		{"abc.sql", false, 0},
		{"001_init.txt", false, 0},
		{"01_init.sql", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := filenameRe.FindStringSubmatch(tt.name)
			if tt.match {
				if match == nil {
					t.Fatal("expected match, got nil")
				}
				v, err := strconv.Atoi(match[1])
				if err != nil {
					t.Fatalf("parse version: %v", err)
				}
				if v != tt.expected {
					t.Errorf("version = %d, want %d", v, tt.expected)
				}
			} else {
				if match != nil {
					t.Fatalf("expected no match, got %v", match)
				}
			}
		})
	}
}
