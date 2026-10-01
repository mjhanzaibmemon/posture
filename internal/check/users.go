package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mjhanzaibmemon/posture/internal/runner"
)

// Users asserts that only the accounts you expect can log in.
//
// It looks at human accounts, meaning UID 1000 and above, and ignores the
// service accounts a distribution creates for itself. An account that appeared
// without a corresponding change in configuration management is the interesting
// case: it means somebody made it by hand, and nobody wrote it down.
type Users struct {
	Allowed []string
}

// ID identifies the check.
func (c Users) ID() string { return "users" }

// Title describes the check.
func (c Users) Title() string { return "no unexpected user accounts" }

const humanUsersCommand = `awk -F: '$3 >= 1000 && $3 < 65534 {print $1}' /etc/passwd`

// Run lists human accounts and compares them with Allowed.
func (c Users) Run(ctx context.Context, r runner.Runner) Result {
	out, err := r.Run(ctx, humanUsersCommand)
	if err != nil {
		return failed(c, err, out.Stderr)
	}
	if out.ExitCode != 0 {
		return failed(c, fmt.Errorf("reading /etc/passwd exited %d: %s", out.ExitCode, strings.TrimSpace(out.Stderr)), out.Stdout)
	}

	allowed := make(map[string]bool, len(c.Allowed))
	for _, a := range c.Allowed {
		allowed[strings.TrimSpace(a)] = true
	}

	var found, unexpected []string
	for _, line := range strings.Split(out.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		found = append(found, name)
		if !allowed[name] {
			unexpected = append(unexpected, name)
		}
	}
	sort.Strings(unexpected)

	if len(unexpected) > 0 {
		return fail(c, "unexpected accounts: "+strings.Join(unexpected, ", "), out.Stdout)
	}
	return pass(c, fmt.Sprintf("%d account(s), all expected", len(found)), out.Stdout)
}
