package discovery

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
)

// Reconciler is the only thing the scanner needs from the device package.
// Defined here so tests can supply a fake.
type Reconciler interface {
	Reconcile(ctx context.Context, obs device.Observation) (string, error)
}

// Source produces a snapshot of network observations on demand. Snapshot must
// be safe to call concurrently with itself once per scan interval.
type Source interface {
	Name() string
	Snapshot(ctx context.Context) ([]device.Observation, error)
}

type Scanner struct {
	Reconciler Reconciler
	Sources    []Source
	Interval   time.Duration
	MinCycles  int             // device must appear in this many cycles before Reconcile is called
	HideMACs   map[string]bool // normalized MACs to suppress (self interfaces, optionally gateway)
	Logger     *slog.Logger

	seen map[string]int
	mu   sync.Mutex
}

func (s *Scanner) Run(ctx context.Context) error {
	if s.Interval <= 0 {
		s.Interval = 30 * time.Second
	}
	if s.MinCycles <= 0 {
		s.MinCycles = 2
	}
	s.mu.Lock()
	if s.seen == nil {
		s.seen = map[string]int{}
	}
	s.mu.Unlock()

	s.cycle(ctx)

	t := time.NewTicker(s.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			s.cycle(ctx)
		}
	}
}

// Tick runs a single scan cycle. Exported for tests.
func (s *Scanner) Tick(ctx context.Context) { s.cycle(ctx) }

func (s *Scanner) cycle(ctx context.Context) {
	s.mu.Lock()
	if s.seen == nil {
		s.seen = map[string]int{}
	}
	s.mu.Unlock()

	blended := map[string]device.Observation{} // normalized MAC -> merged obs
	var enrichers []device.Observation         // observations without a MAC (e.g. mDNS)

	for _, src := range s.Sources {
		obs, err := src.Snapshot(ctx)
		if err != nil {
			s.log("source snapshot failed", "source", src.Name(), "err", err)
			continue
		}
		for _, o := range obs {
			if o.MAC == "" {
				enrichers = append(enrichers, o)
				continue
			}
			key := NormalizeMAC(o.MAC)
			if key == "" || IsNonHostMAC(key) {
				continue
			}
			o.MAC = key
			if existing, ok := blended[key]; ok {
				blended[key] = mergeObs(existing, o)
			} else {
				blended[key] = o
			}
		}
	}

	// Apply MAC-less observations as IP-keyed enrichment (mDNS hostname onto
	// an ARP entry sharing the same IP).
	for _, e := range enrichers {
		if e.IP == "" {
			continue
		}
		for mac, b := range blended {
			if b.IP == e.IP {
				if b.Hostname == "" && e.Hostname != "" {
					b.Hostname = e.Hostname
				}
				blended[mac] = b
			}
		}
	}

	now := time.Now().UTC()
	for mac, o := range blended {
		if s.HideMACs[mac] {
			continue
		}
		s.mu.Lock()
		s.seen[mac]++
		count := s.seen[mac]
		s.mu.Unlock()
		if count < s.MinCycles {
			continue
		}
		o.Source = "scan"
		if o.SeenAt.IsZero() {
			o.SeenAt = now
		}
		if _, err := s.Reconciler.Reconcile(ctx, o); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			s.log("reconcile failed", "mac", mac, "err", err)
		}
	}
}

func (s *Scanner) log(msg string, kv ...any) {
	if s.Logger != nil {
		s.Logger.Warn(msg, kv...)
	}
}

// NormalizeMAC lowercases and pads each octet of a MAC to two hex digits,
// returning "" if the input doesn't parse as a 6-octet MAC. The macOS arp
// command happily emits "1:2:3:4:5:6" without zero padding — without
// normalization we'd produce duplicate identities for the same device.
func NormalizeMAC(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || strings.Contains(s, "incomplete") {
		return ""
	}
	sep := ":"
	if !strings.Contains(s, ":") && strings.Contains(s, "-") {
		sep = "-"
	}
	parts := strings.Split(s, sep)
	if len(parts) != 6 {
		return ""
	}
	var b strings.Builder
	for i, p := range parts {
		if len(p) == 0 || len(p) > 2 {
			return ""
		}
		for _, c := range p {
			if !isHex(c) {
				return ""
			}
		}
		if i > 0 {
			b.WriteByte(':')
		}
		if len(p) == 1 {
			b.WriteByte('0')
		}
		b.WriteString(p)
	}
	return b.String()
}

func isHex(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}

// IsNonHostMAC reports whether the (normalized) MAC belongs to broadcast or
// link-layer multicast traffic — i.e. is not a real host. The ARP table
// commonly includes ff:ff:ff:ff:ff:ff (broadcast), 01:00:5e:* (IPv4 multicast),
// and 33:33:* (IPv6 multicast) entries; the scanner must not register them as
// devices.
func IsNonHostMAC(mac string) bool {
	if mac == "ff:ff:ff:ff:ff:ff" {
		return true
	}
	if strings.HasPrefix(mac, "01:00:5e:") {
		return true
	}
	if strings.HasPrefix(mac, "33:33:") {
		return true
	}
	return false
}

func mergeObs(a, b device.Observation) device.Observation {
	if a.IP == "" {
		a.IP = b.IP
	}
	if a.Hostname == "" {
		a.Hostname = b.Hostname
	}
	if a.AgentID == "" {
		a.AgentID = b.AgentID
	}
	if b.SeenAt.After(a.SeenAt) {
		a.SeenAt = b.SeenAt
	}
	return a
}
