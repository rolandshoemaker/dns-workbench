# DNS workbench

A simple DNS workbench server written in Go, allows for quick loading and reloading
of zone definitions, it's somewhat useful for DNS testing.

## Zone file

Uses YAML which is loaded at runtime and can be gracefully reloaded via a HTTP api.

```
zones:
  bracewel.net:
    a:
      - 1.1.1.1
      - 2.2.2.2
    aaaa:
      - FF01:0:0:0:0:0:0:FB
    caa:
      - 0 issue "letsencrypt.org"

  other.com:
    mx:
      - 10 mail.other.com
```
