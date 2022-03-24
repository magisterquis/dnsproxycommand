package server

/*
 * conn.go
 * Manage connections
 * By J. Stuart McMurray
 * Created 20220323
 * Last Modified 20220323
 */

import (
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

var (
	conns      = make(map[string]*conn)
	connNextID = uint64(time.Now().UnixNano())
	connsL     sync.RWMutex
)

/* conn is a connection between a DNS client and its proxied TCP connection. */
type conn struct {
	start time.Time /* Read-only */
	last  time.Time /* Last activity */
	timeL sync.Mutex

	/* Next message numbers. */
	nextFwd  uint64
	nextFwdL sync.Mutex
	nextRev  uint64
	nextRevL sync.Mutex

	c      net.Conn /* Upstream connection. */
	closed bool
	cL     sync.Mutex
}

/* updateLast updates c.last. */
func (c *conn) updateLast() {
	c.timeL.Lock()
	defer c.timeL.Unlock()
	c.last = time.Now()
}

/* newConn makes a new conn and returns its ID. */
func newConn() (string, error) {
	c := new(conn)
	var err error

	/* Connect upstream */
	c.c, err = net.Dial("tcp", upstreamAddr)
	if nil != err {
		return "", err
	}
	c.start = time.Now()

	/* Give it an ID. */
	connsL.Lock()
	defer connsL.Unlock()
	id := strconv.FormatUint(connNextID, 36)
	connNextID++

	/* Save for future use. */
	conns[id] = c

	log.Printf(
		"[%s] New connection: %s->%s",
		id,
		c.c.LocalAddr(),
		c.c.RemoteAddr(),
	)

	return id, nil
}

/* getConn gets a conn by ID. */
func getConn(id string) (*conn, bool) {
	connsL.RLock()
	defer connsL.RUnlock()
	c, ok := conns[id]
	return c, ok
}

/* deleteConn tries to delete (and close) the conn with the given id.  If the
conn has already been deleted, deleteConn is a no-op. */
func deleteConn(id string) {
	/* Get hold of the conn in question. */
	connsL.Lock()
	c, ok := conns[id]
	/* If we got it, remove it before anybody else can get it. */
	if ok {
		delete(conns, id)
	}
	defer connsL.Unlock()

	/* If we don't have it, nothing else to do. */
	if !ok {
		return
	}

	/* Close the underlying connection. */
	go closeConn(id, c)
}

/* pruneConns prunes the conns which haven't had any activity for a while. */
func pruneConns(interval time.Duration) {
	last := time.Now() /* Last sweep time. */
	for {
		time.Sleep(interval)
		pruneConnsSince(last)
		last = time.Now()
	}
}

/* pruneConnsSince makes one sweep through the conns and closes the ones which
haven't been updated since the last sweep. */
func pruneConnsSince(last time.Time) {
	connsL.Lock()
	defer connsL.Unlock()
	for id, c := range conns {
		if c.last.Before(last) {
			log.Printf("[%s] DNS timeout", id)
			delete(conns, id)
			go closeConn(id, c)
		}
	}
}

/* closeConn closes the given conn.  The ID is used for logging.  If wg is
not nil, its Done method will be called on return. */
func closeConn(id string, c *conn) {
	if err := c.c.Close(); nil != err {
		log.Printf("[%s] Closing connection: %s", id, err)
		return
	}
	log.Printf("[%s] Closed connection", id)
}
