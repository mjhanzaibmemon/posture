package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mjhanzaibmemon/posture/internal/check"
)

func sample() Report {
	started := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	return New("deploy@198.51.100.7", started, started.Add(1200*time.Millisecond), []check.Result{
		{ID: "sshd", Title: "sshd runtime configuration", Status: check.Pass, Detail: "3 settings as expected"},
		{ID: "users", Title: "no unexpected user accounts", Status: check.Fail, Detail: "unexpected accounts: backdoor"},
		{ID: "listeners", Title: "no unexpected listening sockets", Status: check.Error, Error: "ss exited 127"},
	})
}

func TestSummaryCounts(t *testing.T) {
	r := sample()

	if r.Summary.Total != 3 || r.Summary.Passed != 1 || r.Summary.Failed != 1 || r.Summary.Errors != 1 {
		t.Fatalf("summary = %+v, want 3 total with one of each", r.Summary)
	}
}

func TestNotOKWhenACheckOnlyErrored(t *testing.T) {
	r := New("host", time.Now(), time.Now(), []check.Result{
		{ID: "listeners", Status: check.Error, Error: "ss exited 127"},
	})

	if r.OK() {
		t.Error("a check that could not run must not count as a healthy host")
	}
}

func TestTextMentionsEveryCheck(t *testing.T) {
	var buf bytes.Buffer
	if err := sample().Text(&buf); err != nil {
		t.Fatalf("Text: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"sshd", "users", "listeners", "backdoor", "ss exited 127", "passed 1, failed 1, errors 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output is missing %q:\n%s", want, out)
		}
	}
}

func TestJSONRoundTrips(t *testing.T) {
	var buf bytes.Buffer
	if err := sample().JSON(&buf); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var back Report
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatalf("the report did not survive a round trip: %v", err)
	}
	if back.Target != "deploy@198.51.100.7" || len(back.Results) != 3 {
		t.Errorf("decoded report = %+v, want the same target and three results", back)
	}
}
