package server

/*
 * reverse.go
 * Handle reverse transmission requests
 * By J. Stuart McMurray
 * Created 20220323
 * Last Modified 20220324
 */

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	/* maxTXTLen is the maximum number of payload bytes which we'll send
	in a TXT record. */
	maxTXTLen = 189
	/* readWait is the maximum amount of time to wait for a read from the
	TCP connection. */
	readWait = 10 * time.Millisecond
)

/* handleReverse handles requests to send data back to the client. */
func handleReverse(ctr, id string) ([]byte, error) {
	/* Make sure the counter is a number. */
	cn, err := strconv.ParseUint(ctr, 36, 64)
	if nil != err {
		return nil, fmt.Errorf("unable to parse counter: %w", err)
	}

	/* Get the conn. */
	c, ok := getConn(id)
	if !ok {
		return nil, fmt.Errorf("unknown ID")
	}

	/* Make sure we're at the right ID. */
	c.nextRevL.Lock()
	defer c.nextRevL.Unlock()
	if cn != c.nextRev {
		return nil, nil
	}
	c.nextRev++

	/* Try to read from upstream. */
	buf := make([]byte, maxTXTLen)
	if err := c.c.SetReadDeadline(time.Now().Add(readWait)); nil != err {
		deleteConn(id)
		return nil, fmt.Errorf(
			"setting network read deadline: %w",
			err,
		)
	}
	c.updateLast()
	n, err := c.c.Read(buf)
	c.updateLast()
	if 0 != n { /* Read something */
		return buf[:n], nil
	}
	if nil != err { /* Read nothing, and an error. */
		var te interface{ Timeout() bool }
		if errors.As(err, &te) && te.Timeout() { /* Timeout */
			return []byte{}, nil
		}
		return nil, fmt.Errorf("reading from network: %w", err)
	}
	/* No read, no error. */
	return []byte{}, nil
}
