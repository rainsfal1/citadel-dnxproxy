package discovery

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	cfg "dnsproxy/internal/config"
	"dnsproxy/internal/device"
	"dnsproxy/internal/policy"
)

type stubProvider struct {
	name    string
	records []device.DeviceRecord
}

func (s stubProvider) Name() string { return s.name }
func (s stubProvider) Discover(ctx context.Context) ([]device.DeviceRecord, error) {
	return s.records, nil
}

func mustMAC(t *testing.T, s string) net.HardwareAddr {
	t.Helper()
	m, err := net.ParseMAC(s)
	if err != nil {
		t.Fatalf("bad mac %s: %v", s, err)
	}
	return m
}

func TestDiscoveryUpdatesIPKeepsAssignment(t *testing.T) {
	tmp := t.TempDir()
	store, err := policy.LoadOrInitJSON(filepath.Join(tmp, "policy.json"))
	if err != nil {
		t.Fatalf("store init: %v", err)
	}
	now := time.Now().UTC()
	err = store.UpdateMutate(func(d *policy.Data) error {
		if d.Users == nil {
			d.Users = make(map[string]policy.User)
		}
		if d.Devices == nil {
			d.Devices = make(map[string]policy.Device)
		}
		d.Users["u1"] = policy.User{ID: "u1", Name: "Kid", DailyBudgetMinutes: 30, CreatedAt: now, UpdatedAt: now}
		d.Devices["dev1"] = policy.Device{
			ID:        "dev1",
			Name:      "tablet",
			IP:        "192.168.1.10",
			MAC:       "aa:bb:cc:dd:ee:ff",
			UserID:    "u1",
			CreatedAt: now,
			UpdatedAt: now,
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	mgr := NewManager(store, device.NewDeviceCache(nil, cfg.Router{}), Options{Interval: time.Hour})
	mgr.SetProviders([]Provider{
		stubProvider{
			name: "test",
			records: []device.DeviceRecord{
				{
					MAC:      mustMAC(t, "aa:bb:cc:dd:ee:ff"),
					IP:       net.ParseIP("192.168.1.22"),
					Hostname: "tablet",
					Source:   device.SourceARP,
					LastSeen: now.Add(time.Minute),
				},
			},
		},
	})
	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	rt := policy.Compile(store.DataSnapshot())
	eval := policy.Evaluator{Runtime: rt}
	dev, ok := eval.MatchDevice("192.168.1.22")
	if !ok {
		t.Fatalf("device not matched by new ip")
	}
	if dev.UserID != "u1" {
		t.Fatalf("assignment lost, user=%s", dev.UserID)
	}
	if dev.IP != "192.168.1.22" {
		t.Fatalf("ip not updated: %s", dev.IP)
	}
}

func TestDiscoveryMergesSourcesPrefersRouterName(t *testing.T) {
	tmp := t.TempDir()
	store, err := policy.LoadOrInitJSON(filepath.Join(tmp, "policy.json"))
	if err != nil {
		t.Fatalf("store init: %v", err)
	}
	mac := mustMAC(t, "aa:bb:cc:dd:ee:11")
	routerName := "router-box"
	arpName := "arp-name"

	mgr := NewManager(store, device.NewDeviceCache(nil, cfg.Router{}), Options{Interval: time.Hour})
	mgr.SetProviders([]Provider{
		stubProvider{
			name: "router",
			records: []device.DeviceRecord{
				{MAC: mac, IP: net.ParseIP("10.0.0.5"), Hostname: routerName, Name: routerName, Source: device.SourceRouter, LastSeen: time.Now()},
			},
		},
		stubProvider{
			name: "arp",
			records: []device.DeviceRecord{
				{MAC: mac, IP: net.ParseIP("10.0.0.5"), Hostname: arpName, Source: device.SourceARP, LastSeen: time.Now()},
			},
		},
	})
	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	data := store.DataSnapshot()
	if len(data.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(data.Devices))
	}
	var dev policy.Device
	for _, d := range data.Devices {
		dev = d
	}
	if dev.Hostname != routerName && dev.Name != routerName {
		t.Fatalf("expected router name to win, got hostname=%s name=%s", dev.Hostname, dev.Name)
	}
	if dev.Source != string(device.SourceRouter) {
		t.Fatalf("expected router source, got %s", dev.Source)
	}
}
