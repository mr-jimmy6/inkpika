package main

import (
	"errors"
	"strings"
	"testing"
)

func TestTransferFirewallScopedAndRolledBack(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var calls []string
		run := func(args ...string) error {
			line := strings.Join(args, " ")
			calls = append(calls, line)
			if args[0] == "-S" {
				return errors.New("absent")
			}
			if fail && args[0] == "-I" {
				return errors.New("fixture insertion failed")
			}
			return nil
		}
		cleanup, err := configureLocalWebFirewall(8789, transferChain, run)
		if fail {
			if err == nil {
				t.Fatal("failure ignored")
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			cleanup()
		}
		joined := strings.Join(calls, "\n")
		if strings.Contains(joined, "-P ") || strings.Contains(joined, "-F INPUT") || strings.Contains(joined, "8790") {
			t.Fatal("firewall scope widened")
		}
		if !strings.Contains(joined, "-I INPUT 1 -i wlan0 -j "+transferChain) || !strings.HasSuffix(joined, "-X "+transferChain) {
			t.Fatal("missing insertion or cleanup", joined)
		}
		for _, src := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
			if !strings.Contains(joined, "-s "+src+" -p tcp --dport 8789 -j ACCEPT") {
				t.Fatal("incorrect allowed traffic")
			}
		}
	}
}
