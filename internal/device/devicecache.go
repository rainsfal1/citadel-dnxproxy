package device

import (
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	cfg "dnsproxy/internal/config"
)

var ErrDeviceNameNotFound = errors.New("Device name not found")
var ErrDeviceIPNotFound = errors.New("Device IP not found")

type DeviceCache struct {
	routerConfig cfg.Router
	routerClient RouterClient
	devFromMAC   map[string]DeviceRecord
	devFromIP    map[string]DeviceRecord
	lock         sync.Mutex
}

func NewDeviceCache(routerClient RouterClient, router cfg.Router) *DeviceCache {
	dc := DeviceCache{
		routerClient: routerClient,
		routerConfig: router,
		devFromMAC:   make(map[string]DeviceRecord),
		devFromIP:    make(map[string]DeviceRecord),
	}
	return &dc
}

func (dc *DeviceCache) Initialize() error {
	return dc.Refresh()
}

func (dc *DeviceCache) Refresh() error {
	err := dc.doRefresh()
	if err != nil {
		err = dc.reInitialize()
		if err != nil {
			log.Printf("[ERROR] DeviceCache::Refresh, Unable to reinitialize router, err: %s\n", err.Error())
		}
	}
	return err
}

func (dc *DeviceCache) reInitialize() error {
	return dc.routerClient.Login(dc.routerConfig.Host, dc.routerConfig.Port, dc.routerConfig.User, dc.routerConfig.Password)
}

func (dc *DeviceCache) doRefresh() error {
	if dc.routerClient == nil {
		return nil
	}

	devices, err := dc.routerClient.GetAttachedDeviceList()
	if err != nil {
		log.Printf("[ERROR] DeviceCache::Refresh, failed to retrieve list of attached devices: %s\n", err.Error())
		return err
	}

	dc.lock.Lock()
	defer dc.lock.Unlock()

	// set to table
	for _, d := range devices {
		record := DeviceRecord{
			IP:       d.IP,
			Name:     strings.ToLower(d.Name),
			Hostname: strings.ToLower(d.Name),
			MAC:      d.MAC,
			Source:   SourceRouter,
			LastSeen: time.Now(),
		}
		dc.upsertLocked(record)
	}
	return nil
}

func (dc *DeviceCache) StartAutoRefresh(pollintervalsec int) {
	go func() {
		for {
			time.Sleep(time.Duration(pollintervalsec) * time.Second)
			dc.Refresh()
		}
	}()
}

func (dc *DeviceCache) NameToIP(name string) (net.IP, error) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	for _, d := range dc.devFromMAC {
		if d.Name == strings.ToLower(name) || d.Hostname == strings.ToLower(name) {
			return d.IP, nil
		}
	}
	if d, ok := dc.devFromIP[strings.ToLower(name)]; ok {
		return d.IP, nil
	}
	return nil, ErrDeviceNameNotFound
}

func (dc *DeviceCache) IPToName(ipaddr string) (string, error) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	if d, ok := dc.devFromIP[strings.ToLower(ipaddr)]; ok {
		if d.Name != "" {
			return d.Name, nil
		}
		if d.Hostname != "" {
			return d.Hostname, nil
		}
		return normalizeMAC(d.MAC), nil
	}
	return "", ErrDeviceIPNotFound
}

func (dc *DeviceCache) Dump() {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	for _, d := range dc.devFromMAC {
		log.Printf("%s (%s) : %s [%s]\n", d.Name, d.Hostname, d.IP.String(), d.Source)
	}
}

// LoadStaticDevices populates the cache from static configuration (no router needed)
func (dc *DeviceCache) LoadStaticDevices(staticDevices []cfg.StaticDevice) error {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	log.Printf("[INFO] Loading %d static devices...\n", len(staticDevices))
	for _, sd := range staticDevices {
		ip := net.ParseIP(sd.IP)
		if ip == nil {
			log.Printf("[WARN] Invalid IP '%s' for device '%s', skipping\n", sd.IP, sd.Name)
			continue
		}

		mac, err := net.ParseMAC(sd.MAC)
		if err != nil && sd.MAC != "" {
			log.Printf("[WARN] Invalid MAC '%s' for device '%s', using default\n", sd.MAC, sd.Name)
			mac, _ = net.ParseMAC("00:00:00:00:00:00")
		}

		rd := RouterDevice{
			IP:   ip,
			Name: strings.ToLower(sd.Name),
			MAC:  mac,
		}

		record := DeviceRecord{
			IP:       rd.IP,
			Name:     rd.Name,
			Hostname: rd.Name,
			MAC:      rd.MAC,
			Source:   SourceStatic,
			LastSeen: time.Now(),
		}
		dc.upsertLocked(record)
		log.Printf("  %s -> %s\n", record.Name, record.IP.String())
	}

	return nil
}

// Upsert inserts or updates a record in the cache using MAC as primary identity.
func (dc *DeviceCache) Upsert(record DeviceRecord) {
	dc.lock.Lock()
	defer dc.lock.Unlock()
	dc.upsertLocked(record)
}

func (dc *DeviceCache) upsertLocked(record DeviceRecord) {
	macKey := normalizeMAC(record.MAC)
	if macKey == "" {
		macKey = normalizeIP(record.IP)
	}
	// Preserve friendly name if present.
	existing, ok := dc.devFromMAC[macKey]
	if ok && existing.Name != "" && record.Name == "" {
		record.Name = existing.Name
	}
	if record.Name == "" {
		if record.Hostname != "" {
			record.Name = record.Hostname
		} else {
			record.Name = macKey
		}
	}
	dc.devFromMAC[macKey] = record
	if record.IP != nil {
		dc.devFromIP[record.IP.String()] = record
	}
}
