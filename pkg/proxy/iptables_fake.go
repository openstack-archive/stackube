/*
Copyright (c) 2017 OpenStack Foundation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package proxy

import (
	"fmt"
	"strings"
	"sync"
)

const (
	Destination = "-d "
	Source      = "-s "
	DPort       = "--dport "
	Protocol    = "-p "
	Jump        = "-j "
	ToDest      = "--to-destination "
)

// Rule represents chain's rule.
type Rule map[string]string

// FakeIPTables have noop implementation of fake iptables function.
type FakeIPTables struct {
	sync.Mutex
	namespace string
	NSLines   map[string][]byte
}

// NewFake return new FakeIPTables.
func NewFake() *FakeIPTables {
	return &FakeIPTables{
		NSLines: make(map[string][]byte),
	}
}

func (f *FakeIPTables) ensureChain() error {
	return nil
}

func (f *FakeIPTables) ensureRule(op, chain string, args []string) error {
	return nil
}

func (f *FakeIPTables) restoreAll(data []byte) error {
	f.Lock()
	defer f.Unlock()
	d := make([]byte, len(data))
	copy(d, data)
	f.NSLines[f.namespace] = d
	return nil
}

func (f *FakeIPTables) netnsExist() bool {
	return true
}

func (f *FakeIPTables) setNetns(netns string) {
	f.namespace = netns
}

func getToken(line, seperator string) string {
	tokens := strings.Split(line, seperator)
	if len(tokens) == 2 {
		return strings.Split(tokens[1], " ")[0]
	}
	return ""
}

// GetRules returns a list of rules for the given chain.
// The chain name must match exactly.
// The matching is pretty dumb, don't rely on it for anything but testing.
func (f *FakeIPTables) GetRules(chainName, namespace string) (rules []Rule) {
	for _, l := range strings.Split(string(f.NSLines[namespace]), "\n") {
		if strings.Contains(l, fmt.Sprintf("-A %v", chainName)) {
			newRule := Rule(map[string]string{})
			for _, arg := range []string{Destination, Source, DPort, Protocol, Jump, ToDest} {
				tok := getToken(l, arg)
				if tok != "" {
					newRule[arg] = tok
				}
			}
			rules = append(rules, newRule)
		}
	}
	return
}

var _ = iptablesInterface(&FakeIPTables{})
