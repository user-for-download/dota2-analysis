package worker

import "errors"

var ErrAlreadySeen = errors.New("match already seen")