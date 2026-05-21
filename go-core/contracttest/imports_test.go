package contracttest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCoreHasNoDownstreamImports ensures go-core never imports from
// go-analysis or go-ingestion. This enforces the module-boundary
// contract: core is dependency-free of downstream modules.
func TestCoreHasNoDownstreamImports(t *testing.T) {
	// Match downstream modules but NOT go-dota2-core itself.
	forbidden := []string{
		"go-dota2-analysis\"", // trailing quote ensures exact match
		"/go-dota2\"",         // ingestion: .../go-dota2 (not .../go-dota2-core)
	}

	fset := token.NewFileSet()
	hadErr := false
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/.git/") || strings.Contains(path, "/testdata/") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil // skip files that can't be parsed
		}
		for _, imp := range f.Imports {
			val := imp.Path.Value // includes quotes
			for _, bad := range forbidden {
				if strings.Contains(val, bad) {
					t.Errorf("%s imports forbidden module: %s", path, val)
					hadErr = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if hadErr {
		t.FailNow()
	}
}
