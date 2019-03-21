// Copyright 2019 Google Inc. All Rights Reserved.
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

package submission

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	ct "github.com/google/certificate-transparency-go"
	"github.com/google/certificate-transparency-go/client"
	"github.com/google/certificate-transparency-go/ctpolicy"
	"github.com/google/certificate-transparency-go/loglist"
	"github.com/google/certificate-transparency-go/testdata"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/certificate-transparency-go/x509util"

	"github.com/google/go-cmp/cmp"
)

// readCertFile returns the first certificate it finds in file provided.
func readCertFile(filename string) []byte {
	data, err := x509util.ReadPossiblePEMFile(filename, "CERTIFICATE")
	if err != nil {
		return nil
	}
	return data[0]
}

type rootInfo struct {
	raw      []byte
	filename string
}

var (
	RootsCerts = map[string][]rootInfo{
		"ct.googleapis.com/aviator/": {
			rootInfo{filename: "../trillian/testdata/fake-ca-1.cert"},
			rootInfo{filename: "testdata/some.cert"},
		},
		"ct.googleapis.com/rocketeer/": {
			rootInfo{filename: "../trillian/testdata/fake-ca.cert"},
			rootInfo{filename: "../trillian/testdata/fake-ca-1.cert"},
			rootInfo{filename: "testdata/some.cert"},
			rootInfo{filename: "testdata/another.cert"},
		},
		"ct.googleapis.com/icarus/": {
			rootInfo{raw: []byte("invalid000")},
			rootInfo{filename: "testdata/another.cert"},
		},
		"uncollectable-roots/log/": {
			rootInfo{raw: []byte("invalid")},
		},
	}
)

// buildNoLogClient is LogClientBuilder that always fails.
func buildNoLogClient(_ *loglist.Log) (client.AddLogClient, error) {
	return nil, errors.New("bad client builder")
}

// Stub for AddLogClient interface
type emptyLogClient struct {
}

func (e emptyLogClient) AddChain(ctx context.Context, chain []ct.ASN1Cert) (*ct.SignedCertificateTimestamp, error) {
	return nil, nil
}

func (e emptyLogClient) AddPreChain(ctx context.Context, chain []ct.ASN1Cert) (*ct.SignedCertificateTimestamp, error) {
	return nil, nil
}

func (e emptyLogClient) GetAcceptedRoots(ctx context.Context) ([]ct.ASN1Cert, error) {
	return nil, nil
}

// buildEmptyLogClient produces empty stub Log clients.
func buildEmptyLogClient(_ *loglist.Log) (client.AddLogClient, error) {
	return emptyLogClient{}, nil
}

func sampleLogList() *loglist.LogList {
	var loglist loglist.LogList
	if err := json.Unmarshal([]byte(testdata.SampleLogList), &loglist); err != nil {
		panic(fmt.Errorf("unable to Unmarshal testdata.SampleLogList: %v", err))
	}
	return &loglist
}

func sampleValidLogList() *loglist.LogList {
	ll := sampleLogList()
	// Id of invalid Log description Racketeer
	inval := 3
	ll.Logs = append(ll.Logs[:inval], ll.Logs[inval+1:]...)
	return ll
}

func sampleUncollectableLogList() *loglist.LogList {
	ll := sampleValidLogList()
	// Append loglist that is unable to provide roots on request.
	ll.Logs = append(ll.Logs, loglist.Log{
		Description: "Does not return roots", Key: []byte("VW5jb2xsZWN0YWJsZUxvZ0xpc3Q="),
		MaximumMergeDelay: 123, OperatedBy: []int{0},
		URL:            "uncollectable-roots/log/",
		DNSAPIEndpoint: "uncollectavle.ct.googleapis.com",
	})
	return ll
}

