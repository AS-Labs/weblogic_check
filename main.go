package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	//"strings"
	"time"

	"slices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shirou/gopsutil/process"
)

// ServerRuntime represents a WebLogic server runtime from the REST API
type ServerRuntime struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// ServerRuntimesResponse represents the JSON response from the serverRuntimes endpoint
type ServerRuntimesResponse struct {
	Body struct {
		Items []ServerRuntime `json:"items"`
	} `json:"body"`
}

// basicAuthTransport adds basic authentication to HTTP requests
type basicAuthTransport struct {
	username string
	password string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	auth := t.username + ":" + t.password
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	req.Header.Set("Authorization", "Basic "+encoded)
	return http.DefaultTransport.RoundTrip(req)
}

// ProcessCollector collects metrics for a specific WebLogic server
type ProcessCollector struct {
	adminURL   string
	username   string
	password   string
	serverName string
	client     *http.Client
	// Metric descriptors
	adminUpDesc      *prometheus.Desc
	serverStatusDesc *prometheus.Desc
	processArgDesc   *prometheus.Desc
}

// NewProcessCollector initializes a new ProcessCollector
func NewProcessCollector(adminURL, username, password, serverName string) *ProcessCollector {
	client := &http.Client{
		Transport: &basicAuthTransport{username: username, password: password},
		Timeout:   10 * time.Second,
	}
	return &ProcessCollector{
		adminURL:   adminURL,
		username:   username,
		password:   password,
		serverName: serverName,
		client:     client,
		adminUpDesc: prometheus.NewDesc(
			"weblogic_admin_server_up",
			"Whether the WebLogic admin server is up (1 = up, 0 = down)",
			nil,
			nil,
		),
		serverStatusDesc: prometheus.NewDesc(
			"weblogic_server_up",
			"Whether the specified WebLogic server is up (1 = RUNNING, 0 = otherwise)",
			[]string{"server_name"},
			nil,
		),
		processArgDesc: prometheus.NewDesc(
			"process_arg",
			"Command-line arguments of the WebLogic process",
			[]string{"server_name", "pid", "index", "value"},
			nil,
		),
	}
}

// Describe sends the metric descriptors to Prometheus
func (c *ProcessCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.adminUpDesc
	ch <- c.serverStatusDesc
	ch <- c.processArgDesc
}

// Collect gathers and sends the metrics to Prometheus
func (c *ProcessCollector) Collect(ch chan<- prometheus.Metric) {
	// Check admin server status and get server runtime
	resp, err := c.client.Get(c.adminURL + "/management/weblogic/latest/domainRuntime/serverRuntimes")
	if err != nil {
		log.Printf("Failed to connect to admin server: %v", err)
		ch <- prometheus.MustNewConstMetric(c.adminUpDesc, prometheus.GaugeValue, 0)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Admin server returned status: %s", resp.Status)
		ch <- prometheus.MustNewConstMetric(c.adminUpDesc, prometheus.GaugeValue, 0)
		return
	}

	// Admin server is up
	ch <- prometheus.MustNewConstMetric(c.adminUpDesc, prometheus.GaugeValue, 1)

	// Parse server runtimes to find the specified server
	var runtimes ServerRuntimesResponse
	if err := json.NewDecoder(resp.Body).Decode(&runtimes); err != nil {
		log.Printf("Failed to parse server runtimes: %v", err)
		return
	}

	// Check status of the specified server
	serverFound := false
	for _, server := range runtimes.Body.Items {
		if server.Name == c.serverName {
			serverFound = true
			value := 0.0
			if server.State == "RUNNING" {
				value = 1.0
			}
			ch <- prometheus.MustNewConstMetric(
				c.serverStatusDesc,
				prometheus.GaugeValue,
				value,
				server.Name,
			)
			break
		}
	}

	if !serverFound {
		log.Printf("Server %s not found in domain", c.serverName)
	}

	// Collect process arguments for the specified server
	processes, err := process.Processes()
	if err != nil {
		log.Printf("Failed to retrieve processes: %v", err)
		return
	}

	for _, p := range processes {
		cmdline, err := p.CmdlineSlice()
		if err != nil {
			continue
		}
		if slices.Contains(cmdline, "-Dweblogic.Name="+c.serverName) {
			pid := fmt.Sprintf("%d", p.Pid)
			for i, arg := range cmdline {
				ch <- prometheus.MustNewConstMetric(
					c.processArgDesc,
					prometheus.GaugeValue,
					1,
					c.serverName, pid, fmt.Sprintf("%d", i), arg,
				)
			}
		}
	}
}

func main() {
	// Parse command-line arguments
	var (
		adminURL   = flag.String("admin-url", "", "URL of the WebLogic admin server (e.g., http://localhost:7001)")
		username   = flag.String("username", "", "Username for WebLogic admin server")
		password   = flag.String("password", "", "Password for WebLogic admin server")
		serverName = flag.String("server-name", "", "Name of the WebLogic server to monitor (e.g., AdminServer)")
		port       = flag.Int("port", 9255, "Port for the exporter")
	)
	flag.Parse()

	// Validate required flags
	if *adminURL == "" || *username == "" || *password == "" || *serverName == "" {
		fmt.Println("Usage: ./exporter -admin-url <URL> -username <user> -password <pass> -server-name <name> [-port <port>]")
		os.Exit(1)
	}

	// Register the collector
	collector := NewProcessCollector(*adminURL, *username, *password, *serverName)
	prometheus.MustRegister(collector)

	// Start the HTTP server
	http.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting exporter on %s/metrics for server %s", addr, *serverName)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
