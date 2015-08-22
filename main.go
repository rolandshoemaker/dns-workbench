package main

import (
	"fmt"
	"time"

	"github.com/miekg/dns"
	"gopkg.in/yaml.v2"
)

type rawZones struct {
	Zones map[string]map[string][]string `yaml:"zones,flow"`
}

type zones struct {
	Zones map[string]map[uint16][]dns.RR
}

func constrcutZones(rz rawZones) *zones {
	return nil
}

type workbench struct {
	z zones
}

func (wb *workbench) updateZones(nz zones) error {
	return nil
}

func (wb *workbench) dnsHandler(w dns.ResponseWriter, r *dns.Msg) {
	defer w.Close()
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	for _, q := range r.Question {
		fmt.Printf("dns-srv: Query -- [%s] %s\n", q.Name, dns.TypeToString[q.Qtype])
		allRecords, present := wb.z.Zones[q.Name]
		if !present {
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
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			fmt.Println(err)
			return
		}
	}()
}

var t = `zones:
  bracewel.net:
    a:
      - 1.1.1.1
    aaaa:
      - FF01:0:0:0:0:0:0:FB
    caa:
      - 0
      - issue
      - letsencrypt.org

  other.com:
    mx:
      - mail.other.com`

func main() {
	var z rawZones
	err := yaml.Unmarshal([]byte(t), &z)
	fmt.Printf("%v %+v\n", err, z)
}
