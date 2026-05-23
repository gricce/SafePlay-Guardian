package discovery

import (
	"context"

	"github.com/gricce/SafePlay-Guardian/internal/device"
)

// MDNSResult is one mDNS browse hit: a hostname plus an IPv4 address.
type MDNSResult struct {
	Host string
	IP   string
}

// LookupFunc performs a short mDNS browse and returns results. Pluggable so
// the production binary can wire in a real library (hashicorp/mdns,
// grandcat/zeroconf) without this package depending on it directly. Tests pass
// in a stub.
type LookupFunc func(ctx context.Context) ([]MDNSResult, error)

// MDNSSource emits hostname-only observations (no MAC). The Scanner correlates
// them by IP onto ARP-keyed MAC observations later in the same cycle.
type MDNSSource struct {
	Lookup LookupFunc
}

func (MDNSSource) Name() string { return "mdns" }

func (m MDNSSource) Snapshot(ctx context.Context) ([]device.Observation, error) {
	if m.Lookup == nil {
		return nil, nil
	}
	results, err := m.Lookup(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]device.Observation, 0, len(results))
	for _, r := range results {
		if r.Host == "" || r.IP == "" {
			continue
		}
		out = append(out, device.Observation{Hostname: r.Host, IP: r.IP})
	}
	return out, nil
}
