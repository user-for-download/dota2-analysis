package dedup

import "errors"

// ErrAlreadySeen indicates a payload has already been processed.
var ErrAlreadySeen = errors.New("dedup: item already seen")
