# posture

A small Go tool that checks whether a server is in the state you think it is in,
and writes down the evidence either way.

It answers five questions about a host: is sshd configured the way you believe,
are there accounts nobody added on purpose, is anything listening where it should
not be, are the services you depend on running, and did a backup actually
complete recently. Then it prints a verdict and, if you ask, a JSON report
containing the raw command output behind every line of it.

## Why it exists

I kept writing the same shell script for different servers, and every version
drifted. Worse, the shell versions were hard to test, so the checks themselves
were never checked. A wrong check is more dangerous than no check: it tells you
the house is locked while the back door is open.

Two decisions follow from that.

A check reports PASS, FAIL or ERROR, and ERROR is not FAIL. "The host is not in
the expected state" and "I could not find out" are different facts, and folding
the second into the first is how monitoring quietly starts lying.

Every result carries the raw output it was derived from. A green tick nobody can
reproduce is an opinion.

## Install

    go install github.com/mjhanzaibmemon/posture/cmd/posture@latest

Or build it from a clone:

    go build ./cmd/posture

## Use

    cp posture.example.json posture.json
    posture -host 203.0.113.10 -user deploy -key ~/.ssh/id_ed25519
    posture -local -json evidence/posture.json

Exit codes are 0 when everything passed, 1 when something failed or could not be
determined, and 2 when the tool could not start. That makes it usable in CI
without anyone reading the output.

A real run, deliberately taken on a machine that is missing most of what the
checks look for, because it shows the distinction the tool is built around:

    target: localhost

    ERROR  users       reading /etc/passwd exited 2: awk: fatal: cannot open file `/etc/passwd'  308ms
    ERROR  services    asked about 1 units, systemd answered for 0                               180ms
    PASS   backup_age  last backup 1s ago                                                        302ms

    passed 1, failed 0, errors 2, in 309ms

Nothing here failed. Two things could not be determined, and the exit code is 1
either way, because "I could not check" is not a pass.

## Config

`posture.json` describes what the host is supposed to look like. Everything is
optional: leave a section out and that check does not run.

| Key | Meaning |
|---|---|
| `sshd_expect` | Settings and required values, in `sshd -T` spelling |
| `allowed_users` | Accounts with UID 1000 and above that are expected |
| `allowed_listeners` | Sockets allowed to face the outside, as `port/proto` |
| `private_prefixes` | Extra CIDRs to treat as private, beyond loopback and Tailscale |
| `services` | systemd units that must be active |
| `backup_stamp_path` | File the backup job writes a unix timestamp into on success |
| `backup_max_age` | How old that stamp may be, as a Go duration such as `26h` |

## About the listener check

Listing sockets is easy. Deciding which ones matter is the part that goes wrong.

A socket on loopback cannot be reached from anywhere else. A socket bound to a
Tailscale address can only be reached by devices already on that tailnet, and
Tailscale binds a random high port on each of its own addresses, so matching
those by port number would never hold still. Both are filtered out before the
allowlist is applied, which is why the allowlist stays short enough to be read.

Everything else, including a wildcard bind such as `0.0.0.0`, counts as exposure
and has to be listed on purpose.

## Testing

    go test ./...

The checks talk to a `Runner` interface rather than to SSH, so the tests hand
them recorded output from real machines and never touch a network. The listener
fixture is genuine `ss -tulpnH` output from a hardened host, because fixtures
invented to match a parser agree with it by construction and then fail on the
first real server.

## Licence

See LICENSE. Readable so you can judge the work, not open source.
