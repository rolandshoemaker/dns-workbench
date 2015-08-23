# `dns-workbench`

A simple authoritative DNS workbench server written in Go that allows for quick loading
and reloading of zone definitions. Because it can be setup and reconfigured extremely
easily it's somewhat useful for testing and other random things.

## Zone file

Zone definition files use YAML and can be loaded at runtime or via the HTTP API.

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

## Reloading zones

The DNS server can reload all of the zones it is currently serving gracefully
via a HTTP API. To change zones you need to `POST` a JSON version of a zone file
to `/api/reload`. If the zone file cannot be parsed the an error string will be
returned and the server will keep serving the previous zones.

This can be done manually or by using the `dns-workbench` binary like so

```
$ dns-workbench reload --zone-file new-example.yml
```

Loading a new zone file will remove any previous definitions, so

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
