package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mjhanzaibmemon/posture/internal/runner"
)

// SSHD asserts that the running sshd is configured the way you think it is.
//
// It reads the effective configuration from the daemon with "sshd -T" rather
// than from sshd_config on disk. Those two disagree more often than people
// expect, because of drop-in files, Match blocks and edits nobody restarted.
type SSHD struct {
	// Expect maps a setting to its required value, in sshd -T's own lowercase
	// spelling, for example "permitrootlogin": "no".
	Expect map[string]string
}

// ID identifies the check.
func (c SSHD) ID() string { return "sshd" }

// Title describes the check.
func (c SSHD) Title() string { return "sshd runtime configuration" }

// Run reads the effective sshd configuration and compares it with Expect.
func (c SSHD) Run(ctx context.Context, r runner.Runner) Result {
	out, err := r.Run(ctx, "sudo sshd -T")
	if err != nil {
		return failed(c, err, out.Stderr)
	}
	if out.ExitCode != 0 {
		return failed(c, fmt.Errorf("sshd -T exited %d: %s", out.ExitCode, strings.TrimSpace(out.Stderr)), out.Stdout)
	}

	settings := parseSSHD(out.Stdout)

	var problems []string
	for key, want := range c.Expect {
		key = strings.ToLower(key)

		got, ok := settings[key]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is not set, expected %q", key, want))
			continue
		}
		if !strings.EqualFold(got, want) {
			problems = append(problems, fmt.Sprintf("%s is %q, expected %q", key, got, want))
		}
	}
	sort.Strings(problems)

	if len(problems) > 0 {
		return fail(c, strings.Join(problems, "; "), out.Stdout)
	}
	return pass(c, fmt.Sprintf("%d settings as expected", len(c.Expect)), out.Stdout)
}

// parseSSHD turns "sshd -T" output into a map. The daemon prints one setting per
// line as "keyword value", with keywords already lowercased. Some keywords appear
// more than once, such as hostkey, and for those the last line wins.
func parseSSHD(raw string) map[string]string {
	settings := make(map[string]string)

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		key, value, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		settings[strings.ToLower(key)] = strings.TrimSpace(value)
	}
	return settings
}
