package synccommittee

import (
	"sync"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/container/queue"
)

// Store defines the caches for various sync committee objects
// such as message(un-aggregated) and contribution(aggregated).
type Store struct {
	messageLock       sync.RWMutex
	messageCache      *queue.PriorityQueue
	contributionLock  sync.RWMutex
	contributionCache *queue.PriorityQueue
}

// NewStore initializes a new sync committee store.
func NewStore() *Store {
	return &Store{
		messageCache:      queue.New(),
		contributionCache: queue.New(),
	}
}
