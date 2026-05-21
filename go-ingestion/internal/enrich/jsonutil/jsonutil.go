package jsonutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func ReadBody(resp *http.Response) ([]byte, error) {
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}

func Unmarshal[T any](data []byte) (T, error) {
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("unmarshal: %w", err)
	}
	return result, nil
}

func UnmarshalList[T any](data []byte) ([]T, error) {
	var result []T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal list: %w", err)
	}
	return result, nil
}
