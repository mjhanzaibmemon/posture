package check

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mjhanzaibmemon/posture/internal/runner"
)

const sshdOutput = `port 22
permitrootlogin no
passwordauthentication no
pubkeyauthentication yes
maxauthtries 3
x11forwarding no
hostkey /etc/ssh/ssh_host_rsa_key
hostkey /etc/ssh/ssh_host_ed25519_key`

func TestParseSSHD(t *testing.T) {
	got := parseSSHD(sshdOutput)

	if got["permitrootlogin"] != "no" {
		t.Errorf("permitrootlogin = %q, want no", got["permitrootlogin"])
	}
	// A repeated keyword should not produce two entries or an empty one.
	if got["hostkey"] != "/etc/ssh/ssh_host_ed25519_key" {
		t.Errorf("hostkey = %q, want the last one to win", got["hostkey"])
	}
}

func TestSSHDCheck(t *testing.T) {
	cases := []struct {
		name   string
		expect map[string]string
		want   Status
	}{
		{
			name:   "matches",
			expect: map[string]string{"permitrootlogin": "no", "passwordauthentication": "no"},
			want:   Pass,
		},
		{
			name:   "value differs",
			expect: map[string]string{"passwordauthentication": "yes"},
			want:   Fail,
		},
		{
			name:   "setting absent",
			expect: map[string]string{"kbdinteractiveauthentication": "no"},
			want:   Fail,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner.Fake{Answers: []runner.Canned{
				{Match: "sshd -T", Result: runner.Result{Stdout: sshdOutput}},
			}}

			got := SSHD{Expect: tc.expect}.Run(context.Background(), r)
			if got.Status != tc.want {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Detail, tc.want)
			}
		})
	}
}

func TestUsersCheck(t *testing.T) {
	cases := []struct {
		name    string
		stdout  string
		allowed []string
		want    Status
		detail  string
	}{
		{name: "only expected", stdout: "deploy\n", allowed: []string{"deploy"}, want: Pass},
		{name: "stranger appeared", stdout: "deploy\nbackdoor\n", allowed: []string{"deploy"}, want: Fail, detail: "backdoor"},
		{name: "empty host", stdout: "\n", allowed: []string{"deploy"}, want: Pass},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner.Fake{Answers: []runner.Canned{
				{Match: "/etc/passwd", Result: runner.Result{Stdout: tc.stdout}},
			}}

			got := Users{Allowed: tc.allowed}.Run(context.Background(), r)
			if got.Status != tc.want {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Detail, tc.want)
			}
			if tc.detail != "" && !strings.Contains(got.Detail, tc.detail) {
				t.Errorf("detail = %q, want it to name %q", got.Detail, tc.detail)
			}
		})
	}
}

func TestServicesCheck(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
		want   Status
	}{
		{name: "all active", stdout: "active\nactive\n", want: Pass},
		{name: "one down", stdout: "active\nfailed\n", want: Fail},
		{name: "systemd answered for fewer units", stdout: "active\n", want: Error},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// systemctl exits non-zero when any unit is not active, which is why
			// the check reads the output instead: the exit code cannot say which.
			r := &runner.Fake{Answers: []runner.Canned{
				{Match: "systemctl is-active", Result: runner.Result{Stdout: tc.stdout, ExitCode: 3}},
			}}

			got := Services{Names: []string{"docker", "tailscaled"}}.Run(context.Background(), r)
			if got.Status != tc.want {
				t.Fatalf("status = %s (%s / %s), want %s", got.Status, got.Detail, got.Error, tc.want)
			}
		})
	}
}

func TestBackupAgeCheck(t *testing.T) {
	now := time.Now().UTC().Unix()

	cases := []struct {
		name   string
		stdout string
		want   Status
	}{
		{name: "fresh", stdout: line(now, now-3600), want: Pass},
		{name: "stale", stdout: line(now, now-(40*3600)), want: Fail},
		{name: "never ran", stdout: strconv.FormatInt(now, 10) + "\n", want: Fail},
		{name: "stamp in the future", stdout: line(now, now+7200), want: Fail},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner.Fake{Answers: []runner.Canned{
				{Match: "date -u", Result: runner.Result{Stdout: tc.stdout}},
			}}

			got := BackupAge{StampPath: "/var/lib/baseline/last-backup", MaxAge: 26 * time.Hour}.
				Run(context.Background(), r)
			if got.Status != tc.want {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Detail, tc.want)
			}
		})
	}
}

// line renders what "date -u +%s; cat stamp" prints on the target.
func line(now, stamp int64) string {
	return strconv.FormatInt(now, 10) + "\n" + strconv.FormatInt(stamp, 10) + "\n"
}

func TestRunAllKeepsOrderAndTimes(t *testing.T) {
	r := &runner.Fake{Answers: []runner.Canned{
		{Match: "sshd -T", Result: runner.Result{Stdout: sshdOutput}},
		{Match: "/etc/passwd", Result: runner.Result{Stdout: "deploy\n"}},
	}}

	checks := []Check{
		SSHD{Expect: map[string]string{"permitrootlogin": "no"}},
		Users{Allowed: []string{"deploy"}},
	}

	results := RunAll(context.Background(), r, checks, 4)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].ID != "sshd" || results[1].ID != "users" {
		t.Errorf("results came back as %s then %s, want the order the checks were given",
			results[0].ID, results[1].ID)
	}
}
