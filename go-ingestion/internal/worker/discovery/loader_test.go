package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadQueries(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "q1.sql"), []byte("SELECT 1;"), 0644)
	if err != nil { t.Fatal(err) }
	
	err = os.WriteFile(filepath.Join(dir, "q2.sql"), []byte("  \n "), 0644)
	if err != nil { t.Fatal(err) }
	
	err = os.WriteFile(filepath.Join(dir, "not_sql.txt"), []byte("text"), 0644)
	if err != nil { t.Fatal(err) }

	qs, err := LoadQueries(dir)
	if err != nil {
		t.Fatalf("LoadQueries: %v", err)
	}

	if len(qs) != 1 {
		t.Errorf("len(qs) = %d, want 1", len(qs))
	}
	if qs["q1"] != "SELECT 1;" {
		t.Errorf("qs[q1] = %q", qs["q1"])
	}

	// Empty dir
	emptyDir := t.TempDir()
	_, err = LoadQueries(emptyDir)
	if err == nil {
		t.Error("expected error on empty dir")
	}

	// Non-existent dir
	_, err = LoadQueries("/non/existent/dir/xyz")
	if err == nil {
		t.Error("expected error on non-existent dir")
	}
}