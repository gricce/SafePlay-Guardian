package device

import (
	"strconv"
	"strings"
)

// IsRandomizedMAC reports whether the given MAC has the locally-administered
// bit set. Modern phones rotate locally-administered MACs to defeat tracking,
// so such a MAC is too weak a signal to merge devices on its own.
//
// The IEEE-assigned (universal) bit is the second-least-significant bit of the
// first octet: bit value 0x02 set means "locally administered" (randomized
// or otherwise non-globally-unique).
func IsRandomizedMAC(mac string) bool {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return false
	}
	sep := ":"
	if !strings.Contains(mac, ":") && strings.Contains(mac, "-") {
		sep = "-"
	}
	parts := strings.Split(mac, sep)
	if len(parts) == 0 {
		return false
	}
	b, err := strconv.ParseUint(parts[0], 16, 8)
	if err != nil {
		return false
	}
	return byte(b)&0x02 != 0
}
