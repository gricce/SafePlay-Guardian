package discovery

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/gricce/SafePlay-Guardian/internal/device"
)

// ARPSource reads the OS ARP table by exec'ing `arp -a`. Passive, no
// privileges. Parses macOS/BSD and Linux output; Windows output is recognized
// by a separate regex.
type ARPSource struct{}

func (ARPSource) Name() string { return "arp" }

func (a ARPSource) Snapshot(ctx context.Context) ([]device.Observation, error) {
	out, err := exec.CommandContext(ctx, "arp", "-a").Output()
	if err != nil {
		return nil, err
	}
	return ParseARP(string(out), runtime.GOOS), nil
}

// Format on macOS/Linux: "? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 [ethernet]"
//                       "host.local (192.168.1.42) at 11:22:33:44:55:66 on en0 ..."
var unixARP = regexp.MustCompile(`^([^\s]+) \((\d{1,3}(?:\.\d{1,3}){3})\) at ([0-9a-fA-F:]+|\(incomplete\))`)

// Format on Windows: "  192.168.1.1        aa-bb-cc-dd-ee-ff     dynamic"
var winARP = regexp.MustCompile(`^\s*(\d{1,3}(?:\.\d{1,3}){3})\s+([0-9a-fA-F-]{17})\s+(dynamic|static)`)

func ParseARP(text, goos string) []device.Observation {
	var out []device.Observation
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		if goos == "windows" {
			if m := winARP.FindStringSubmatch(line); m != nil {
				if mac := NormalizeMAC(m[2]); mac != "" {
					out = append(out, device.Observation{MAC: mac, IP: m[1]})
				}
			}
			continue
		}
		m := unixARP.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		host, ip, mac := m[1], m[2], m[3]
		mac = NormalizeMAC(mac)
		if mac == "" {
			continue
		}
		obs := device.Observation{MAC: mac, IP: ip}
		if host != "?" {
			obs.Hostname = strings.TrimSuffix(host, ".")
		}
		out = append(out, obs)
	}
	return out
}
