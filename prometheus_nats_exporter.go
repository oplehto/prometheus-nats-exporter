// Copyright 2017-2018 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"time"

	"github.com/nats-io/prometheus-nats-exporter/collector"
	"github.com/nats-io/prometheus-nats-exporter/exporter"
)

// parseServerIDAndURL parses the url argument the optional id for the server ID.
func parseServerIDAndURL(urlArg string) (string, string, error) {
	var id string
	var monURL string

	// if there is an optional tag, parse it out and check the url
	if strings.Contains(urlArg, ",") {
		idx := strings.LastIndex(urlArg, ",")
		id = urlArg[:idx]
		monURL = urlArg[idx+1:]
		if _, err := url.ParseRequestURI(monURL); err != nil {
			return "", "", err
		}
	} else {
		// The URL is the basis for a default id with credentials stripped out.
		u, err := url.ParseRequestURI(urlArg)
		if err != nil {
			return "", "", err
		}
		id = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		monURL = urlArg
	}
	return id, monURL, nil
}

// updateOptions sets up additional options based on the provided flags.
func updateOptions(debugAndTrace, useSysLog bool, opts *exporter.NATSExporterOptions) {
	if debugAndTrace {
		opts.Debug = true
		opts.Trace = true
	}

	// default is console, then handle in order of precedence:
	// remote sys log, syslog, then file.  This simplifies error handling.
	if opts.LogFile != "" {
		opts.LogType = collector.FileLogType
	}
	if useSysLog {
		opts.LogType = collector.SysLogType
	}
	if opts.RemoteSyslog != "" {
		opts.LogType = collector.RemoteSysLogType
	}

	if !opts.GetConnz && !opts.GetVarz && !opts.GetSubz && !opts.GetRoutez {
		// Mo logger setup yet, so use fmt
		fmt.Printf("No metrics specified.  Defaulting to varz.\n")
		opts.GetVarz = true
	}
}

func main() {
	var useSysLog bool
	var debugAndTrace bool
	var retryInterval int

	opts := exporter.GetDefaultExporterOptions()

	// Parse flags
	flag.IntVar(&opts.ListenPort, "port", exporter.DefaultListenPort, "Port to listen on.")
	flag.IntVar(&opts.ListenPort, "p", exporter.DefaultListenPort, "Port to listen on.")
	flag.StringVar(&opts.ListenAddress, "addr", exporter.DefaultListenAddress, "Network host to listen on.")
	flag.StringVar(&opts.ListenAddress, "a", exporter.DefaultListenAddress, "Network host to listen on.")
	flag.IntVar(&retryInterval, "ri", exporter.DefaultRetryIntervalSecs, "Interval in seconds to retry NATS Server monitor URL.")
	flag.StringVar(&opts.LogFile, "l", "", "Log file name.")
	flag.StringVar(&opts.LogFile, "log", "", "Log file name.")
	flag.BoolVar(&useSysLog, "s", false, "Write log statements to the syslog.")
	flag.BoolVar(&useSysLog, "syslog", false, "Write log statements to the syslog.")
	flag.StringVar(&opts.RemoteSyslog, "r", "", "Remote syslog address to write log statements.")
	flag.StringVar(&opts.RemoteSyslog, "remote_syslog", "", "Write log statements to a remote syslog.")
	flag.BoolVar(&opts.Debug, "D", false, "Enable debug log level.")
	flag.BoolVar(&opts.Trace, "V", false, "Enable trace log level.")
	flag.BoolVar(&debugAndTrace, "DV", false, "Enable debug and trace log levels.")
	flag.BoolVar(&opts.GetConnz, "connz", false, "Get connection metrics.")
	flag.BoolVar(&opts.GetRoutez, "routez", false, "Get route metrics.")
	flag.BoolVar(&opts.GetSubz, "subz", false, "Get subscription metrics.")
	flag.BoolVar(&opts.GetVarz, "varz", false, "Get general metrics.")
	flag.StringVar(&opts.CertFile, "tlscert", "", "Server certificate file (Enables HTTPS).")
	flag.StringVar(&opts.KeyFile, "tlskey", "", "Private key for server certificate (used with HTTPS).")
	flag.StringVar(&opts.CaFile, "tlscacert", "", "Client certificate CA for verification (used with HTTPS).")
	flag.StringVar(&opts.HTTPUser, "http_user", "", "Enable basic auth and set user name for HTTP scrapes.")
	flag.StringVar(&opts.HTTPPassword, "http_pass", "", "Set the password for HTTP scrapes. NATS bcrypt supported.")
	flag.Parse()

	opts.RetryInterval = time.Duration(retryInterval) * time.Second

	args := flag.Args()
	if len(args) < 1 {
		fmt.Printf("Usage:  %s <flags> url\n\n", os.Args[0])
		flag.Usage()
		return
	} else if len(args) > 1 {
		fmt.Println(
			`WARNING:  While permitted by this exporter, monitoring more than one server 
violates Prometheus guidelines and best practices.  Each Prometheus NATS 
exporter should monitor exactly one NATS server, preferably sitting right 
beside it on the same machine.  Aggregate multiple servers only when 
necessary.`)
	}

	updateOptions(debugAndTrace, useSysLog, opts)

	// Create an instance of the NATS exporter.
	exp := exporter.NewExporter(opts)

	// For each URL specified, add the NATS server with the optional ID.
	for _, arg := range flag.Args() {
		id, url, err := parseServerIDAndURL(arg)
		if err != nil {
			collector.Fatalf("Unable to parse URL %q: %v", arg, err)
		}
		if err := exp.AddServer(id, url); err != nil {
			collector.Fatalf("Unable to setup server in exporter: %s, %s: %v", id, url, err)
		}
	}

	// Start the exporter.
	if err := exp.Start(); err != nil {
		collector.Fatalf("error starting the exporter: %v\n", err)
	}

	// Setup the interrupt handler to gracefully exit.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		exp.Stop()
		os.Exit(0)
	}()

	runtime.Goexit()
}
