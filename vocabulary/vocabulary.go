package vocabulary

import (
	"time"
)

type Store struct {
	data    string
	expires time.Duration
}
