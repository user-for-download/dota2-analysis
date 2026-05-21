package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadQueries(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	out := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		key := strings.TrimSuffix(name, ".sql")
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		s := strings.TrimSpace(string(body))
		if s == "" {
			continue
		}
		out[key] = s
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no .sql files found in %s", dir)
	}
	return out, nil
}