// Package runner executes a shell command somewhere and reports what it printed.
//
// Checks depend on the Runner interface rather than on SSH directly. That is the
// whole reason the checks can be tested without a server: tests hand them a Fake.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// Result is what a command left behind. A non-zero ExitCode is data, not an
// error: plenty of useful commands exit non-zero and still tell us what we need.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner runs a command against some target.
type Runner interface {
	Run(ctx context.Context, command string) (Result, error)
	Target() string
}

// SSH runs commands on a remote host by shelling out to the ssh binary.
//
// Shelling out instead of using a Go SSH library keeps this dependency free, and
// it means the tool honours whatever ssh_config, agent and known_hosts setup the
// operator already has. Reimplementing that badly is a good way to be surprised.
type SSH struct {
	Host    string
	User    string
	KeyPath string
}

// Target is the user@host the checks are running against.
func (s SSH) Target() string {
	if s.User == "" {
		return s.Host
	}
	return s.User + "@" + s.Host
}

// Run executes command on the remote host.
func (s SSH) Run(ctx context.Context, command string) (Result, error) {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if s.KeyPath != "" {
		args = append(args, "-i", s.KeyPath)
	}
	args = append(args, s.Target(), command)

	return run(ctx, "ssh", args...)
}

// Local runs commands on this machine. Useful when posture runs on the host it
// is checking, for example from a systemd timer or inside CI.
type Local struct{}

// Target identifies the local machine.
func (Local) Target() string { return "localhost" }

// Run executes command through a shell.
func (Local) Run(ctx context.Context, command string) (Result, error) {
	return run(ctx, "sh", "-c", command)
}

func run(ctx context.Context, name string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("running %s: %w", name, err)
	}
	return res, nil
}

// Canned is one scripted answer for the Fake runner.
type Canned struct {
	// Match is a substring of the command. The first Canned whose Match appears
	// in the command wins, so order matters and the list stays deterministic.
	Match  string
	Result Result
	Err    error
}

// Fake is a Runner for tests. It never touches a network.
//
// Checks are run concurrently, so a shared Fake is written to from several
// goroutines at once. The mutex is not decoration: without it the race detector
// fails the build, and it is right to.
type Fake struct {
	Answers []Canned

	mu sync.Mutex
	// calls records every command it was asked to run, so a test can assert on
	// what a check actually did rather than only on what it returned.
	calls []string
}

// Target identifies the fake.
func (f *Fake) Target() string { return "fake" }

// Run returns the first scripted answer matching the command.
func (f *Fake) Run(_ context.Context, command string) (Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, command)
	f.mu.Unlock()

	for _, a := range f.Answers {
		if strings.Contains(command, a.Match) {
			return a.Result, a.Err
		}
	}
	return Result{ExitCode: 127, Stderr: "fake runner has no answer for: " + command}, nil
}

// Calls returns a copy of the commands this Fake was asked to run.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.calls...)
}
