DNSProxyCommand
===============
Small single-binary Client/Server for tunneling OpenSSH over DNS using
OpenSSH's ProxyCommand.

For legal use only.

Features
--------
- Tunnels SSH over DNS using OpenSSH's `-o ProxyCommand=`
- Can tunnel anything else which communicates with its stdio
- Single binary for client and server
- Comms so slow you'll finally have time for a cup of tea
- No worries about stealth or detection evasion (there's none of either)

Quickstart
----------
1. Register a domain, point its NS records wherever DNSProxyCommand will be running
2. Build the binary, setting the domain from step 1
```sh
go install -ldflags '-X main.Domain=example.com' github.com/magisterquis/dnsproxycommand@latest
```
3. Start the DNS server and point it at an SSH server
```sh
dnsproxycommand 127.0.0.1:22
```
4. Tell SSH to connect via DNS
```sh
ssh -o ProxyCommand=dnsproxycommand c2
```

Warnings
--------
- There is no protection against any of the following
  - Injection into the stream of proxied data
  - Spoofed queries
  - Spoofed replies
  - Reverse engineering
- The [handshake](#handshake) requieres that the client and server's clocks be
  within a day or so of each other
- It's really slow

From a security perspective, confidentiality and integrity are the
responsibility of the SSH connection.

Configuration
-------------
Most configuration takes place via commandline options.  Please run with `-h`
for more info.  Currently, the options are
```
Options:
  -domain name
    	DNS domain name (default "sshprox.ga")
  -listen address
    	DNS server listen address (default "0.0.0.0:53")
  -poll-max interval
    	Maximum client poll interval (plus jitter) (default 3s)
  -prune-interval interval
    	Dead connection prune interval (default 1m0s)
```

The default domain name may be set at compile-time by setting `main.Domain`,
e.g. `-ldflags '-X main.Domain=example.com'`

Protocol
--------
The protocol used by DNSProxyCommand takes the form of queries for TXT records
and TXT record responses with base64'd data.  In the example queries/responses
below, the parent domain has been elided.

### Handshake
The client requests the server make a new connection to the DNS server by
sending a timestamp.  If successful, the reply is the ID the server wants the
client to use.

```
Request: 1648240942101289254             # Really, a Unix timestamp
Reply:   Y2l0OG16YXMxajU1 (cit8mzas1j55) # Another Unix timestamp
```

### Client->Server
Data is sent from the client to the server in a query containing a counter, the
payload, base32-encoded, and the ID from the handshake.  The reply will be an
empty TXT record if all worked well, or an NXDomain if something went wrong.

```
0.KNJUQLJSFYYC2T3QMVXFGU2IL44C4OANBI.cit8mzas1j55
```

### Server->Client
The client polls at increasing intervals for data from the server.  Queries
have a counter and the ID from the handshake.  Replies are possibly-empty
TXT records, or an NXDomain if something went wrong.
```
Request: 0.cit8mzas1j55
Reply:   U1NILTIuMC1PcGVuU1NIXzguMnAxIFVidW50dS00dWJ1bnR1MC40DQoAAAQcChRghyPRY
```

Non-OpenSSH
===========
It's possible to tunnel protocols besides SSH, though whatever's being tunneled
should have its own encryption and integrity-checking built-in (e.g. TLS).

Something like the below would proxy to [Libera](https://libera.chat):
```sh
./dnsproxycommand -domain example.com irc.libera.chat:6667 # Server, bad idea
./dnsproxycommand -domain example.com                      # Client, also a bad idea
```
