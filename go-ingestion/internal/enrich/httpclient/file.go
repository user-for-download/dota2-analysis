package httpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type FileClient struct{}

func NewFileClient() *FileClient { return &FileClient{} }

var _ HTTPClient = (*FileClient)(nil)

func (f *FileClient) Get(ctx context.Context, rawURL string) (*http.Response, error) {
	if !strings.HasPrefix(rawURL, "file://") {
		return nil, fmt.Errorf("file client only handles file://, got %s", rawURL)
	}

	path := strings.TrimPrefix(rawURL, "file://")
	path = filepath.Clean(path)

	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("expected file, got directory: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return &http.Response{
		Status:        "200 OK",
		StatusCode:    200,
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)),
		Header:        http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}