func TestNewDistributorLogClients(t *testing.T) {
	testCases := []struct {
		name      string
		ll        *loglist.LogList
		lcBuilder LogClientBuilder
		errRegexp *regexp.Regexp
	}{
		{
			name:      "ValidLogClients",
			ll:        sampleValidLogList(),
			lcBuilder: buildEmptyLogClient,
		},
		{
			name:      "NoLogClients",
			ll:        sampleValidLogList(),
			lcBuilder: buildNoLogClient,
			errRegexp: regexp.MustCompile("failed to create log client"),
		},
		{
			name:      "NoLogClientsEmptyLogList",
			ll:        &loglist.LogList{},
			lcBuilder: buildNoLogClient,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDistributor(tc.ll, ctpolicy.ChromeCTPolicy{}, tc.lcBuilder)
			if gotErr, wantErr := err != nil, tc.errRegexp != nil; gotErr != wantErr {
				var unwantedErr string
				if gotErr {
					unwantedErr = fmt.Sprintf(" (%q)", err)
				}
				t.Errorf("Got error = %v%s, expected error = %v", gotErr, unwantedErr, wantErr)
			} else if tc.errRegexp != nil && !tc.errRegexp.MatchString(err.Error()) {
				t.Errorf("Error %q did not match expected regexp %q", err, tc.errRegexp)
			}
		})
	}
}

// TestSCT builds a mock SCT for given logURL.
func testSCT(logURL string) *ct.SignedCertificateTimestamp {
	var keyID [sha256.Size]byte
	copy(keyID[:], logURL)
	return &ct.SignedCertificateTimestamp{
		SCTVersion: ct.V1,
		LogID:      ct.LogID{KeyID: keyID},
		Timestamp:  1234,
		Extensions: []byte{},
		Signature: ct.DigitallySigned{
			Algorithm: tls.SignatureAndHashAlgorithm{
				Hash:      tls.SHA256,
				Signature: tls.ECDSA,
			},
		},
	}
}

// Stub for AddLogCLient interface
type stubLogClient struct {
	logURL string
}

func (m stubLogClient) AddChain(ctx context.Context, chain []ct.ASN1Cert) (*ct.SignedCertificateTimestamp, error) {
	return nil, nil
}

func (m stubLogClient) AddPreChain(ctx context.Context, chain []ct.ASN1Cert) (*ct.SignedCertificateTimestamp, error) {
	if _, ok := RootsCerts[m.logURL]; ok {
		return testSCT(m.logURL), nil
	}
	return nil, fmt.Errorf("log %q has no roots", m.logURL)
}

func (m stubLogClient) GetAcceptedRoots(ctx context.Context) ([]ct.ASN1Cert, error) {
	roots := []ct.ASN1Cert{}
	if certInfos, ok := RootsCerts[m.logURL]; ok {
		for _, certInfo := range certInfos {
			if len(certInfo.raw) > 0 {
				roots = append(roots, ct.ASN1Cert{Data: certInfo.raw})
			} else {

				roots = append(roots, ct.ASN1Cert{Data: readCertFile(certInfo.filename)})
			}
		}
	}
	return roots, nil
}

func buildStubLogClient(log *loglist.Log) (client.AddLogClient, error) {
	return stubLogClient{logURL: log.URL}, nil
}

func TestNewDistributorRootPools(t *testing.T) {
	testCases := []struct {
		name    string
		ll      *loglist.LogList
		rootNum map[string]int
	}{
		{
			name:    "InactiveZeroRoots",
			ll:      sampleValidLogList(),
			rootNum: map[string]int{"ct.googleapis.com/aviator/": 0, "ct.googleapis.com/rocketeer/": 4, "ct.googleapis.com/icarus/": 1}, // aviator is not active; 1 of 2 icarus roots is not x509 struct
		},
		{
			name:    "CouldNotCollect",
			ll:      sampleUncollectableLogList(),
			rootNum: map[string]int{"ct.googleapis.com/aviator/": 0, "ct.googleapis.com/rocketeer/": 4, "ct.googleapis.com/icarus/": 1, "uncollectable-roots/log/": 0}, // aviator is not active; uncollectable client cannot provide roots
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dist, _ := NewDistributor(tc.ll, ctpolicy.ChromeCTPolicy{}, buildStubLogClient)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			go dist.Run(ctx)
			// First Log refresh expected.
			<-ctx.Done()

			dist.mu.Lock()
			defer dist.mu.Unlock()
			for logURL, wantNum := range tc.rootNum {
				gotNum := 0
				if roots, ok := dist.logRoots[logURL]; ok {
					gotNum = len(roots.RawCertificates())
				}
				if wantNum != gotNum {
					t.Errorf("Expected %d root(s) for Log %s, got %d", wantNum, logURL, gotNum)
				}
			}
		})
	}
}

