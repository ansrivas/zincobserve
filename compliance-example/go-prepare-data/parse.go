// Copyright 2014 Prometheus Team
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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/prom2json"
)

// var usage = fmt.Sprintf(`Usage: %s [METRICS_PATH | METRICS_URL [--cert CERT_PATH --key KEY_PATH | --accept-invalid-cert]]

// Example:

// 	$ prom2json http://my-prometheus-server:9000/metrics

// 	$ curl http://my-prometheus-server:9000/metrics | prom2json

// `, os.Args[0])

var fileMap = map[string]string{
	"http://demo.promlabs.com:10000/metrics": "file1.json",
	"http://demo.promlabs.com:10001/metrics": "file2.json",
	"http://demo.promlabs.com:10002/metrics": "file3.json",
}

func writeToFile(payload []byte, fileName string) {
	// f, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	if _, err = f.Write(payload); err != nil {
		panic(err)
	}
	f.Write([]byte("\n"))

}

func CreatePayload(serverUrl string) []byte {
	// cert := flag.String("cert", "", "client certificate file")
	// key := flag.String("key", "", "client certificate's key file")
	// skipServerCertCheck := flag.Bool("accept-invalid-cert", false, "Accept any certificate during TLS handshake. Insecure, use only for testing.")
	// flag.Usage = func() {
	// 	fmt.Fprint(os.Stderr, usage)
	// }
	// flag.Parse()

	var input io.Reader
	var err error
	skipServerCertCheck := false
	cert := ""
	key := ""
	// arg := flag.Arg(0)
	// flag.NArg()

	// if flag.NArg() > 1 {
	// 	fmt.Fprintf(os.Stderr, "Too many arguments.\n%s", usage)
	// 	os.Exit(2)
	// }

	if url, urlErr := url.Parse(serverUrl); urlErr != nil || url.Scheme == "" {
		// `url, err := url.Parse("/some/path.txt")` results in: `err == nil && url.Scheme == ""`
		// Open file since serverUrl appears not to be a valid URL (parsing error occurred or the scheme is missing).
		if input, err = os.Open(serverUrl); err != nil {
			fmt.Fprintln(os.Stderr, "error opening file:", err)
			os.Exit(1)
		}
	}

	mfChan := make(chan *dto.MetricFamily, 1024)
	// Missing input means we are reading from an URL.
	if input != nil {
		go func() {
			if err := prom2json.ParseReader(input, mfChan); err != nil {
				fmt.Fprintln(os.Stderr, "error reading metrics:", err)
				os.Exit(1)
			}
		}()
	} else {
		transport, err := makeTransport(cert, key, skipServerCertCheck)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		go func() {
			err := prom2json.FetchMetricFamilies(serverUrl, mfChan, transport)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}()
	}

	result := []*prom2json.Family{}
	for mf := range mfChan {
		result = append(result, prom2json.NewFamily(mf))
	}
	jsonText, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error marshaling JSON:", err)
		os.Exit(1)
	}

	fileName := fileMap[serverUrl]
	writeToFile(jsonText, fileName)
	return jsonText
	// if _, err := os.Stdout.Write(jsonText); err != nil {
	// 	fmt.Fprintln(os.Stderr, "error writing to stdout:", err)
	// 	os.Exit(1)
	// }
	// fmt.Println()
}

func makeTransport(
	certificate string, key string,
	skipServerCertCheck bool,
) (*http.Transport, error) {
	// Start with the DefaultTransport for sane defaults.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Conservatively disable HTTP keep-alives as this program will only
	// ever need a single HTTP request.
	transport.DisableKeepAlives = true
	// Timeout early if the server doesn't even return the headers.
	transport.ResponseHeaderTimeout = time.Minute
	tlsConfig := &tls.Config{InsecureSkipVerify: skipServerCertCheck}
	if certificate != "" && key != "" {
		cert, err := tls.LoadX509KeyPair(certificate, key)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	transport.TLSClientConfig = tlsConfig
	return transport, nil
}
