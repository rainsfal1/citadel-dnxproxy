package device

import (
	"net"
	"strings"
	"time"
)

// Source indicates where a device record originated.
type Source string

const (
	SourceRouter Source = "router"
	SourceARP    Source = "arp"
	SourceStatic Source = "static"
	SourceManual Source = "manual"
)

// DeviceRecord captures the stable device identity we cache from discovery.
type DeviceRecord struct {
	MAC      net.HardwareAddr
	IP       net.IP
	Name     string
	Hostname string
	Vendor   string
	Source   Source
	LastSeen time.Time
}

func normalizeMAC(mac net.HardwareAddr) string {
	if mac == nil {
		return ""
	}
	s := strings.ToLower(mac.String())
	s = strings.ReplaceAll(s, "-", ":")
	return s
}

func normalizeIP(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return strings.ToLower(ip.String())
}
