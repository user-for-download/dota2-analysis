package refdatapg

import (
	"bytes"
	"encoding/json"
)

func jsonbOrNull(raw json.RawMessage) any {
	s := bytes.TrimSpace(raw)
	if len(s) == 0 || bytes.Equal(s, []byte("null")) {
		return nil
	}
	return raw
}