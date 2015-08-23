# `dns-workbench`

A simple authoritative DNS workbench server written in Go that allows for quick loading
and reloading of zone definitions. Because it can be setup and reconfigured extremely
easily it's somewhat useful for testing and other random things. Plus it's so easy
easy to use, you don't need a degree from ISC!

## Zone file

Zone definition files use YAML and can be loaded at runtime or via the HTTP API.
The zone defintion format follows a somewhat similar path to the standard definition
files used by other DNS servers like BIND. Record values must be in the 'presentation'
format in order to be properly parsed. Generally just googling the RR type name
will give you enough information to construct pretty much whatever record you'd
like (restricted to the types that [`github.com/miekg/dns`](https://github.com/miekg/dns)
supports of course).

```
zones:
  bracewel.net:
    bracewel.net:
      a:
        - 1.1.1.1
        - 2.2.2.2
      aaaa:
        - FF01:0:0:0:0:0:0:FB
      caa:
        - 0 issue "letsencrypt.org"
      ns:
        - 1.1.1.2
        - 1.1.1.3
        - 1.1.1.4
        - 1.1.1.5
    www.bracewel.net:
      a:
        - 1.1.1.1
        - 2.2.2.2
      aaaa:
        - FF01:0:0:0:0:0:0:FB
      ns:
        - 1.1.1.2
        - 1.1.1.3
        - 1.1.1.4
        - 1.1.1.5
```

`SOA` and `NS` authority records are automatically generated for each zone. All
`SOA` parameters and `TTL`s on RRs are automatically set.

The above zone definition file would be equivalent to something along the lines
of the following BIND zone definition.

```
bracewel.net. 3600  IN  SOA localhost. dns.localhost. 1508222340 10000 2400 604800 3600

bracewel.net. 3600  IN  A     1.1.1.1
bracewel.net. 3600  IN  A     2.2.2.2
bracewel.net. 3600  IN  AAAA  ff01::fb
bracewel.net. 3600  IN  NS    1.1.1.2.
bracewel.net. 3600  IN  NS    1.1.1.3.
bracewel.net. 3600  IN  NS    1.1.1.4.
bracewel.net. 3600  IN  NS    1.1.1.5.
bracewel.net. 3600  IN  CAA   0 issue "letsencrypt.org"

www 3600  IN  A     1.1.1.1
www 3600  IN  A     2.2.2.2
www 3600  IN  AAAA  ff01::fb
www 3600  IN  NS    1.1.1.2.
www 3600  IN  NS    1.1.1.3.
www 3600  IN  NS    1.1.1.4.
www 3600  IN  NS    1.1.1.5.
```

## Reloading zones

The DNS server can reload all of the zones it is currently serving gracefully
via a HTTP API. To change zones you need to `POST` a JSON version of a zone file
to `/api/reload`. If the zone file cannot be parsed then an error string will be
returned and the server will keep serving the previous zones.

This can be done manually or by using the `dns-workbench` binary like so

```
$ dns-workbench reload --zone-file new-zones.yml
```

Loading a new zone file will replace any previous definitions.

## Building

Building is super simple, thanks Go!

```
$ go build dns-workbench.go
```

## Usage

```
$ dns-workbench
NAME:
   dns-workbench - a simple authoritative DNS workbench server

USAGE:
   dns-workbench [global options] command [command options] [arguments...]

VERSION:
   0.0.1

AUTHOR(S):
   Roland Shoemaker <rolandshoemaker@gmail.com>

COMMANDS:
   run      Starts the DNS server
   reload   Loads a new zone file into a running workbench
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help
   --version, -v  print the version
```
