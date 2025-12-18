package discovery

import (
	"context"
	"log"
	"strings"
	"time"

	cfg "dnsproxy/internal/config"
	"dnsproxy/internal/device"
	"dnsproxy/internal/policy"
)

// Provider supplies discovered device records from a source (router, ARP, static, etc.).
type Provider interface {
	Name() string
	Discover(ctx context.Context) ([]device.DeviceRecord, error)
}

// Options configures a Manager.
type Options struct {
	Interval      time.Duration
	RouterClient  device.RouterClient
	StaticDevices []cfg.StaticDevice
	OnUpdate      func()
}

// Manager orchestrates discovery from multiple sources and persists stable identities.
type Manager struct {
	store     policy.Store
	cache     *device.DeviceCache
	providers []Provider
	interval  time.Duration
	onUpdate  func()
}

// NewManager builds a discovery manager with default providers (router, ARP, static).
func NewManager(store policy.Store, cache *device.DeviceCache, opts Options) *Manager {
	interval := opts.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	providers := []Provider{
		NewARPProvider(),
	}
	if opts.RouterClient != nil {
		providers = append(providers, NewRouterProvider(opts.RouterClient))
	}
	if len(opts.StaticDevices) > 0 {
		providers = append(providers, NewStaticProvider(opts.StaticDevices))
	}

	return &Manager{
		store:     store,
		cache:     cache,
		providers: providers,
		interval:  interval,
		onUpdate:  opts.OnUpdate,
	}
}

// SetProviders overrides the provider list (useful for tests).
func (m *Manager) SetProviders(providers []Provider) {
	m.providers = providers
}

// Start begins the periodic refresh loop and performs an initial scan.
func (m *Manager) Start(ctx context.Context) {
	if m.store == nil {
		return
	}
	_ = m.Refresh(ctx)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Refresh(ctx); err != nil {
				log.Printf("[WARN] device discovery refresh failed: %v", err)
			}
		}
	}
}

// Refresh queries all providers once and merges the results.
func (m *Manager) Refresh(ctx context.Context) error {
	if m.store == nil {
		return nil
	}
	var records []device.DeviceRecord
	for _, p := range m.providers {
		res, err := p.Discover(ctx)
		if err != nil {
			log.Printf("[WARN] discovery provider %s failed: %v", p.Name(), err)
			continue
		}
		records = append(records, res...)
	}
	if len(records) == 0 {
		return nil
	}
	if err := m.merge(records); err != nil {
		return err
	}
	if m.onUpdate != nil {
		m.onUpdate()
	}
	return nil
}

func (m *Manager) merge(records []device.DeviceRecord) error {
	now := time.Now()
	byKey := collapse(records, now)
	return m.store.UpdateMutate(func(data *policy.Data) error {
		if data.Devices == nil {
			data.Devices = make(map[string]policy.Device)
		}
		existingByMAC := make(map[string]string)
		for id, d := range data.Devices {
			mac := normalizeMAC(d.MAC)
			if mac != "" {
				existingByMAC[mac] = id
			}
		}
		for key, rec := range byKey {
			id, ok := existingByMAC[key]
			if !ok {
				id = key
			}
			dev := data.Devices[id]
			if dev.ID == "" {
				dev.ID = id
				dev.CreatedAt = now
			}
			if dev.Name == "" {
				dev.Name = rec.Name
			}
			if rec.Hostname != "" {
				dev.Hostname = rec.Hostname
			}
			if rec.Vendor != "" {
				dev.Vendor = rec.Vendor
			}
			if rec.Source != "" {
				dev.Source = string(rec.Source)
			}
			if rec.IP != nil {
				dev.IP = rec.IP.String()
			}
			if len(rec.MAC) > 0 {
				if mac := normalizeMAC(rec.MAC.String()); mac != "" {
					dev.MAC = mac
				}
			}
			if dev.LastSeen.IsZero() || rec.LastSeen.After(dev.LastSeen) {
				dev.LastSeen = rec.LastSeen
			}
			dev.UpdatedAt = now
			data.Devices[id] = dev

			if m.cache != nil {
				m.cache.Upsert(rec)
			}
		}
		return nil
	})
}

func collapse(records []device.DeviceRecord, now time.Time) map[string]device.DeviceRecord {
	result := make(map[string]device.DeviceRecord)
	for _, r := range records {
		if r.IP == nil && len(r.MAC) == 0 {
			continue
		}
		if r.LastSeen.IsZero() {
			r.LastSeen = now
		}
		key := recordKey(r)
		if key == "" {
			continue
		}
		curr, ok := result[key]
		if !ok || sourcePriority(r.Source) > sourcePriority(curr.Source) {
			if r.Name == "" {
				r.Name = bestName(r)
			}
			if r.Hostname == "" {
				r.Hostname = r.Name
			}
			result[key] = r
			continue
		}
		// Merge secondary sources without overwriting good data.
		merged := curr
		if merged.IP == nil && r.IP != nil {
			merged.IP = r.IP
		}
		if merged.Hostname == "" && r.Hostname != "" {
			merged.Hostname = r.Hostname
		}
		if merged.Name == "" && r.Name != "" {
			merged.Name = r.Name
		}
		if r.LastSeen.After(merged.LastSeen) {
			merged.LastSeen = r.LastSeen
		}
		result[key] = merged
	}
	return result
}

func bestName(r device.DeviceRecord) string {
	if r.Name != "" {
		return r.Name
	}
	if r.Hostname != "" {
		return r.Hostname
	}
	if len(r.MAC) > 0 {
		return normalizeMAC(r.MAC.String())
	}
	if r.IP != nil {
		return r.IP.String()
	}
	return ""
}

func recordKey(r device.DeviceRecord) string {
	if len(r.MAC) > 0 {
		return normalizeMAC(r.MAC.String())
	}
	if r.IP != nil {
		return "ip:" + r.IP.String()
	}
	return ""
}

func normalizeMAC(mac string) string {
	mac = strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
	return mac
}

func sourcePriority(s device.Source) int {
	switch s {
	case device.SourceRouter:
		return 3
	case device.SourceStatic:
		return 2
	case device.SourceARP:
		return 1
	default:
		return 0
	}
}
