// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package loglist2

import (
	"log"
	"testing"
	"time"

	"github.com/google/certificate-transparency-go/testdata"
	"github.com/google/certificate-transparency-go/trillian/ctfe"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/certificate-transparency-go/x509util"

	"github.com/kylelemons/godebug/pretty"
)

func subLogList(logURLs map[string]bool) LogList {
	var ll LogList
	for _, op := range sampleLogList.Operators {
		opCopy := *op
		opCopy.Logs = []*Log{}
		for _, l := range op.Logs {
			if logURLs[l.URL] {
				opCopy.Logs = append(opCopy.Logs, l)
			}
		}
		if len(opCopy.Logs) > 0 {
			ll.Operators = append(ll.Operators, &opCopy)
		}
	}
	return ll
}

func TestSelectUsable(t *testing.T) {
	tests := []struct {
		name string
		in   LogList
		want LogList
	}{
		{
			name: "Sample",
			in:   sampleLogList,
			want: subLogList(map[string]bool{"ct.googleapis.com/icarus/": true}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in.SelectUsable()
			if diff := pretty.Compare(test.want, got); diff != "" {
				t.Errorf("Extracting active logs out of %v diff: (-want +got)\n%s", test.in, diff)
			}
		})
	}
}

func TestSelectQualified(t *testing.T) {
	tests := []struct {
		name string
		in   LogList
		want LogList
	}{
		{
			name: "Sample",
			in:   sampleLogList,
			want: subLogList(map[string]bool{"https://ct.googleapis.com/logs/argon2020/": true}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in.SelectQualified()
			if diff := pretty.Compare(test.want, got); diff != "" {
				t.Errorf("Extracting qualified logs out of %v diff: (-want +got)\n%s", test.in, diff)
			}
		})
	}
}

func artificialRoots(source string) LogRoots {
	roots := LogRoots{
		"https://log.bob.io":                        ctfe.NewPEMCertPool(),
		"https://ct.googleapis.com/racketeer/":      ctfe.NewPEMCertPool(),
		"https://ct.googleapis.com/rocketeer/":      ctfe.NewPEMCertPool(),
		"https://ct.googleapis.com/aviator/":        ctfe.NewPEMCertPool(),
		"https://ct.googleapis.com/logs/argon2020/": ctfe.NewPEMCertPool(),
	}
	roots["https://log.bob.io"].AppendCertsFromPEM([]byte(source))
	return roots
}

func TestRootCompatible(t *testing.T) {
	cert, _ := x509util.CertificateFromPEM([]byte(testdata.TestPreCertPEM))
	caCert, _ := x509util.CertificateFromPEM([]byte(testdata.CACertPEM))

	tests := []struct {
		name     string
		in       LogList
		cert     *x509.Certificate
		rootCert *x509.Certificate
		roots    LogRoots
		want     LogList
	}{
		{
			name:     "RootedChain",
			in:       sampleLogList,
			cert:     cert,
			rootCert: caCert,
			roots:    artificialRoots(testdata.CACertPEM),
			want:     subLogList(map[string]bool{"https://log.bob.io": true, "https://ct.googleapis.com/icarus/": true}), // icarus has no root info.
		},
		{
			name:     "RootedChainNoRootAccepted",
			in:       sampleLogList,
			cert:     cert,
			rootCert: caCert,
			roots:    artificialRoots(testdata.TestPreCertPEM),
			want:     subLogList(map[string]bool{"https://ct.googleapis.com/icarus/": true}), // icarus has no root info.
		},
		{
			name:     "UnRootedChain",
			in:       sampleLogList,
			cert:     cert,
			rootCert: cert,
			roots:    artificialRoots(testdata.CACertPEM),
			want:     subLogList(map[string]bool{}),
		},
		{
			name:     "EmptyChain",
			in:       sampleLogList,
			cert:     nil,
			rootCert: nil,
			roots:    artificialRoots(testdata.CACertPEM),
			want:     subLogList(map[string]bool{"https://ct.googleapis.com/icarus/": true}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in.RootCompatible(test.cert, test.rootCert, test.roots)
			if diff := pretty.Compare(test.want, got); diff != "" {
				t.Errorf("Getting compatible logs diff: (-want +got)\n%s", diff)
			}
		})
	}
}

// stripErr is helper func for wrapping time.Parse
func stripErr(t time.Time, err error) time.Time {
	if err != nil {
		log.Fatal(err)
	}
	return t
}

func TestTemporalCompatible(t *testing.T) {
	cert, _ := x509util.CertificateFromPEM([]byte(testdata.TestPreCertPEM))

	tests := []struct {
		name      string
		in        LogList
		cert      *x509.Certificate
		notBefore time.Time
		want      LogList
	}{
		{
			name:      "AllLogsFitTemporal",
			in:        sampleLogList,
			cert:      cert,
			notBefore: stripErr(time.Parse(time.UnixDate, "Sat Nov 8 11:06:00 PST 2014")),
			want:      subLogList(map[string]bool{"https://ct.googleapis.com/aviator/": true, "https://log.bob.io": true, "https://ct.googleapis.com/icarus/": true, "https://ct.googleapis.com/racketeer/": true, "https://ct.googleapis.com/rocketeer/": true}),
		},
		{
			name:      "OperatorExcludedAllItsLogsMismatch",
			in:        sampleLogList,
			cert:      cert,
			notBefore: stripErr(time.Parse(time.UnixDate, "Sat Mar 8 11:06:00 PST 2014")),
			want:      subLogList(map[string]bool{"https://ct.googleapis.com/aviator/": true, "https://ct.googleapis.com/icarus/": true, "https://ct.googleapis.com/racketeer/": true, "https://ct.googleapis.com/rocketeer/": true}),
		},
		{
			name:      "TwoLogsAfterCertTimeExcluded",
			in:        sampleLogList,
			cert:      cert,
			notBefore: stripErr(time.Parse(time.UnixDate, "Sat Mar 8 11:06:00 PST 2013")),
			want:      subLogList(map[string]bool{"https://ct.googleapis.com/icarus/": true, "https://ct.googleapis.com/racketeer/": true, "https://ct.googleapis.com/rocketeer/": true}),
		},
		{
			name:      "TwoLogsBeforeCertTimeExcluded",
			in:        sampleLogList,
			cert:      cert,
			notBefore: stripErr(time.Parse(time.UnixDate, "Sat Mar 8 11:06:00 PST 2016")),
			want:      subLogList(map[string]bool{"https://ct.googleapis.com/icarus/": true, "https://ct.googleapis.com/racketeer/": true, "https://ct.googleapis.com/rocketeer/": true}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.cert.NotBefore = test.notBefore
			got := test.in.TemporalCompatible(test.cert)
			if diff := pretty.Compare(test.want, got); diff != "" {
				t.Errorf("Getting NotBefore-compatible logs diff: (-want +got)\n%s", diff)
			}
		})
	}
}
