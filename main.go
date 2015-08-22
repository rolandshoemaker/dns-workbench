package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/codegangsta/cli"
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
		// Always add generated SOA rr?
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
	z           *zones
	name        string
	bind        string
	port        string
	net         string
	rTimeout    time.Duration
	wTimeout    time.Duration
	iTimeout    time.Duration
	compression bool
}

func (wb *workbench) updateZones(nz zones) error {
	return nil
}

func (wb *workbench) dnsHandler(w dns.ResponseWriter, r *dns.Msg) {
	defer w.Close()
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = wb.compression
	for _, q := range r.Question {
		fmt.Printf("[dns-wb] Recieved query for %s [%s]\n", q.Name, dns.TypeToString[q.Qtype])
		allRecords, present := wb.z.Zones[q.Name]
		if !present {
			// NXDOMAIN
			continue
		}
		m.Authoritative = true
		authRR, err := dns.NewRR(fmt.Sprintf("%s NS %s", q.Name, wb.name))
		if err != nil {
			fmt.Printf("Failed to create authority NS record: %v\n", err)
			continue
		}
		m.Ns = append(m.Ns, authRR)
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
		Addr:         net.JoinHostPort(wb.bind, wb.port),
		Net:          wb.net,
		ReadTimeout:  wb.rTimeout,
		WriteTimeout: wb.wTimeout,
		IdleTimeout:  func() time.Duration { return wb.iTimeout },
		NotifyStartedFunc: func() {
			fmt.Printf("[dns-wb] Started listening on %s:%s, serving %d zones\n", wb.bind, wb.port, len(wb.z.Zones))
		},
	}
	err := server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
		return
	}
}

func loadYAML(filename string) (rawZones, error) {
	rz := rawZones{}
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return rz, nil
	}
	err = yaml.Unmarshal(content, &rz)
	return rz, err
}

func main() {
	app := cli.NewApp()
	app.Name = "dns-workbench"
	app.Usage = "a simple authoritative DNS workbench server"
	app.Authors = []cli.Author{cli.Author{Name: "Roland Shoemaker", Email: "rolandshoemaker@gmail.com"}}
	app.Version = "0.0.1"

	app.Flags = []cli.Flag{}
	app.Commands = []cli.Command{
		{
			Name:  "run",
			Usage: "Starts the DNS server",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "dns-name",
					Value: "localhost",
					Usage: "Hostname of the DNS server",
				},
				cli.StringFlag{
					Name:  "dns-address",
					Value: "127.0.0.1",
					Usage: "Address for the DNS server to listen on",
				},
				cli.StringFlag{
					Name:  "dns-port",
					Value: "8053",
					Usage: "Port for the DNS server to listen on",
				},
				cli.StringFlag{
					Name:  "dns-network",
					Value: "udp",
					Usage: "Network for the DNS server to listen on",
				},
				cli.BoolFlag{
					Name:  "dns-compression",
					Usage: "Use DNS message compression",
				},
				cli.StringFlag{
					Name:  "zone-file",
					Usage: "Path to workbench zones file",
				},
				cli.StringFlag{
					Name:  "api-uri",
					Value: "127.0.0.1:5353",
					Usage: "Address for the HTTP API to listen on",
				},
				cli.BoolTFlag{
					Name:  "disable-api",
					Usage: "Don't start the HTTP API",
				},
			},
			Action: func(c *cli.Context) {
				var rz rawZones
				var z *zones
				var err error
				rzFile := c.String("zone-file")
				if rzFile != "" {
					rz, err = loadYAML("example.yml")
					if err != nil {
						fmt.Println(err)
						return
					}
					z, err = constrcutZones(rz)
					if err != nil {
						fmt.Println(err)
						return
					}
				} else {
					z = &zones{Zones: make(map[string]map[uint16][]dns.RR)}
				}

				wb := workbench{
					z:           z,
					name:        dns.Fqdn(c.String("dns-name")),
					bind:        c.String("dns-address"),
					port:        c.String("dns-port"),
					net:         c.String("dns-network"),
					rTimeout:    time.Second * 2,
					wTimeout:    time.Second * 2,
					iTimeout:    time.Second * 8,
					compression: c.Bool("dns-compression"),
				}
				wb.serveWorkbench()
			},
		},
		{
			Name:  "reload",
			Usage: "Loads a new zone file into a running workbench",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "zone-file",
					Usage: "Path to workbench zones file",
				},
				cli.StringFlag{
					Name:  "api-uri",
					Value: "127.0.0.1:5353",
					Usage: "Address for the HTTP API",
				},
			},
			Action: func(c *cli.Context) {
				// do something
				fmt.Println(c.String("zone-file"))
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}
