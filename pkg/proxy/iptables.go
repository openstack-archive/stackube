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
	"bytes"
	"fmt"

	"github.com/golang/glog"
	utilexec "k8s.io/utils/exec"
)

const (
	TableNAT = "nat"

	ChainPrerouting   = "PREROUTING"
	ChainSKPrerouting = "STACKUBE-PREROUTING"

	opCreateChain = "-N"
	opFlushChain  = "-F"
	opAddpendRule = "-A"
	opCheckRule   = "-C"
	opDeleteRule  = "-D"
)

// iptablesInterface is an injectable interface for running iptables commands.
type iptablesInterface interface {
	// ensureChain ensures chain STACKUBE-PREROUTING is created.
	ensureChain() error
	// ensureRule links STACKUBE-PREROUTING chain.
	ensureRule(op, chain string, args []string) error
	// restoreAll runs `iptables-restore` passing data through []byte.
	restoreAll(data []byte) error
	// netnsExist checks netns exist or not.
	netnsExist() bool
	// setNetns populates namespace of iptables.
	setNetns(netns string)
}

type Iptables struct {
	exec      utilexec.Interface
	namespace string
}

func NewIptables(exec utilexec.Interface) iptablesInterface {
	return &Iptables{
		exec: exec,
	}
}

func (r *Iptables) setNetns(netns string) {
	r.namespace = netns
}

// runInNat executes iptables command in nat table.
func (r *Iptables) runInNat(op, chain string, args []string) ([]byte, error) {
	fullArgs := []string{"netns", "exec", r.namespace, "iptables", "-t", TableNAT, op, chain}
	fullArgs = append(fullArgs, args...)
	return r.exec.Command("ip", fullArgs...).CombinedOutput()
}

func (r *Iptables) restoreAll(data []byte) error {
	glog.V(3).Infof("running iptables-restore with data %s", data)

	fullArgs := []string{"netns", "exec", r.namespace, "iptables-restore", "--noflush", "--counters"}
	cmd := r.exec.Command("ip", fullArgs...)
	cmd.SetStdin(bytes.NewBuffer(data))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables-restore failed: %s: %v", output, err)
	}

	return nil
}

// ensureChain ensures chain STACKUBE-PREROUTING is created.
func (r *Iptables) ensureChain() error {
	output, err := r.runInNat(opCreateChain, ChainSKPrerouting, nil)
	if err == nil {
		return nil
	}

	if ee, ok := err.(utilexec.ExitError); ok {
		if ee.Exited() && ee.ExitStatus() == 1 {
			return nil
		}
	}
	return fmt.Errorf("ensure rule failed: %v: %s", err, output)
}

func (r *Iptables) checkRule(chain string, args []string) (bool, error) {
	out, err := r.runInNat(opCheckRule, chain, args)
	if err == nil {
		return true, nil
	}

	if ee, ok := err.(utilexec.ExitError); ok {
		if ee.Exited() && ee.ExitStatus() == 1 {
			return false, nil
		}
	}

	return false, fmt.Errorf("error checking rule: %v: %s", err, out)
}

func (r *Iptables) ensureRule(op, chain string, args []string) error {
	exists, err := r.checkRule(chain, args)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	out, err := r.runInNat(op, chain, args)
	if err != nil {
		return fmt.Errorf("error ensuring rule: %v: %s", err, out)
	}

	return nil
}

// Join all words with spaces, terminate with newline and write to buf.
func writeLine(buf *bytes.Buffer, words ...string) {
	// We avoid strings.Join for performance reasons.
	for i := range words {
		buf.WriteString(words[i])
		if i < len(words)-1 {
			buf.WriteByte(' ')
		} else {
			buf.WriteByte('\n')
		}
	}
}

func (r *Iptables) netnsExist() bool {
	args := []string{"netns", "pids", r.namespace}
	out, err := r.exec.Command("ip", args...).CombinedOutput()
	if err != nil {
		glog.V(5).Infof("Checking netns %q failed: %s: %v", r.namespace, out, err)
		return false
	}

	return true
}
