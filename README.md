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
*-- This idea probably won't entire work --*

These zones are parsed by using `miekg/dns` to convert RR name -> record type ->
record struct. `reflect` is then used to inspect each of the fields on the respective
struct and properly convert `string` -> the right type. If there aren't enough
strings in the slice to fill all the fields (or and empty string is provided)
the field will be left empty. If a string cannot be marshaled to the proper type
the workbench will fail to load the entire zone file. The zone file will throw
warnings if too many fields were passed

*-- This on the other handle will but will be more annoying to write --*

...or, It could just convert from name -> record type -> converter func. This would
allow much finer control over records which would allow much better spec compliance
(e.g. with `TXT` records and such). The converter functions would know how many
fields are required, the type specific parsing needed, etcetcetc which would make
life a whole bunch easier in terms of weird edge cases.
