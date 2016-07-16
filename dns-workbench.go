package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rolandshoemaker/dns-workbench/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/rolandshoemaker/dns-workbench/Godeps/_workspace/src/github.com/miekg/dns"
	"github.com/rolandshoemaker/dns-workbench/Godeps/_workspace/src/gopkg.in/yaml.v2"
)

type rawZones struct {
	// so horribly gross but w/e for now
	Zones map[string]map[string]map[string][]string `yaml:"zones"`
}

type zones map[string]map[uint16][]dns.RR
type auth map[string]*dns.RR

func constructZones(rz rawZones, serverName string) (zones, auth, error) {
	a := make(auth)
	z := make(zones)
	for zoneName, hosts := range rz.Zones {
		zoneName = dns.Fqdn(zoneName)
		soa, err := dns.NewRR(fmt.Sprintf("%s SOA %s dns.%s  %s 10000 2400 604800 3600", zoneName, serverName, serverName, time.Now().Format("0601021504")))
		if err != nil {
			return nil, nil, err
		}
		z[zoneName] = map[uint16][]dns.RR{
			dns.TypeSOA: []dns.RR{soa},
		}

		authRR, err := dns.NewRR(fmt.Sprintf("%s NS %s", zoneName, serverName))
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to create authority NS record: %v", err)
		}

		for host, records := range hosts {
			host = dns.Fqdn(host)
			if _, present := z[host]; !present {
				z[host] = make(map[uint16][]dns.RR)
			}

			for typeStr, v := range records {
				rType, present := dns.StringToType[strings.ToUpper(typeStr)]
				if !present {
					return nil, nil, fmt.Errorf("Invalid record type")
				}

				for _, presentation := range v {
					rr, err := dns.NewRR(fmt.Sprintf("%s %s %s", host, strings.ToUpper(typeStr), presentation))
					if err != nil {
						return nil, nil, fmt.Errorf("Couldn't parse record: %v", err)
					}
					z[host][rType] = append(z[host][rType], rr)
					a[host] = &authRR
				}
			}
		}
	}
	return z, a, nil
}

type workbench struct {
	mu sync.RWMutex
	z  zones
	a  auth

	l *log.Logger

	name        string
	bind        string
	port        string
	net         string
	rTimeout    time.Duration
	wTimeout    time.Duration
	iTimeout    time.Duration
	compression bool
}

func (wb *workbench) reloadZones(nz zones, na auth) error {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	wb.z = nz
	wb.a = na
	return nil
}

func (wb *workbench) dnsHandler(w dns.ResponseWriter, r *dns.Msg) {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	defer w.Close()
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = wb.compression

	if len(r.Question) > 1 || r.Rcode != dns.OpcodeQuery {
		m.Rcode = dns.RcodeNotImplemented
	} else if len(r.Question) == 0 {
		m.Rcode = dns.RcodeFormatError
	}
	if m.Rcode == dns.RcodeFormatError || m.Rcode == dns.RcodeNotImplemented {
		w.WriteMsg(m)
		return
	}

	q := &r.Question[0]

	wb.l.Printf("Received query for [%s] %s\n", dns.TypeToString[q.Qtype], q.Name)
	allRecords, present := wb.z[q.Name]
	if !present {
		m.Rcode = dns.RcodeNameError
		w.WriteMsg(m)
		return
	}

	if auth, present := wb.a[q.Name]; present {
		m.Authoritative = true
		m.Ns = append(m.Ns, *auth)
	}

	qRecords, present := allRecords[q.Qtype]
	if !present {
		m.Rcode = dns.RcodeNXRrset
		w.WriteMsg(m)
		return
	}

	m.Answer = append(m.Answer, qRecords...)
	w.WriteMsg(m)
	return
}

func (wb *workbench) serveWorkbench() error {
	dns.HandleFunc(".", wb.dnsHandler)
	server := &dns.Server{
		Addr:         net.JoinHostPort(wb.bind, wb.port),
		Net:          wb.net,
		ReadTimeout:  wb.rTimeout,
		WriteTimeout: wb.wTimeout,
		IdleTimeout:  func() time.Duration { return wb.iTimeout },
		NotifyStartedFunc: func() {
			wb.mu.RLock()
			defer wb.mu.RUnlock()
			wb.l.Printf("DNS listening on %s:%s, serving %d zones\n", wb.bind, wb.port, len(wb.z))
		},
	}
	err := server.ListenAndServe()
	return err
}

func sendError(err string, w http.ResponseWriter) {
	w.WriteHeader(400)
	w.Write([]byte(err))
}

func (wb *workbench) apiReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		sendError("Method not supported", w)
		return
	}

	rStart := time.Now()
	var rz rawZones
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		sendError(err.Error(), w)
		return
	}
	err = json.Unmarshal(body, &rz)
	if err != nil {
		sendError(err.Error(), w)
		return
	}

	z, a, err := constructZones(rz, wb.name)
	if err != nil {
		sendError(err.Error(), w)
		return
	}

	wb.reloadZones(z, a)

	wb.mu.RLock()
	defer wb.mu.RUnlock()
	wb.l.Printf("Reloaded zone definitions, now serving %d zones, took %s\n", len(wb.z), time.Since(rStart))
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
				var z zones
				var a auth
				var err error

				logger := log.New(os.Stdout, "[dns-wb] ", log.Flags())

				if rzFile := c.String("zone-file"); rzFile != "" {
					rz, err = loadYAML(rzFile)
					if err != nil {
						logger.Fatalf("Failed to read zone file: %s\n", err)
					}
					z, a, err = constructZones(rz, dns.Fqdn(c.String("dns-name")))
					if err != nil {
						logger.Fatalf("Failed to parse zone file: %s\n", err)
					}
				} else {
					z = make(zones)
					a = make(auth)
				}

				wb := workbench{
					z:           z,
					a:           a,
					l:           logger,
					name:        dns.Fqdn(c.String("dns-name")),
					bind:        c.String("dns-address"),
					port:        c.String("dns-port"),
					net:         c.String("dns-network"),
					rTimeout:    time.Second * 2,
					wTimeout:    time.Second * 2,
					iTimeout:    time.Second * 8,
					compression: c.Bool("dns-compression"),
				}
				go func() {
					http.HandleFunc("/api/reload", wb.apiReload)
					logger.Printf("API listening on %s\n", c.String("api-uri"))
					err := http.ListenAndServe(c.String("api-uri"), nil)
					if err != nil {
						logger.Printf("HTTP API crashed: %s\n", err)
						// Dont' exit
					}
				}()
				err = wb.serveWorkbench()
				if err != nil {
					logger.Fatalf("DNS server crashed: %s\n", err)
					os.Exit(1)
				}
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
				logger := log.New(os.Stdout, "[dns-wb] ", log.Flags())

				if c.String("zone-file") == "" {
					logger.Fatalf("Zone file option is required\n")
				}
				rz, err := loadYAML(c.String("zone-file"))
				if err != nil {
					logger.Fatalf("Failed to load zone file: %s\n", err)
				}
				rzJSON, err := json.Marshal(rz)
				if err != nil {
					logger.Fatalf("Failed to marshal zone file to JSON: %s\n", err)
				}
				req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/api/reload", c.String("api-uri")), bytes.NewBuffer(rzJSON))
				req.Header.Set("Content-Type", "application/json")
				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					logger.Fatalf("Failed to send update: %s\n", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != 200 {
					apiErr, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						logger.Fatalf("Failed to read response body: %s\n", err)
						return
					}
					logger.Fatalf("Failed to reload zones: %s\n", apiErr)
					return
				}
				logger.Printf("Successfully reloaded zones\n")
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}
