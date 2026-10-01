package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/mjhanzaibmemon/posture/internal/runner"
)

// Services asserts that the units you depend on are actually running.
type Services struct {
	Names []string
}

// ID identifies the check.
func (c Services) ID() string { return "services" }

// Title describes the check.
func (c Services) Title() string { return "expected services are active" }

// Run asks systemd about every unit in one call.
func (c Services) Run(ctx context.Context, r runner.Runner) Result {
	if len(c.Names) == 0 {
		return pass(c, "no services configured", "")
	}

	// systemctl prints one state per unit, in the order asked, and exits
	// non-zero when any of them is not active. The exit code alone does not say
	// which one, so the output is what matters here.
	out, err := r.Run(ctx, "systemctl is-active "+strings.Join(c.Names, " "))
	if err != nil {
		return failed(c, err, out.Stderr)
	}

	states := strings.Fields(out.Stdout)
	if len(states) != len(c.Names) {
		return failed(c, fmt.Errorf("asked about %d units, systemd answered for %d", len(c.Names), len(states)), out.Stdout)
	}

	var problems []string
	for i, name := range c.Names {
		if states[i] != "active" {
			problems = append(problems, fmt.Sprintf("%s is %s", name, states[i]))
		}
	}

	if len(problems) > 0 {
		return fail(c, strings.Join(problems, "; "), out.Stdout)
	}
	return pass(c, fmt.Sprintf("%d service(s) active", len(c.Names)), out.Stdout)
}
