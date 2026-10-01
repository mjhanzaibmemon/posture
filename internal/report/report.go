// Package report turns check results into something a person can read and
// something a pipeline can keep.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/mjhanzaibmemon/posture/internal/check"
)

// Summary counts outcomes.
type Summary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	Errors int `json:"errors"`
}

// Report is a complete run, including the evidence each check produced. It is
// written to disk as JSON so a reviewer can reproduce the conclusion instead of
// trusting a green tick.
type Report struct {
	Tool       string         `json:"tool"`
	Target     string         `json:"target"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	Summary    Summary        `json:"summary"`
	Results    []check.Result `json:"results"`
}

// New builds a report and counts the outcomes.
func New(target string, started, finished time.Time, results []check.Result) Report {
	s := Summary{Total: len(results)}
	for _, r := range results {
		switch r.Status {
		case check.Pass:
			s.Passed++
		case check.Fail:
			s.Failed++
		default:
			s.Errors++
		}
	}

	return Report{
		Tool:       "posture",
		Target:     target,
		StartedAt:  started,
		FinishedAt: finished,
		Summary:    s,
		Results:    results,
	}
}

// OK reports whether everything passed. An error counts as not OK, because a
// check that could not run has not told you the host is healthy.
func (r Report) OK() bool { return r.Summary.Failed == 0 && r.Summary.Errors == 0 }

// Text writes the human readable form, without the raw evidence.
func (r Report) Text(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "target: %s\n\n", r.Target); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, res := range r.Results {
		detail := res.Detail
		if res.Status == check.Error {
			detail = res.Error
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%dms\n", res.Status, res.ID, detail, res.DurationMS); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	_, err := fmt.Fprintf(w, "\npassed %d, failed %d, errors %d, in %s\n",
		r.Summary.Passed, r.Summary.Failed, r.Summary.Errors,
		r.FinishedAt.Sub(r.StartedAt).Round(time.Millisecond))
	return err
}

// JSON writes the full report, evidence included.
func (r Report) JSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
