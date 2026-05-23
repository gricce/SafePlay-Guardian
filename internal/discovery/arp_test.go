package discovery_test

import (
	"testing"

	"github.com/gricce/SafePlay-Guardian/internal/discovery"
)

func TestParseARPDarwin(t *testing.T) {
	sample := `? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]
alice-laptop.local (192.168.1.42) at 1:2:3:4:5:6 on en0 ifscope [ethernet]
? (192.168.1.7) at (incomplete) on en0 ifscope [ethernet]
? (224.0.0.251) at ff:ff:ff:ff:ff:ff on en0 ifscope permanent [ethernet]
`
	got := discovery.ParseARP(sample, "darwin")
	if len(got) != 3 {
		t.Fatalf("expected 3 entries (router + alice + multicast), got %d: %+v", len(got), got)
	}

	want := map[string]struct {
		ip   string
		host string
	}{
		"aa:bb:cc:dd:ee:ff": {ip: "192.168.1.1", host: ""},
		"01:02:03:04:05:06": {ip: "192.168.1.42", host: "alice-laptop.local"},
		"ff:ff:ff:ff:ff:ff": {ip: "224.0.0.251", host: ""},
	}
	for _, o := range got {
		exp, ok := want[o.MAC]
		if !ok {
			t.Errorf("unexpected MAC parsed: %s", o.MAC)
			continue
		}
		if o.IP != exp.ip {
			t.Errorf("MAC %s IP = %s, want %s", o.MAC, o.IP, exp.ip)
		}
		if o.Hostname != exp.host {
			t.Errorf("MAC %s host = %q, want %q", o.MAC, o.Hostname, exp.host)
		}
	}
}

func TestParseARPSkipsIncomplete(t *testing.T) {
	sample := `? (192.168.1.7) at (incomplete) on en0 ifscope [ethernet]`
	if got := discovery.ParseARP(sample, "darwin"); len(got) != 0 {
		t.Fatalf("incomplete entries must be skipped, got %+v", got)
	}
}

func TestParseARPWindows(t *testing.T) {
	sample := `Interface: 192.168.1.10 --- 0x12
  Internet Address      Physical Address      Type
  192.168.1.1           aa-bb-cc-dd-ee-ff     dynamic
  192.168.1.42          11-22-33-44-55-66     dynamic
`
	got := discovery.ParseARP(sample, "windows")
	if len(got) != 2 {
		t.Fatalf("expected 2 Windows entries, got %d", len(got))
	}
}
