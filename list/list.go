package list

import (
	"time"
)

type Store struct {
	data    string
	expires time.Duration
}
