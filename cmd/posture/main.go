// Command posture checks whether a server is in the state you think it is in,
// and writes down the evidence either way.
//
//	posture -host 203.0.113.10 -user deploy -key ~/.ssh/id_ed25519 -config posture.json
//	posture -local -config posture.json -json evidence/posture.json
//
// It exits 0 when every check passes, 1 when any check fails or could not run,
// and 2 when it could not start at all. That makes it usable in CI without
// anyone having to read the output.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mjhanzaibmemon/posture/internal/check"
	"github.com/mjhanzaibmemon/posture/internal/report"
	"github.com/mjhanzaibmemon/posture/internal/runner"
)

// Config is what the host is supposed to look like.
type Config struct {
	Services         []string          `json:"services"`
	AllowedUsers     []string          `json:"allowed_users"`
	AllowedListeners []string          `json:"allowed_listeners"`
	PrivatePrefixes  []string          `json:"private_prefixes"`
	SSHDExpect       map[string]string `json:"sshd_expect"`
	BackupStampPath  string            `json:"backup_stamp_path"`
	BackupMaxAge     string            `json:"backup_max_age"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "posture:", err)
		os.Exit(2)
	}
}

func run() error {
	var (
		host       = flag.String("host", "", "host to check over ssh")
		user       = flag.String("user", "", "ssh user")
		key        = flag.String("key", "", "ssh private key file")
		local      = flag.Bool("local", false, "check this machine instead of a remote host")
		configPath = flag.String("config", "posture.json", "expected state of the host")
		jsonPath   = flag.String("json", "", "write the full report, evidence included, to this file")
		parallel   = flag.Int("parallel", 4, "how many checks may run at once")
		timeout    = flag.Duration("timeout", 2*time.Minute, "give up after this long")
	)
	flag.Parse()

	if !*local && *host == "" {
		return fmt.Errorf("give me a -host to check, or -local to check this machine")
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}

	checks, err := buildChecks(cfg)
	if err != nil {
		return err
	}

	var r runner.Runner = runner.Local{}
	if !*local {
		r = runner.SSH{Host: *host, User: *user, KeyPath: *key}
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	started := time.Now()
	results := check.RunAll(ctx, r, checks, *parallel)
	rep := report.New(r.Target(), started, time.Now(), results)

	if err := rep.Text(os.Stdout); err != nil {
		return err
	}

	if *jsonPath != "" {
		f, err := os.Create(*jsonPath)
		if err != nil {
			return fmt.Errorf("writing the report: %w", err)
		}
		defer f.Close()

		if err := rep.JSON(f); err != nil {
			return fmt.Errorf("writing the report: %w", err)
		}
	}

	if !rep.OK() {
		os.Exit(1)
	}
	return nil
}

func loadConfig(path string) (Config, error) {
	var cfg Config

	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

func buildChecks(cfg Config) ([]check.Check, error) {
	var checks []check.Check

	if len(cfg.SSHDExpect) > 0 {
		checks = append(checks, check.SSHD{Expect: cfg.SSHDExpect})
	}
	if len(cfg.AllowedUsers) > 0 {
		checks = append(checks, check.Users{Allowed: cfg.AllowedUsers})
	}
	if len(cfg.AllowedListeners) > 0 {
		checks = append(checks, check.Listeners{
			Allowed:         cfg.AllowedListeners,
			PrivatePrefixes: cfg.PrivatePrefixes,
		})
	}
	if len(cfg.Services) > 0 {
		checks = append(checks, check.Services{Names: cfg.Services})
	}
	if cfg.BackupStampPath != "" {
		maxAge := 26 * time.Hour
		if cfg.BackupMaxAge != "" {
			parsed, err := time.ParseDuration(cfg.BackupMaxAge)
			if err != nil {
				return nil, fmt.Errorf("backup_max_age: %w", err)
			}
			maxAge = parsed
		}
		checks = append(checks, check.BackupAge{StampPath: cfg.BackupStampPath, MaxAge: maxAge})
	}

	if len(checks) == 0 {
		return nil, fmt.Errorf("the config asks for nothing, so there is nothing to check")
	}
	return checks, nil
}
