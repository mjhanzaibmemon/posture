package check

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/mjhanzaibmemon/posture/internal/runner"
)

// Listeners asserts that nothing is listening where it should not be.
//
// The question is not "what sockets exist" but "what can be reached from
// outside". A socket on loopback is the host's own business, and one bound to a
// private overlay address can only be reached by devices already on that
// network. Counting those as exposure produces noise, and a check that cries
// wolf gets switched off.
type Listeners struct {
	// Allowed lists sockets that may face the outside world, as "port/proto",
	// for example "22/tcp".
	Allowed []string

	// PrivatePrefixes are extra CIDRs to treat as private, on top of loopback.
	// Tailscale's ranges are the default because that is the common case.
	PrivatePrefixes []string
}

// ID identifies the check.
func (c Listeners) ID() string { return "listeners" }

// Title describes the check.
func (c Listeners) Title() string { return "no unexpected listening sockets" }

// Socket is one listening socket as reported by ss.
type Socket struct {
	Proto   string
	Address string
	Port    string
}

// PortProto renders the socket the way the allowlist is written.
func (s Socket) PortProto() string { return s.Port + "/" + s.Proto }

var defaultPrivatePrefixes = []string{
	"127.0.0.0/8",         // loopback
	"::1/128",             // loopback
	"100.64.0.0/10",       // CGNAT, which is what Tailscale hands out
	"fd7a:115c:a1e0::/48", // Tailscale's IPv6 range
}

// Run lists sockets and compares the externally reachable ones with Allowed.
func (c Listeners) Run(ctx context.Context, r runner.Runner) Result {
	out, err := r.Run(ctx, "sudo ss -tulpnH")
	if err != nil {
		return failed(c, err, out.Stderr)
	}
	if out.ExitCode != 0 {
		return failed(c, fmt.Errorf("ss exited %d: %s", out.ExitCode, strings.TrimSpace(out.Stderr)), out.Stdout)
	}

	prefixes := append(append([]string{}, defaultPrivatePrefixes...), c.PrivatePrefixes...)

	exposed, err := exposedSockets(parseListeners(out.Stdout), prefixes)
	if err != nil {
		return failed(c, err, out.Stdout)
	}

	allowed := make(map[string]bool, len(c.Allowed))
	for _, a := range c.Allowed {
		allowed[strings.TrimSpace(a)] = true
	}

	seen := make(map[string]bool)
	var unexpected []string
	for _, s := range exposed {
		key := s.PortProto()
		if allowed[key] || seen[key] {
			continue
		}
		seen[key] = true
		unexpected = append(unexpected, key)
	}
	sort.Strings(unexpected)

	if len(unexpected) > 0 {
		return fail(c, "reachable from outside and not allowed: "+strings.Join(unexpected, ", "), out.Stdout)
	}
	return pass(c, fmt.Sprintf("%d externally reachable socket(s), all allowed", len(exposed)), out.Stdout)
}

// parseListeners reads "ss -tulpnH" output. Each line starts with the protocol
// and carries the local address in the fifth field, which is why this does not
// try to be clever about the rest.
func parseListeners(raw string) []Socket {
	var sockets []Socket

	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		proto := fields[0]
		if proto != "tcp" && proto != "udp" {
			continue
		}

		local := fields[4]
		idx := strings.LastIndex(local, ":")
		if idx < 0 {
			continue
		}

		address := local[:idx]
		port := local[idx+1:]
		if port == "" {
			continue
		}

		// ss writes the interface it is bound to as "addr%iface", and IPv6
		// addresses arrive wrapped in brackets.
		if cut := strings.Index(address, "%"); cut >= 0 {
			address = address[:cut]
		}
		address = strings.TrimPrefix(address, "[")
		address = strings.TrimSuffix(address, "]")

		sockets = append(sockets, Socket{Proto: proto, Address: address, Port: port})
	}
	return sockets
}

// exposedSockets drops everything bound to an address that cannot be reached
// from outside the host or its private network.
func exposedSockets(sockets []Socket, privatePrefixes []string) ([]Socket, error) {
	parsed := make([]netip.Prefix, 0, len(privatePrefixes))
	for _, p := range privatePrefixes {
		prefix, err := netip.ParsePrefix(p)
		if err != nil {
			return nil, fmt.Errorf("private prefix %q: %w", p, err)
		}
		parsed = append(parsed, prefix)
	}

	var exposed []Socket
	for _, s := range sockets {
		addr, err := netip.ParseAddr(s.Address)
		if err != nil {
			// A wildcard bind such as "*" is not an address, and it means the
			// socket answers on every interface. That is the opposite of private.
			exposed = append(exposed, s)
			continue
		}

		if addr.IsLoopback() {
			continue
		}

		private := false
		for _, prefix := range parsed {
			if prefix.Contains(addr) {
				private = true
				break
			}
		}
		if !private {
			exposed = append(exposed, s)
		}
	}
	return exposed, nil
}
