// Program DNSProxyCommand is a ProxyCommand for OpenSSH which uses DNS as its
// transport.
package main

/*
 * dnsproxycommand.go
 * OpenSSH ProxyCommand using SSH
 * By J. Stuart McMurray
 * Created 20220323
 * Last Modified 20220324
 */

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/magisterquis/dnsproxycommand/internal/client"
	"github.com/magisterquis/dnsproxycommand/internal/server"
)

var (
	// Domain is the base DNS domain.
	Domain = ""
)

func main() {
	start := time.Now()
	flag.StringVar(
		&Domain,
		"domain",
		Domain,
		"DNS domain `name`",
	)
	var (
		laddr = flag.String(
			"listen",
			"0.0.0.0:53",
			"DNS server listen `address`",
		)
		pollMax = flag.Duration(
			"poll-max",
			5*time.Second,
			"Maximum client poll `interval` (less jitter)",
		)
		pruneInterval = flag.Duration(
			"prune-interval",
			time.Minute,
			"Dead connection prune `interval`",
		)
	)
	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			`Usage: %s [options] [serveraddr]

With no serveraddr, proxies stdio via DNS.

With a serveraddr, listens for the above and proxies to the serveraddr.

Options:
`,
			os.Args[0],
		)
		flag.PrintDefaults()
	}
	flag.Parse()

	/* Seed the PRNG. */
	rand.Seed(time.Now().UnixNano())

	/* Work out whether we're a client or server. */
	switch flag.NArg() {
	case 0: /* Client */
		fwd, rev, err := client.Client(Domain, *pollMax)
		log.Printf(
			"Finished proxying after %s: %d bytes forward, %d bytes reverse, %d total",
			time.Since(start).Round(time.Millisecond),
			fwd,
			rev,
			fwd+rev,
		)
		if nil != err {
			log.Fatalf("Fatal error: %s", err)
		}
	case 1: /* Server */
		log.Fatalf(
			"Fatal error: %s",
			server.Server(
				flag.Arg(0),
				*laddr,
				Domain,
				*pruneInterval,
			),
		)
	default: /* Error. */
		log.Fatalf("Too many command-line arguments")
	}
}
