package discovery_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/discovery"
)

type fakeReconciler struct {
	mu   sync.Mutex
	seen []device.Observation
}

func (f *fakeReconciler) Reconcile(_ context.Context, obs device.Observation) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, obs)
	return "id-" + obs.MAC, nil
}

func (f *fakeReconciler) calls() []device.Observation {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]device.Observation, len(f.seen))
	copy(out, f.seen)
	return out
}

type fixedSource struct {
	name string
	obs  []device.Observation
}

func (f fixedSource) Name() string                                      { return f.name }
func (f fixedSource) Snapshot(_ context.Context) ([]device.Observation, error) { return f.obs, nil }

func TestScannerPromotesAfterMinCycles(t *testing.T) {
	rec := &fakeReconciler{}
	src := fixedSource{name: "arp", obs: []device.Observation{
		{MAC: "AA:BB:CC:DD:EE:FF", IP: "192.168.1.10"},
	}}
	s := &discovery.Scanner{
		Reconciler: rec,
		Sources:    []discovery.Source{src},
		Interval:   time.Hour, // never auto-tick
		MinCycles:  3,
	}

	for i := 0; i < 2; i++ {
		s.Tick(context.Background())
	}
	if len(rec.calls()) != 0 {
		t.Fatalf("scanner promoted before MinCycles: %d calls", len(rec.calls()))
	}
	s.Tick(context.Background())
	if len(rec.calls()) != 1 {
		t.Fatalf("scanner should promote on cycle %d, got %d calls", 3, len(rec.calls()))
	}
	if got := rec.calls()[0].MAC; got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("MAC not normalized: %s", got)
	}
}

func TestScannerHidesSelfMAC(t *testing.T) {
	rec := &fakeReconciler{}
	src := fixedSource{name: "arp", obs: []device.Observation{
		{MAC: "AA:BB:CC:DD:EE:FF", IP: "192.168.1.10"},
		{MAC: "11:22:33:44:55:66", IP: "192.168.1.11"},
	}}
	s := &discovery.Scanner{
		Reconciler: rec,
		Sources:    []discovery.Source{src},
		Interval:   time.Hour,
		MinCycles:  1,
		HideMACs:   map[string]bool{"aa:bb:cc:dd:ee:ff": true},
	}
	s.Tick(context.Background())
	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 promoted device after filtering, got %d", len(calls))
	}
	if calls[0].MAC != "11:22:33:44:55:66" {
		t.Fatalf("wrong MAC promoted: %s", calls[0].MAC)
	}
}

func TestScannerBlendsMDNSHostnameByIP(t *testing.T) {
	rec := &fakeReconciler{}
	arp := fixedSource{name: "arp", obs: []device.Observation{
		{MAC: "AA:BB:CC:DD:EE:FF", IP: "192.168.1.10"},
	}}
	mdns := fixedSource{name: "mdns", obs: []device.Observation{
		{IP: "192.168.1.10", Hostname: "alice-phone"},
	}}
	s := &discovery.Scanner{
		Reconciler: rec,
		Sources:    []discovery.Source{arp, mdns},
		Interval:   time.Hour,
		MinCycles:  1,
	}
	s.Tick(context.Background())
	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 promoted device, got %d", len(calls))
	}
	if calls[0].Hostname != "alice-phone" {
		t.Fatalf("mdns hostname not blended into ARP obs: %+v", calls[0])
	}
}

func TestScannerMergesDuplicateMACsAcrossSources(t *testing.T) {
	rec := &fakeReconciler{}
	a := fixedSource{name: "arp", obs: []device.Observation{
		{MAC: "AA:BB:CC:DD:EE:FF", IP: "192.168.1.10"},
	}}
	b := fixedSource{name: "sweep", obs: []device.Observation{
		{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", Hostname: "from-sweep"},
	}}
	s := &discovery.Scanner{
		Reconciler: rec,
		Sources:    []discovery.Source{a, b},
		Interval:   time.Hour,
		MinCycles:  1,
	}
	s.Tick(context.Background())
	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("two sources for same MAC should produce one obs, got %d", len(calls))
	}
}

func TestScannerSkipsBroadcastAndMulticast(t *testing.T) {
	rec := &fakeReconciler{}
	src := fixedSource{name: "arp", obs: []device.Observation{
		{MAC: "FF:FF:FF:FF:FF:FF", IP: "192.168.1.255"},
		{MAC: "01:00:5e:00:00:fb", IP: "224.0.0.251"},
		{MAC: "33:33:00:00:00:fb", IP: "ff02::fb"},
		{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10"},
	}}
	s := &discovery.Scanner{
		Reconciler: rec, Sources: []discovery.Source{src},
		Interval: time.Hour, MinCycles: 1,
	}
	s.Tick(context.Background())
	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("expected only the real host to be promoted, got %d", len(calls))
	}
	if calls[0].MAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("unexpected promoted MAC: %s", calls[0].MAC)
	}
}

func TestIsNonHostMAC(t *testing.T) {
	cases := map[string]bool{
		"ff:ff:ff:ff:ff:ff": true,
		"01:00:5e:00:00:fb": true,
		"33:33:00:00:00:01": true,
		"aa:bb:cc:dd:ee:ff": false,
		"02:11:22:33:44:55": false,
	}
	for in, want := range cases {
		if got := discovery.IsNonHostMAC(in); got != want {
			t.Errorf("IsNonHostMAC(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNormalizeMAC(t *testing.T) {
	cases := map[string]string{
		"AA:BB:CC:DD:EE:FF": "aa:bb:cc:dd:ee:ff",
		"1:2:3:4:5:6":       "01:02:03:04:05:06",
		"aa-bb-cc-dd-ee-ff": "aa:bb:cc:dd:ee:ff",
		"(incomplete)":      "",
		"xx:yy:zz:00:11:22": "",
		"":                  "",
	}
	for in, want := range cases {
		if got := discovery.NormalizeMAC(in); got != want {
			t.Errorf("NormalizeMAC(%q) = %q, want %q", in, got, want)
		}
	}
}
