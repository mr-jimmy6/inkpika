package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

const transferChain = "INKPIKA_TRANSFER"

// Kindle's INPUT policy drops unsolicited Wi-Fi traffic. Own only this named
// chain and its exact wlan0 jump; never change the policy or other services.
func localWebFirewall(port int, chain string) (func(), error) {
	filename := ""
	for _, path := range []string{"/usr/sbin/iptables", "/sbin/iptables"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			filename = path
			break
		}
	}
	if filename == "" {
		return func() {}, nil
	}
	return configureLocalWebFirewall(port, chain, func(args ...string) error {
		out, err := exec.Command(filename, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("LAN service firewall setup failed: %w (%s)", err, out)
		}
		return nil
	})
}

func configureLocalWebFirewall(port int, chain string, run func(...string) error) (func(), error) {
	cleanup := func() {
		_ = run("-D", "INPUT", "-i", "wlan0", "-j", chain)
		_ = run("-F", chain)
		_ = run("-X", chain)
	}
	// A killed previous transfer process may have left its chain behind.
	if run("-S", chain) == nil {
		cleanup()
	}
	if err := run("-N", chain); err != nil {
		return nil, err
	}
	for _, source := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		if err := run("-A", chain, "-s", source, "-p", "tcp", "--dport", strconv.Itoa(port), "-j", "ACCEPT"); err != nil {
			cleanup()
			return nil, err
		}
	}
	if err := run("-I", "INPUT", "1", "-i", "wlan0", "-j", chain); err != nil {
		cleanup()
		return nil, err
	}
	return cleanup, nil
}
