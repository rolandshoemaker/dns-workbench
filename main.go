package main

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/miekg/dns"
)

type rawZones struct {
	Zones map[string]map[string][]string `yaml:"zones"`
}

type zones struct {
	Zones map[string]map[uint16][]dns.RR
}

func constrcutZones(rz rawZones) (*zones, error) {
	z := zones{
		Zones: make(map[string]map[uint16][]dns.RR),
	}
	for host, records := range rz.Zones {
		host = dns.Fqdn(host)
		z.Zones[host] = make(map[uint16][]dns.RR)
		for typeStr, v := range records {
			rType, present := dns.StringToType[strings.ToUpper(typeStr)]
			if !present {
				return nil, fmt.Errorf("Invalid record type")
			}
			for _, presentation := range v {
				rr, err := dns.NewRR(fmt.Sprintf("%s %s %s", host, strings.ToUpper(typeStr), presentation))
				if err != nil {
					return nil, fmt.Errorf("Couldn't parse record: %v", err)
				}
				z.Zones[host][rType] = append(z.Zones[host][rType], rr)
			}
		}
	}
	return &z, nil
}

type workbench struct {
	z *zones
}

func (wb *workbench) updateZones(nz zones) error {
	return nil
}

func (wb *workbench) dnsHandler(w dns.ResponseWriter, r *dns.Msg) {
	defer w.Close()
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false
	// Always add generated authority section?
	for _, q := range r.Question {
		fmt.Printf("dns-srv: Query -- [%s] %s\n", q.Name, dns.TypeToString[q.Qtype])
		allRecords, present := wb.z.Zones[q.Name]
		if !present {
			// NXDOMAIN
			continue
		}
		qRecords, present := allRecords[q.Qtype]
		if !present {
			continue
		}
		m.Answer = append(m.Answer, qRecords...)
	}
	w.WriteMsg(m)
	return
}

func (wb *workbench) serveWorkbench() {
	dns.HandleFunc(".", wb.dnsHandler)
	server := &dns.Server{
		Addr:         "127.0.0.1:8053",
		Net:          "udp",
		ReadTimeout:  time.Millisecond,
		WriteTimeout: time.Millisecond,
	}
	err := server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
		return
	}
}

var t = `zones:
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
      - 10 mail.other.com`

func main() {
	var rz rawZones
	err := yaml.Unmarshal([]byte(t), &rz)
	fmt.Printf("%v\n %+v\n", err, rz)

	z, err := constrcutZones(rz)
	fmt.Printf("%v\n %+v\n", err, z)

	wb := workbench{z: z}
	wb.serveWorkbench()

	// a := `bracewel.net. CAA 0 issue "letsencrypt.org"`
	// rr, err := dns.NewRR(a)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// fmt.Printf("%+v\n", rr)
}
