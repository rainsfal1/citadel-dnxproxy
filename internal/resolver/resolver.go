package resolver

import (
	"errors"
	"log"
	"strings"

	cfg "dnsproxy/internal/config"
	dev "dnsproxy/internal/device"
	"dnsproxy/internal/utils"
)

type Resolver struct {
	conf        *cfg.Config
	deviceCache *dev.DeviceCache
}

var ErrHostNotFound = errors.New("Host not found")

func New(conf *cfg.Config, deviceCache *dev.DeviceCache) *Resolver {
	re := Resolver{
		conf:        conf,
		deviceCache: deviceCache,
	}
	log.Printf("Resolving the following:\n")
	for _, r := range conf.Resolve {
		log.Printf("%s -> %s\n", r.Name, r.IpV4)
	}
	return &re
}

func (r *Resolver) Resolve(domain string) (string, error) {
	for _, r := range r.conf.Resolve {
		if utils.WildcardPatternMatch(strings.ToLower(domain), strings.ToLower(r.Name)) {
			return r.IpV4, nil
		}
	}

	if r.deviceCache != nil {
		domain = strings.TrimSuffix(domain, ".")
		ip, err := r.deviceCache.NameToIP(domain)
		if err != nil {
			return "", ErrHostNotFound
		}
		return ip.String(), nil
	}
	return "", ErrHostNotFound
}

// SetDeviceCache swaps the device cache used for local name lookups.
func (r *Resolver) SetDeviceCache(dc *dev.DeviceCache) {
	r.deviceCache = dc
}
