// Package server implements the DPC server logic.
package server

/*
 * server.go
 * Server side of dnsproxycommand
 * By J. Stuart McMurray
 * Created 20220323
 * Last Modified 20220324
 */

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/net/dns/dnsmessage"
)

const (
	/* bufLen is the length of the buffers in bufPool. */
	bufLen = 2048

	/* ansCacheSize is the size of the answer cache. */
	ansCacheSize = 1024 * 1024
)

var (
	/* bufPool is a pool of buffers of size bufLen. */
	bufPool = sync.Pool{New: func() any { return make([]byte, bufLen) }}

	/* domainName is the domain we'll serve. */
	domainName string

	/* upstreamAddr is the upstream address to which to connect. */
	upstreamAddr string

	/* ansCache holds cached answers to queries. */
	ansCache *lru.TwoQueueCache
)

func init() {
	var err error
	ansCache, err = lru.New2Q(ansCacheSize)
	if nil != err {
		panic(fmt.Sprintf("making answer cache: %s", err))
	}
}

// Server listens on laddr for DNS queries for the parent domain domain and
// proxies connections from clients to TCP connections to caddr.
func Server(caddr, laddr, domain string, pruneInterval time.Duration) error {
	/* Grab a socket. */
	pc, err := net.ListenPacket("udp", laddr)
	if nil != err {
		return fmt.Errorf("listening on %q: %w", laddr, err)
	}
	log.Printf("Listening on %s", pc.LocalAddr())

	/* Work out the domain we'll serve. */
	domainName = "." + strings.Trim(domain, ".") + "."

	/* Start pruning dead conns. */
	go pruneConns(pruneInterval)

	/* Yeah, package-global :( */
	upstreamAddr = caddr

	/* Pop packets, handle. */
	for {
		/* Pop a packet. */
		b := bufPool.Get().([]byte)
		n, addr, err := pc.ReadFrom(b)
		if nil != err {
			return fmt.Errorf("DNS read: %w", err)
		}
		go func() {
			handlePacket(pc, addr, b[:n])
			bufPool.Put(b)
		}()
	}
}

/* handlePacket handles a packet off the wire. */
func handlePacket(pc net.PacketConn, addr net.Addr, b []byte) {
	/* Unpack the packet. */
	var msg dnsmessage.Message
	if err := msg.Unpack(b); nil != err {
		log.Printf("[%s] Unpacking packet: %s", addr, err)
		return
	}

	/* We should get exactly one TXT query. */
	if 1 != len(msg.Questions) {
		log.Printf(
			"[%s] Multiple (%d) questions",
			addr,
			len(msg.Questions),
		)
		return
	}
	q := msg.Questions[0]
	if dnsmessage.TypeTXT != q.Type {
		return
	}
	qn := msg.Questions[0].Name.String()

	/* Sanity-check other things. */
	if msg.Response {
		log.Printf("[%s] Response for %s", addr, q.Name)
		return
	}

	/* Only care about our domain. */
	if !strings.HasSuffix(qn, domainName) {
		return
	}
	qn = strings.ToLower(strings.TrimSuffix(qn, domainName))

	/* Need 1-3 labels */
	labels := strings.Split(qn, ".")
	switch len(labels) {
	case 1, 2, 3: /* Ok. */
	case 0:
		log.Printf("[%s] No labels from %s", qn, addr)
	default:
		log.Printf("[%s] Too many labels from %s", qn, addr)
		return
	}

	/* Reply to be sent back. */
	var reply *string

	/* Send back a response when we're done. */
	msg.Response = true
	msg.Authoritative = true
	defer func() {
		/* Roll a response. */
		if nil != reply {
			msg.RCode = dnsmessage.RCodeSuccess
			msg.Answers = append(msg.Answers, dnsmessage.Resource{
				Header: dnsmessage.ResourceHeader{
					Name:  q.Name,
					Type:  q.Type,
					Class: q.Class,
				},
				Body: &dnsmessage.TXTResource{TXT: []string{
					*reply,
				}},
			})
		} else {
			msg.RCode = dnsmessage.RCodeNameError
		}
		b, err := msg.Pack()
		if nil != err {
			log.Printf("[%s] Packing response: %s", qn, err)
			return
		}
		/* Send it back. */
		if _, err := pc.WriteTo(b, addr); nil != err {
			log.Printf(
				"[%s] Sending response to %s: %s",
				qn,
				addr,
				err,
			)
		}
	}()

	/* If we've got a cached answer, use that. */
	if 2 == len(labels) {
		if ca, ok := ansCache.Get(qn); ok {
			reply = ca.(*string)
			return
		}
	}

	/* First label tells us what to do. */
	var (
		rb  []byte
		err error
	)
	switch len(labels) {
	case 1: /* New connection: timestamp. */
		rb, err = handleNewConn(labels[0])
	case 2: /* Proxy reverse: counter.id. */
		rb, err = handleReverse(labels[0], labels[1])
	case 3: /* Proxy forward: counter.data.id. */
		rb, err = handleForward(labels[0], labels[1], labels[2])
	default:
		panic(fmt.Sprintf(
			"unpossible number of labels in %s: %d",
			qn,
			len(labels),
		))
	}
	if nil != err {
		log.Printf("[%s] Error from %s: %s", qn, addr, err)
		return
	}
	if nil == rb {
		return
	}

	/* Got an answer, prepare to send it back and cache it. */
	r := base64.RawStdEncoding.EncodeToString(rb)
	reply = &r
	if 2 == len(labels) {
		ansCache.Add(qn, &r)
	}
}
