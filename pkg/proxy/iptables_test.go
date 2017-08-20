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
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/exec"
	fakeexec "k8s.io/utils/exec/testing"
)

func TestEnsureChain(t *testing.T) {
	fcmd := fakeexec.FakeCmd{
		CombinedOutputScript: []fakeexec.FakeCombinedOutputAction{
			// Success.
			func() ([]byte, error) { return []byte{}, nil },
			// Exists.
			func() ([]byte, error) { return nil, &fakeexec.FakeExitError{Status: 1} },
			// Failure.
			func() ([]byte, error) { return nil, &fakeexec.FakeExitError{Status: 2} },
		},
	}
	fexec := fakeexec.FakeExec{
		CommandScript: []fakeexec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}
	ipt := NewIptables(&fexec)
	ipt.setNetns("FOO")
	// Success.
	err := ipt.ensureChain()
	fmt.Println(err)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	if fcmd.CombinedOutputCalls != 1 {
		t.Errorf("expected 1 CombinedOutput() calls, got %d", fcmd.CombinedOutputCalls)
	}
	if !sets.NewString(fcmd.CombinedOutputLog[0]...).HasAll("ip", "netns", "exec", "FOO", "iptables", "-t", "nat", "-N", ChainSKPrerouting) {
		t.Errorf("wrong CombinedOutput() log, got %s", fcmd.CombinedOutputLog[0])
	}
	// Exists.
	err = ipt.ensureChain()
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	// Failure.
	err = ipt.ensureChain()
	if err == nil {
		t.Errorf("expected failure")
	}
}

func TestEnsureRuleAlreadyExists(t *testing.T) {
	fcmd := fakeexec.FakeCmd{
		CombinedOutputScript: []fakeexec.FakeCombinedOutputAction{
			// Success.
			func() ([]byte, error) { return []byte{}, nil },
		},
	}
	fexec := fakeexec.FakeExec{
		CommandScript: []fakeexec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}
	ipt := NewIptables(&fexec)
	ipt.setNetns("FOO")

	err := ipt.ensureRule(opAddpendRule, ChainPrerouting, []string{"abc", "123"})
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	if fcmd.CombinedOutputCalls != 1 {
		t.Errorf("expected 1 CombinedOutput() calls, got %d", fcmd.CombinedOutputCalls)
	}
	if !sets.NewString(fcmd.CombinedOutputLog[0]...).HasAll("ip", "netns", "exec", "FOO", "iptables", "-t", "nat", "-C", "PREROUTING", "abc", "123") {
		t.Errorf("wrong CombinedOutput() log, got %s", fcmd.CombinedOutputLog[0])
	}
}

func TestEnsureRuleNew(t *testing.T) {
	fcmd := fakeexec.FakeCmd{
		CombinedOutputScript: []fakeexec.FakeCombinedOutputAction{
			// Status 1 on the first call.
			func() ([]byte, error) { return nil, &fakeexec.FakeExitError{Status: 1} },
			// Success on the second call.
			func() ([]byte, error) { return []byte{}, nil },
		},
	}
	fexec := fakeexec.FakeExec{
		CommandScript: []fakeexec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}
	ipt := NewIptables(&fexec)
	ipt.setNetns("FOO")

	err := ipt.ensureRule(opAddpendRule, ChainPrerouting, []string{"abc", "123"})
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	if fcmd.CombinedOutputCalls != 2 {
		t.Errorf("expected 2 CombinedOutput() calls, got %d", fcmd.CombinedOutputCalls)
	}
	if !sets.NewString(fcmd.CombinedOutputLog[1]...).HasAll("ip", "netns", "exec", "FOO", "iptables", "-t", "nat", "-A", "PREROUTING", "abc", "123") {
		t.Errorf("wrong CombinedOutput() log, got %s", fcmd.CombinedOutputLog[1])
	}
}

func TestEnsureRuleErrorChecking(t *testing.T) {
	fcmd := fakeexec.FakeCmd{
		CombinedOutputScript: []fakeexec.FakeCombinedOutputAction{
			// Status 2 on the first call.
			func() ([]byte, error) { return nil, &fakeexec.FakeExitError{Status: 2} },
		},
	}
	fexec := fakeexec.FakeExec{
		CommandScript: []fakeexec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}
	ipt := NewIptables(&fexec)
	ipt.setNetns("FOO")

	err := ipt.ensureRule(opAddpendRule, ChainPrerouting, []string{"abc", "123"})
	if err == nil {
		t.Errorf("expected failure")
	}
	if fcmd.CombinedOutputCalls != 1 {
		t.Errorf("expected 1 CombinedOutput() calls, got %d", fcmd.CombinedOutputCalls)
	}
}

func TestEnsureRuleErrorCreating(t *testing.T) {
	fcmd := fakeexec.FakeCmd{
		CombinedOutputScript: []fakeexec.FakeCombinedOutputAction{
			// Status 1 on the first call.
			func() ([]byte, error) { return nil, &fakeexec.FakeExitError{Status: 1} },
			// Status 1 on the second call.
			func() ([]byte, error) { return nil, &fakeexec.FakeExitError{Status: 1} },
		},
	}
	fexec := fakeexec.FakeExec{
		CommandScript: []fakeexec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}
	ipt := NewIptables(&fexec)
	ipt.setNetns("FOO")

	err := ipt.ensureRule(opAddpendRule, ChainPrerouting, []string{"abc", "123"})
	if err == nil {
		t.Errorf("expected failure")
	}
	if fcmd.CombinedOutputCalls != 2 {
		t.Errorf("expected 2 CombinedOutput() calls, got %d", fcmd.CombinedOutputCalls)
	}
}

func TestRestoreAll(t *testing.T) {
	fcmd := fakeexec.FakeCmd{
		CombinedOutputScript: []fakeexec.FakeCombinedOutputAction{
			func() ([]byte, error) { return []byte{}, nil },
		},
	}
	fexec := fakeexec.FakeExec{
		CommandScript: []fakeexec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}
	ipt := NewIptables(&fexec)
	ipt.setNetns("FOO")

	err := ipt.restoreAll([]byte{})
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}

	if fcmd.CombinedOutputCalls != 1 {
		t.Errorf("expected 1 CombinedOutput() calls, got %d", fcmd.CombinedOutputCalls)
	}

	if !sets.NewString(fcmd.CombinedOutputLog[0]...).HasAll("ip", "netns", "exec", "FOO", "iptables-restore", "--noflush", "--counters") {
		t.Errorf("wrong CombinedOutput() log, got %s", fcmd.CombinedOutputLog[0])
	}
}

func TestNetnsExist(t *testing.T) {
	fcmd := fakeexec.FakeCmd{
		CombinedOutputScript: []fakeexec.FakeCombinedOutputAction{
			func() ([]byte, error) { return []byte{}, nil },
		},
	}
	fexec := fakeexec.FakeExec{
		CommandScript: []fakeexec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}
	ipt := NewIptables(&fexec)
	ipt.setNetns("FOO")

	ok := ipt.netnsExist()
	if !ok {
		t.Errorf("expected true, got %v", ok)
	}

	if fcmd.CombinedOutputCalls != 1 {
		t.Errorf("expected 1 CombinedOutput() calls, got %d", fcmd.CombinedOutputCalls)
	}

	if !sets.NewString(fcmd.CombinedOutputLog[0]...).HasAll("ip", "netns", "pids", "FOO") {
		t.Errorf("wrong CombinedOutput() log, got %s", fcmd.CombinedOutputLog[0])
	}
}
