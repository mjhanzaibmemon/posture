package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSSHTarget(t *testing.T) {
	cases := []struct {
		name string
		ssh  SSH
		want string
	}{
		{name: "user and host", ssh: SSH{Host: "198.51.100.7", User: "deploy"}, want: "deploy@198.51.100.7"},
		{name: "host only", ssh: SSH{Host: "web-01"}, want: "web-01"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ssh.Target(); got != tc.want {
				t.Errorf("Target() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFakeMatchesInOrder(t *testing.T) {
	f := &Fake{Answers: []Canned{
		{Match: "systemctl is-active", Result: Result{Stdout: "active\n"}},
		{Match: "systemctl", Result: Result{Stdout: "something else\n"}},
	}}

	got, err := f.Run(context.Background(), "systemctl is-active docker")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Stdout != "active\n" {
		t.Errorf("stdout = %q, want the first matching answer to win", got.Stdout)
	}

	if calls := f.Calls(); len(calls) != 1 || !strings.Contains(calls[0], "docker") {
		t.Errorf("calls = %v, want the command recorded", calls)
	}
}

func TestFakeWithoutAnAnswer(t *testing.T) {
	f := &Fake{}

	got, err := f.Run(context.Background(), "ss -tulpnH")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.ExitCode == 0 {
		t.Error("an unscripted command should not look like it succeeded")
	}
}

func TestFakeReturnsScriptedError(t *testing.T) {
	want := errors.New("ssh: connection refused")
	f := &Fake{Answers: []Canned{{Match: "sshd -T", Err: want}}}

	if _, err := f.Run(context.Background(), "sudo sshd -T"); !errors.Is(err, want) {
		t.Errorf("err = %v, want it passed through unchanged", err)
	}
}

func TestLocalRunsAShellCommand(t *testing.T) {
	// Skipped on machines without a POSIX shell rather than failing: the point
	// is to exercise the real exec path where one exists.
	got, err := Local{}.Run(context.Background(), "echo posture")
	if err != nil {
		t.Skipf("no usable shell here: %v", err)
	}
	if strings.TrimSpace(got.Stdout) != "posture" {
		t.Errorf("stdout = %q, want %q", got.Stdout, "posture")
	}
}

func TestLocalReportsExitCodeWithoutAnError(t *testing.T) {
	got, err := Local{}.Run(context.Background(), "exit 7")
	if err != nil {
		t.Skipf("no usable shell here: %v", err)
	}
	if got.ExitCode != 7 {
		t.Errorf("exit code = %d, want 7 reported as data rather than as a failure to run", got.ExitCode)
	}
}
