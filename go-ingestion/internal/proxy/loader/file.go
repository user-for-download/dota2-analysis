package loader

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

type FileSource struct {
	Path string
}

func (f FileSource) Name() string { return "file:" + f.Path }

func (f FileSource) Load(_ context.Context) ([]string, error) {
	if f.Path == "" {
		return nil, fmt.Errorf("file path is empty")
	}

	fh, err := os.Open(f.Path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", f.Path, err)
	}
	defer fh.Close()

	var out []string
	s := bufio.NewScanner(fh)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, s.Err()
}