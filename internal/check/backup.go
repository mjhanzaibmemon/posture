package check

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mjhanzaibmemon/posture/internal/runner"
)

// BackupAge asserts that a backup ran recently enough to be worth having.
//
// It reads a stamp file that the backup job writes on success. Checking the job
// exists, or that a timer is enabled, proves nothing: timers fail quietly and
// units can succeed while the backup inside them did nothing. A timestamp
// written only after a successful run is the smallest honest signal.
type BackupAge struct {
	StampPath string
	MaxAge    time.Duration
}

// ID identifies the check.
func (c BackupAge) ID() string { return "backup_age" }

// Title describes the check.
func (c BackupAge) Title() string { return "a backup completed recently" }

// Run compares the stamp with the host's own clock.
func (c BackupAge) Run(ctx context.Context, r runner.Runner) Result {
	// Both numbers come from the target so that clock skew between the machine
	// running posture and the machine being checked cannot invent a problem,
	// or worse, hide one.
	command := fmt.Sprintf("date -u +%%s; cat %s", c.StampPath)

	out, err := r.Run(ctx, command)
	if err != nil {
		return failed(c, err, out.Stderr)
	}

	fields := strings.Fields(out.Stdout)
	if len(fields) < 2 {
		return fail(c, fmt.Sprintf("no usable stamp at %s, so no backup has been recorded", c.StampPath), out.Stdout+out.Stderr)
	}

	now, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return failed(c, fmt.Errorf("reading the host clock: %w", err), out.Stdout)
	}
	stamp, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return failed(c, fmt.Errorf("reading %s: %w", c.StampPath, err), out.Stdout)
	}

	age := time.Duration(now-stamp) * time.Second
	switch {
	case age < 0:
		return fail(c, fmt.Sprintf("the stamp is %s in the future, which means a clock problem", (-age).Round(time.Second)), out.Stdout)
	case age > c.MaxAge:
		return fail(c, fmt.Sprintf("last backup was %s ago, limit is %s", age.Round(time.Second), c.MaxAge), out.Stdout)
	default:
		return pass(c, fmt.Sprintf("last backup %s ago", age.Round(time.Second)), out.Stdout)
	}
}
