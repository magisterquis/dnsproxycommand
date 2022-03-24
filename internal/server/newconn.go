package server

/*
 * newconn.go
 * Handle new connection requests
 * By J. Stuart McMurray
 * Created 20220323
 * Last Modified 20220323
 */

import (
	"fmt"
	"strconv"
	"time"

	lru "github.com/hashicorp/golang-lru"
)

const (
	/* tsCacheSize is the number of new connection timestamps we
	remember. */
	tsCacheSize = 1024 * 1024

	/* maxTSOff is the maximum amount a new connection request timestamp
	may be off of the local clock. */
	maxTSOff = 24 * time.Hour
)

/* seenTSCache attempts to prevent replays of queries for new connections. */
var seenTSCache *lru.TwoQueueCache

func init() {
	var err error
	seenTSCache, err = lru.New2Q(tsCacheSize)
	if nil != err {
		panic(fmt.Sprintf("making timestamp cache: %s", err))
	}
}

/* handleNewConn handles requests for new connections. */
func handleNewConn(l string) ([]byte, error) {
	/* Make sure the timestamp is within a day or so. */
	n, err := strconv.ParseInt(l, 10, 64)
	if nil != err {
		return nil, fmt.Errorf("parsing timestamp: %w", err)
	}
	diff := time.Until(time.Unix(0, n))
	if 0 > diff {
		diff *= -1
	}
	if maxTSOff < diff {
		return nil, fmt.Errorf(
			"timestamp difference is too big (%s > %s)",
			diff,
			maxTSOff,
		)
	}

	/* Make sure we've not seen this timestamp. */
	if seenTSCache.Contains(l) {
		return nil, nil
	}
	seenTSCache.Add(l, nil)

	/* Connect upstream. */
	id, err := newConn()
	if nil != err {
		return nil, fmt.Errorf("connecting upstream: %w", err)
	}
	return []byte(id), nil
}
