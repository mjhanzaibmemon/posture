// Package check holds the individual posture checks and the machinery to run them.
//
// Every check answers one question about a host and returns evidence for its
// answer. A check that cannot explain itself is not worth running.
package check

import (
	"context"
	"sync"
	"time"

	"github.com/mjhanzaibmemon/posture/internal/runner"
)

// Status is the outcome of a single check.
type Status string

// The three outcomes. Fail and Error are deliberately different: Fail means the
// host is not in the expected state, Error means we could not find out. Treating
// "I could not tell" as "everything is fine" is how monitoring lies to people.
const (
	Pass  Status = "PASS"
	Fail  Status = "FAIL"
	Error Status = "ERROR"
)

// Result is one check's verdict plus the raw output that led to it.
type Result struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Status     Status `json:"status"`
	Detail     string `json:"detail,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// Check is one question asked of a host.
type Check interface {
	ID() string
	Title() string
	Run(ctx context.Context, r runner.Runner) Result
}

func pass(c Check, detail, evidence string) Result {
	return Result{ID: c.ID(), Title: c.Title(), Status: Pass, Detail: detail, Evidence: evidence}
}

func fail(c Check, detail, evidence string) Result {
	return Result{ID: c.ID(), Title: c.Title(), Status: Fail, Detail: detail, Evidence: evidence}
}

func failed(c Check, err error, evidence string) Result {
	return Result{ID: c.ID(), Title: c.Title(), Status: Error, Error: err.Error(), Evidence: evidence}
}

// RunAll runs every check and returns the results in the order the checks were
// given, regardless of which finished first. Deterministic output matters when
// the report ends up in a pull request or an evidence archive.
//
// parallel caps how many run at once. Zero or less means run them one at a time.
func RunAll(ctx context.Context, r runner.Runner, checks []Check, parallel int) []Result {
	results := make([]Result, len(checks))

	if parallel < 1 {
		parallel = 1
	}
	slots := make(chan struct{}, parallel)

	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func(i int, c Check) {
			defer wg.Done()

			slots <- struct{}{}
			defer func() { <-slots }()

			started := time.Now()
			res := c.Run(ctx, r)
			res.DurationMS = time.Since(started).Milliseconds()
			results[i] = res
		}(i, c)
	}
	wg.Wait()

	return results
}
