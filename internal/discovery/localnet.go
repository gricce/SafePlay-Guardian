package discovery

import (
	"net"
	"strings"
)

// LocalMACs returns the set of normalized MACs belonging to interfaces on this
// host. The scanner uses this to hide its own presence from the discovered
// list. Loopback and zero-MAC interfaces are skipped.
func LocalMACs() map[string]bool {
	out := map[string]bool{}
	ifs, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifs {
		if iface.HardwareAddr == nil || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		hw := iface.HardwareAddr.String()
		if hw == "" || strings.HasPrefix(hw, "00:00:00:00:00:00") {
			continue
		}
		if n := NormalizeMAC(hw); n != "" {
			out[n] = true
		}
	}
	return out
}

// PrimaryIPv4Network returns the IPv4 network of the first non-loopback,
// up-and-running interface that has an IPv4 address. It's a best-effort
// guess for which subnet to sweep.
func PrimaryIPv4Network() *net.IPNet {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifs {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil {
				continue
			}
			return &net.IPNet{IP: ip4, Mask: ipn.Mask}
		}
	}
	return nil
}
