package check

import (
	"context"
	"testing"

	"github.com/mjhanzaibmemon/posture/internal/runner"
)

// Real output from "ss -tulpnH" on a hardened Ubuntu host that is on a tailnet.
// Keeping the genuine article here matters: made up fixtures agree with the
// parser by construction, which is how a parser passes its tests and then fails
// on the first real machine.
const ssOutput = `udp UNCONN 0      0                          0.0.0.0:41641 0.0.0.0:* users:(("tailscaled",pid=4658,fd=20))
udp UNCONN 0      0                       127.0.0.54:53    0.0.0.0:* users:(("systemd-resolve",pid=328,fd=16))
udp UNCONN 0      0                    127.0.0.53%lo:53    0.0.0.0:* users:(("systemd-resolve",pid=328,fd=14))
udp UNCONN 0      0               172.31.43.115%ens5:68    0.0.0.0:* users:(("systemd-network",pid=531,fd=11))
udp UNCONN 0      0                        127.0.0.1:323   0.0.0.0:* users:(("chronyd",pid=774,fd=5))
udp UNCONN 0      0                             [::]:41641    [::]:* users:(("tailscaled",pid=4658,fd=19))
udp UNCONN 0      0                            [::1]:323      [::]:* users:(("chronyd",pid=774,fd=6))
tcp LISTEN 0      4096                    127.0.0.54:53    0.0.0.0:* users:(("systemd-resolve",pid=328,fd=17))
tcp LISTEN 0      4096                100.64.106.115:63353 0.0.0.0:* users:(("tailscaled",pid=4658,fd=22))
tcp LISTEN 0      4096                 127.0.0.53%lo:53    0.0.0.0:* users:(("systemd-resolve",pid=328,fd=15))
tcp LISTEN 0      4096                       0.0.0.0:22    0.0.0.0:* users:(("sshd",pid=1382,fd=3))
tcp LISTEN 0      4096   [fd7a:115c:a1e0::8f2b:6a74]:50715    [::]:* users:(("tailscaled",pid=4658,fd=25))
tcp LISTEN 0      4096                          [::]:22       [::]:* users:(("sshd",pid=1382,fd=4))`

func TestParseListeners(t *testing.T) {
	got := parseListeners(ssOutput)

	if len(got) != 13 {
		t.Fatalf("parsed %d sockets, want 13", len(got))
	}

	first := got[0]
	if first.Proto != "udp" || first.Address != "0.0.0.0" || first.Port != "41641" {
		t.Errorf("first socket = %+v, want udp 0.0.0.0:41641", first)
	}

	// The interface suffix has to come off, or the address never parses.
	var dhcp Socket
	for _, s := range got {
		if s.Port == "68" {
			dhcp = s
		}
	}
	if dhcp.Address != "172.31.43.115" {
		t.Errorf("dhcp socket address = %q, want 172.31.43.115 with the interface stripped", dhcp.Address)
	}
}

func TestExposedSocketsIgnoresLoopbackAndTailnet(t *testing.T) {
	exposed, err := exposedSockets(parseListeners(ssOutput), defaultPrivatePrefixes)
	if err != nil {
		t.Fatalf("exposedSockets: %v", err)
	}

	want := map[string]bool{"41641/udp": true, "68/udp": true, "22/tcp": true}
	got := map[string]bool{}
	for _, s := range exposed {
		got[s.PortProto()] = true
	}

	for k := range want {
		if !got[k] {
			t.Errorf("%s should be treated as externally reachable", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("%s is bound to loopback or the tailnet and should not count as exposure", k)
		}
	}
}

func TestListenersCheck(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		want    Status
	}{
		{name: "everything allowed", allowed: []string{"22/tcp", "41641/udp", "68/udp"}, want: Pass},
		{name: "ssh not allowed", allowed: []string{"41641/udp", "68/udp"}, want: Fail},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner.Fake{Answers: []runner.Canned{
				{Match: "ss -tulpnH", Result: runner.Result{Stdout: ssOutput}},
			}}

			got := Listeners{Allowed: tc.allowed}.Run(context.Background(), r)
			if got.Status != tc.want {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Detail, tc.want)
			}
			if got.Evidence == "" {
				t.Error("a result without evidence is an opinion, not a check")
			}
		})
	}
}

func TestListenersReportsCommandFailure(t *testing.T) {
	r := &runner.Fake{Answers: []runner.Canned{
		{Match: "ss -tulpnH", Result: runner.Result{ExitCode: 1, Stderr: "ss: command not found"}},
	}}

	got := Listeners{Allowed: []string{"22/tcp"}}.Run(context.Background(), r)
	if got.Status != Error {
		t.Fatalf("status = %s, want ERROR: a command that did not run cannot prove the host is clean", got.Status)
	}
}
