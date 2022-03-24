package server

/*
 * forward.go
 * Handle forward transmission requests
 * By J. Stuart McMurray
 * Created 20220323
 * Last Modified 20220323
 */

import (
	"encoding/base32"
	"fmt"
	"strconv"
	"strings"
)

/* dec decodes base32'd data. */
var dec = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString

/* handleForward handles requests to send data upstream. */
func handleForward(ctr, payload, id string) ([]byte, error) {
	/* Make sure the counter is a number. */
	cn, err := strconv.ParseUint(ctr, 36, 64)
	if nil != err {
		return nil, fmt.Errorf("unable to parse counter: %w", err)
	}

	/* Make sure the payload is base32'd */
	b, err := dec(strings.ToUpper(payload))
	if nil != err {
		return nil, fmt.Errorf("decoding payload: %w", err)
	}

	/* Get the conn. */
	c, ok := getConn(id)
	if !ok {
		return nil, fmt.Errorf("unknown ID")
	}

	/* Make sure we're at the right ID. */
	c.nextFwdL.Lock()
	defer c.nextFwdL.Unlock()
	if cn != c.nextFwd {
		return nil, nil
	}
	c.nextFwd++

	/* Send the data upstream. */
	c.updateLast()
	if _, err := c.c.Write(b); nil != err {
		deleteConn(id)
		return nil, fmt.Errorf("sending to network: %w", err)
	}
	c.updateLast()

	return []byte{}, nil
}
