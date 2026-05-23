package discovery

import (
	"context"
	"net"
	"os/exec"
	"sync"
	"time"
)

// PingSweep walks every host in the given IPv4 /24 (or smaller) network and
// fires a single ICMP echo via the platform `ping` command. The intent isn't
// to record results — it's to populate the ARP cache so the next ARPSource
// snapshot picks up devices that hadn't sent traffic recently.
//
// Uses context for timeout, not -W/-w flags (which differ in units across
// macOS and Linux). Concurrency is bounded so a /24 sweep doesn't spawn 254
// processes at once.
func PingSweep(ctx context.Context, network *net.IPNet, parallelism int, perHostTimeout time.Duration) {
	if network == nil {
		return
	}
	if parallelism <= 0 {
		parallelism = 32
	}
	if perHostTimeout <= 0 {
		perHostTimeout = 800 * time.Millisecond
	}
	if _, err := exec.LookPath("ping"); err != nil {
		return // graceful degrade — no ping binary, no sweep
	}
	ips := enumerateIPs(network)
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for _, ip := range ips {
		select {
		case <-ctx.Done():
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, perHostTimeout)
			defer cancel()
			_ = exec.CommandContext(cctx, "ping", "-c", "1", ip).Run()
		}(ip)
	}
	wg.Wait()
}

// enumerateIPs returns every host address inside the given /24-or-smaller
// IPv4 network, skipping the network and broadcast addresses.
func enumerateIPs(n *net.IPNet) []string {
	ones, bits := n.Mask.Size()
	if bits != 32 || ones < 22 || ones > 30 {
		return nil // refuse to enumerate /21 (2046 hosts) or larger
	}
	base := n.IP.Mask(n.Mask).To4()
	if base == nil {
		return nil
	}
	count := 1 << (uint(32 - ones))
	out := make([]string, 0, count-2)
	for i := 1; i < count-1; i++ {
		ip := make(net.IP, 4)
		copy(ip, base)
		v := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
		v += uint32(i)
		ip[0], ip[1], ip[2], ip[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
		out = append(out, ip.String())
	}
	return out
}
