// Package client implements the DPC client logic.
package client

/*
 * client.go
 * Client side of dnsproxycommand
 * By J. Stuart McMurray
 * Created 20220323
 * Last Modified 20220324
 */

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	/* rbuflen is the read buffer length.  It corresponds to <=63 base32'd
	characters. */
	rBufLen = 39
	/* maxLabelLen is the maximum length of a DNS label. */
	maxLabelLen = 63
	/* pollIncFactor is the maximum by which a poll interval will
	increase. */
	pollIncFactor = 1.5
	/* pollMin is the minimum poll interval. */
	pollMin = time.Nanosecond
)

/* Coders. */
var (
	enc = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString
	dec = base64.RawStdEncoding.DecodeString
)

/* client represents a DPC client. */
type client struct {
	fwd    uint64
	rev    uint64
	domain string

	/* Maximum and current poll intervals. */
	pollMax time.Duration
	pollCur time.Duration
}

/* query makes a TXT query with the given payload and returns the TXT record.
If there is more than one, query returns an error. */
func (c *client) query(sd string) ([]byte, error) {
	/* Send it off and get a reply. */
	txts, err := net.LookupTXT(sd + c.domain)
	if nil != err {
		return nil, fmt.Errorf("querying for %q: %w", sd, err)
	}
	switch len(txts) {
	case 0: /* Not NXDomain, no record. */
		return nil, nil
	case 1: /* What we expect. */
	default:
		return nil, fmt.Errorf(
			"got %d TXT records resolving %q",
			len(txts),
			sd,
		)
	}
	txt := txts[0]

	/* If we got a record with nothing, that's not weird. */
	if 0 == len(txt) {
		return []byte{}, nil
	}

	/* Hopefully it decodes nicely? */
	b, err := dec(txt)
	if nil != err {
		return nil, fmt.Errorf("decoding %q: %w", txt, err)
	}

	return b, nil
}

/* handshake tells the server we want to communicate and gets an ID.  c's
domain is updated with the ID.  handshake must not be called concurrently with
any other of c's methods. */
func (c *client) handshake() error {
	/* Ask server for an ID. */
	id, err := c.query(fmt.Sprintf("%d", time.Now().UnixNano()))
	if nil != err {
		return err
	}
	if 0 == len(id) {
		return fmt.Errorf("empty ID from server")
	}
	/* ID always comes before the domain. */
	c.domain = "." + string(id) + c.domain

	return nil
}

/* proxyForward proxies from stdin to the DNS server. */
func (c *client) proxyForward(done chan<- error) {
	var (
		qn  uint64 /* Query counter. */
		buf = make([]byte, rBufLen)
		qb  bytes.Buffer
	)
	for {
		/* Read a chunk to send. */
		n, rerr := os.Stdin.Read(buf)
		if 0 != n { /* Got something. */
			/* Roll a query. */
			qb.Reset()
			qb.WriteString(strconv.FormatUint(qn, 36))
			qb.WriteRune('.')
			qb.WriteString(enc(buf[:n]))
			/* Send it off. */
			if _, serr := c.query(qb.String()); nil != serr {
				done <- fmt.Errorf("send: %w", serr)
				return
			}
			/* Note how many bytes we sent. */
			atomic.AddUint64(&c.fwd, uint64(n))
			qn++
		}
		if nil != rerr {
			done <- fmt.Errorf("read: %w", rerr)
		}
	}
}

/* proxyBack proxies from DNS to stdout.  It is unsafe to call proxyBack from
multiple goroutines. */
func (c *client) proxyBack(done chan<- error) {
	var (
		qn uint64
		b  bytes.Buffer
	)
	for {
		/* Ask for some data from the server. */
		b, err := c.poll(qn, &b)
		if nil != err {
			done <- err
			return
		}
		qn++
		/* We got some. Don't sleep before next poll. */
		if 0 != len(b) {
			/* Try to proxy to stdout. */
			if _, werr := os.Stdout.Write(b); nil != werr {
				done <- fmt.Errorf("write: %w", werr)
				return
			}
			atomic.AddUint64(&c.rev, uint64(len(b)))
			c.pollCur = pollMin
			continue
		}
		/* Didn't get any.  Sleep more than last time plus some
		jitter. */
		c.pollCur = time.Duration(float64(c.pollCur) * pollIncFactor)
		if c.pollCur > c.pollMax {
			c.pollCur = c.pollMax
		}
		/* Don't actually sleep the full time, to confuse anything
		looking for periodic requests (just in case our labels aren't
		bad enough). */
		st := time.Duration(rand.Int63n(int64(c.pollCur)))
		time.Sleep(st)
	}
}

/* poll polls for new data from the server.  It returns true if there was
data received. */
func (c *client) poll(qn uint64, b *bytes.Buffer) ([]byte, error) {
	/* Roll a query */
	b.Reset()
	b.WriteString(strconv.FormatUint(qn, 36))
	/* Ask for data. */
	d, err := c.query(b.String())
	if nil != err {
		return nil, fmt.Errorf("recv: %w", err)
	}
	return d, nil
}

// Client is the client side of DPC.  It proxies stdio via DNS queries for the
// given domain and reports the number of bytes transferred.
func Client(domain string, poll time.Duration) (fwd, rev uint64, err error) {
	/* Roll a client with a clean domain. */
	c := client{
		domain:  "." + strings.Trim(domain, "."),
		pollMax: poll,
	}

	/* Ask the server for a connection ID. */
	if err := c.handshake(); nil != err {
		return 0, 0, fmt.Errorf("handshake: %w", err)
	}

	/* Start proxying. */
	done := make(chan error, 2)
	go c.proxyForward(done)
	go c.proxyBack(done)

	/* Wait for one side to have an error. */
	err = <-done
	defer func() { <-done }() /* Don't leak. */
	fwd = atomic.LoadUint64(&c.fwd)
	rev = atomic.LoadUint64(&c.rev)

	return
}