func pemFileToDERChain(filename string) [][]byte {
	rawChain, err := x509util.ReadPossiblePEMFile(filename, "CERTIFICATE")
	if err != nil {
		panic(err)
	}
	return rawChain
}

func getSCTMap(l []*AssignedSCT) map[string]*AssignedSCT {
	m := map[string]*AssignedSCT{}
	for _, asct := range l {
		m[asct.LogURL] = asct
	}
	return m
}

// Stub CT policy to run tests.
type stubCTPolicy struct {
	baseNum int
}

// Builds simplistic policy requiring n SCTs from any Logs for each cert.
func buildStubCTPolicy(n int) stubCTPolicy {
	return stubCTPolicy{baseNum: n}
}

func (stubP stubCTPolicy) LogsByGroup(cert *x509.Certificate, approved *loglist.LogList) (ctpolicy.LogPolicyData, error) {
	baseGroup, err := ctpolicy.BaseGroupFor(approved, stubP.baseNum)
	groups := ctpolicy.LogPolicyData{baseGroup.Name: &baseGroup}
	return groups, err
}

func TestDistributorAddPreChain(t *testing.T) {
	testCases := []struct {
		name     string
		ll       *loglist.LogList
		plc      ctpolicy.CTPolicy
		rawChain [][]byte
		getRoots bool
		scts     []*AssignedSCT
		wantErr  bool
	}{
		{
			name:     "MalformedChainRequest with log roots available",
			ll:       sampleValidLogList(),
			plc:      ctpolicy.ChromeCTPolicy{},
			rawChain: pemFileToDERChain("../trillian/testdata/subleaf.misordered.chain"),
			getRoots: true,
			scts:     nil,
			wantErr:  true,
		},
		{
			name:     "MalformedChainRequest without log roots available",
			ll:       sampleValidLogList(),
			plc:      ctpolicy.ChromeCTPolicy{},
			rawChain: pemFileToDERChain("../trillian/testdata/subleaf.misordered.chain"),
			getRoots: false,
			scts:     nil,
			wantErr:  true,
		},
		{
			name:     "CallBeforeInit",
			ll:       sampleValidLogList(),
			plc:      ctpolicy.ChromeCTPolicy{},
			rawChain: nil,
			scts:     nil,
			wantErr:  true,
		},
		{
			name:     "InsufficientSCTsForPolicy",
			ll:       sampleValidLogList(),
			plc:      ctpolicy.AppleCTPolicy{},
			rawChain: pemFileToDERChain("../trillian/testdata/subleaf.chain"), // subleaf chain is fake-ca-1-rooted
			getRoots: true,
			scts:     []*AssignedSCT{},
			wantErr:  true, // Not enough SCTs for policy
		},
		{
			name:     "FullChain1Policy",
			ll:       sampleValidLogList(),
			plc:      buildStubCTPolicy(1),
			rawChain: pemFileToDERChain("../trillian/testdata/subleaf.chain"),
			getRoots: true,
			scts: []*AssignedSCT{
				{
					LogURL: "ct.googleapis.com/rocketeer/",
					SCT:    testSCT("ct.googleapis.com/rocketeer/"),
				},
			},
			wantErr: false,
		},
		// TODO(merkulova): Add tests to cover more cases where log roots aren't available
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dist, _ := NewDistributor(tc.ll, tc.plc, buildStubLogClient)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			if tc.getRoots {
				dist.Run(ctx)
			}

			scts, err := dist.AddPreChain(context.Background(), tc.rawChain)
			if gotErr := (err != nil); gotErr != tc.wantErr {
				t.Errorf("Expected to get errors is %v while actually getting errors is %v", tc.wantErr, gotErr)
			}

			if got, want := len(scts), len(tc.scts); got != want {
				t.Errorf("Expected to get %d SCTs on AddPreChain request, got %d", want, got)
			}
			gotMap := getSCTMap(tc.scts)
			for _, asct := range scts {
				if wantedSCT, ok := gotMap[asct.LogURL]; !ok {
					t.Errorf("dist.AddPreChain() = (_, %v), want err? %t", err, tc.wantErr)
				} else if diff := cmp.Diff(asct, wantedSCT); diff != "" {
					t.Errorf("Got unexpected SCT for Log %q", asct.LogURL)
				}
			}
		})
	}
}
