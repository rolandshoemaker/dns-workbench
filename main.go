package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/codegangsta/cli"
	"github.com/miekg/dns"
	"gopkg.in/yaml.v2"
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
		// Always add a generated SOA rr?
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
	zMu         sync.RWMutex
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

func (wb *workbench) reloadZones(nz *zones) error {
	wb.zMu.Lock()
	defer wb.zMu.Unlock()
	wb.z = nz
	fmt.Printf("[dns-wb] Reloaded zone definitions, now serving %d zones\n", len(wb.z.Zones))
	return nil
}

func (wb *workbench) dnsHandler(w dns.ResponseWriter, r *dns.Msg) {
	defer w.Close()
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = wb.compression
	for _, q := range r.Question {
		fmt.Printf("[dns-wb] Recieved query for %s [%s]\n", q.Name, dns.TypeToString[q.Qtype])
		wb.zMu.RLock()
		allRecords, present := wb.z.Zones[q.Name]
		wb.zMu.RUnlock()
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

func (wb *workbench) serveWorkbench() error {
	dns.HandleFunc(".", wb.dnsHandler)
	server := &dns.Server{
		Addr:         net.JoinHostPort(wb.bind, wb.port),
		Net:          wb.net,
		ReadTimeout:  wb.rTimeout,
		WriteTimeout: wb.wTimeout,
		IdleTimeout:  func() time.Duration { return wb.iTimeout },
		NotifyStartedFunc: func() {
			wb.zMu.RLock()
			fmt.Printf("[dns-wb] DNS listening on %s:%s, serving %d zones\n", wb.bind, wb.port, len(wb.z.Zones))
			wb.zMu.RUnlock()
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

	z, err := constrcutZones(rz)
	if err != nil {
		sendError(err.Error(), w)
		return
	}

	wb.reloadZones(z)
	return
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
				if rzFile := c.String("zone-file"); rzFile != "" {
					rz, err = loadYAML(rzFile)
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
				go func() {
					http.HandleFunc("/api/reload", wb.apiReload)
					fmt.Printf("[dns-wb] API listening on %s\n", c.String("api-uri"))
					err := http.ListenAndServe(c.String("api-uri"), nil)
					if err != nil {
						fmt.Println(err)
					}
				}()
				err = wb.serveWorkbench()
				if err != nil {
					fmt.Println(err)
					return
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
				// do something
				if c.String("zone-file") == "" {
					fmt.Println("I neeeeed this")
					return
				}
				rz, err := loadYAML(c.String("zone-file"))
				if err != nil {
					fmt.Println(err)
					return
				}
				rzJSON, err := json.Marshal(rz)
				if err != nil {
					fmt.Println(err)
					return
				}
				fmt.Println(string(rzJSON))
				// Send json to HTTP API and wait for success/errors
				req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/api/reload", c.String("api-uri")), bytes.NewBuffer(rzJSON))
				req.Header.Set("Content-Type", "application/json")
				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					fmt.Println(err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != 200 {
					apiErr, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						fmt.Println(err)
						return
					}
					fmt.Println(apiErr)
				}
				fmt.Println("Succesesfully reloaded zones")
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}
