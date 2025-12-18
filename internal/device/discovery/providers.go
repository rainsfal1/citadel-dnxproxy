package discovery

import (
	"bufio"
	"context"
	"net"
	"os"
	"strings"
	"time"

	cfg "dnsproxy/internal/config"
	"dnsproxy/internal/device"
)

type routerProvider struct {
	client device.RouterClient
}

// NewRouterProvider returns a Provider backed by a router API.
func NewRouterProvider(client device.RouterClient) Provider {
	return &routerProvider{client: client}
}

func (p *routerProvider) Name() string { return "router" }

func (p *routerProvider) Discover(ctx context.Context) ([]device.DeviceRecord, error) {
	if p.client == nil {
		return nil, nil
	}
	devs, err := p.client.GetAttachedDeviceList()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]device.DeviceRecord, 0, len(devs))
	for _, d := range devs {
		out = append(out, device.DeviceRecord{
			MAC:      d.MAC,
			IP:       d.IP,
			Name:     strings.ToLower(d.Name),
			Hostname: strings.ToLower(d.Name),
			Source:   device.SourceRouter,
			LastSeen: now,
		})
	}
	return out, nil
}

type arpProvider struct{}

// NewARPProvider reads the local ARP/neighbor table when available.
func NewARPProvider() Provider {
	return &arpProvider{}
}

func (p *arpProvider) Name() string { return "arp" }

func (p *arpProvider) Discover(ctx context.Context) ([]device.DeviceRecord, error) {
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		// Non-Linux platforms simply skip ARP discovery.
		return nil, nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// Skip header
	if !scanner.Scan() {
		return nil, nil
	}
	now := time.Now()
	var records []device.DeviceRecord
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		ip := net.ParseIP(fields[0])
		mac, err := net.ParseMAC(fields[3])
		if err != nil || ip == nil {
			continue
		}
		records = append(records, device.DeviceRecord{
			IP:       ip,
			MAC:      mac,
			Hostname: strings.ToLower(fields[0]),
			Source:   device.SourceARP,
			LastSeen: now,
		})
	}
	return records, nil
}

type staticProvider struct {
	devices []cfg.StaticDevice
}

// NewStaticProvider wraps configured static devices.
func NewStaticProvider(devices []cfg.StaticDevice) Provider {
	return &staticProvider{devices: devices}
}

func (p *staticProvider) Name() string { return "static" }

func (p *staticProvider) Discover(ctx context.Context) ([]device.DeviceRecord, error) {
	now := time.Now()
	var records []device.DeviceRecord
	for _, d := range p.devices {
		ip := net.ParseIP(d.IP)
		mac, _ := net.ParseMAC(d.MAC)
		records = append(records, device.DeviceRecord{
			MAC:      mac,
			IP:       ip,
			Name:     strings.ToLower(d.Name),
			Hostname: strings.ToLower(d.Name),
			Source:   device.SourceStatic,
			LastSeen: now,
		})
	}
	return records, nil
}
