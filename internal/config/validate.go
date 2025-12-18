package config

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// Validate performs strict validation of the top-level configuration.
// It returns the first encountered error with a JSON-style field path.
func (c *Config) Validate() error {
	if c.Logfile == "" {
		return fmt.Errorf("Logfile: must be set")
	}
	if c.ListenAddress == "" {
		return fmt.Errorf("ListenAddress: must be set (ex: \":53\" or \"127.0.0.1:2053\")")
	}
	if err := validateAction("DefaultRule", c.DefaultRule, true); err != nil {
		return err
	}
	if err := validateAction("OnErrorRule", c.OnErrorRule, true); err != nil {
		return err
	}
	if len(c.NameServers) == 0 {
		return fmt.Errorf("NameServers: must include at least one entry")
	}
	for i, ns := range c.NameServers {
		if err := ns.Validate(fieldPath("NameServers", i)); err != nil {
			return err
		}
	}
	if err := c.Router.Validate("Router"); err != nil {
		return err
	}
	if c.UpstreamTimeoutMs < 0 {
		return fmt.Errorf("UpstreamTimeoutMs: must be >=0")
	}
	if c.UpstreamRetries < 0 {
		return fmt.Errorf("UpstreamRetries: must be >=0")
	}
	if c.UpstreamMaxFails < 0 {
		return fmt.Errorf("UpstreamMaxFails: must be >=0")
	}
	if c.UpstreamFailWindowSec < 0 {
		return fmt.Errorf("UpstreamFailWindowSec: must be >=0")
	}
	if c.CacheTTLOverrideSec < 0 {
		return fmt.Errorf("CacheTTLOverrideSec: must be >=0")
	}
	if c.NegativeCacheTTL < 0 {
		return fmt.Errorf("NegativeCacheTTL: must be >=0")
	}
	for i, h := range c.Hosts {
		if err := h.Validate(fieldPath("Hosts", i)); err != nil {
			return err
		}
	}
	for i, d := range c.Domains {
		if err := d.Validate(fieldPath("Domains", i)); err != nil {
			return err
		}
	}
	for i, r := range c.Resolve {
		if err := r.ValidateResolve(fieldPath("Resolve", i)); err != nil {
			return err
		}
	}
	for i, sd := range c.StaticDevices {
		if err := sd.Validate(fieldPath("StaticDevices", i)); err != nil {
			return err
		}
	}
	if err := c.BlockStrategy.Validate("BlockStrategy", c.IPv4BlockResolve, c.IPv6BlockResolve); err != nil {
		return err
	}
	return nil
}

// Validate validates a router configuration.
func (r Router) Validate(path string) error {
	if r.Engine == RouterTypeNone {
		return nil
	}
	if err := validateRouterType(path+".Engine", r.Engine); err != nil {
		return err
	}
	if strings.TrimSpace(r.Host) == "" {
		return fmt.Errorf("%s.Host: must be set when router engine is enabled", path)
	}
	if strings.TrimSpace(r.Port) == "" {
		return fmt.Errorf("%s.Port: must be set when router engine is enabled", path)
	}
	if strings.TrimSpace(r.User) == "" {
		return fmt.Errorf("%s.User: must be set when router engine is enabled", path)
	}
	if strings.TrimSpace(r.Password) == "" {
		return fmt.Errorf("%s.Password: must be set when router engine is enabled", path)
	}
	if r.PollChanges && r.PollInterval < 10 {
		return fmt.Errorf("%s.PollInterval: must be >= 10 seconds when PollChanges is true", path)
	}
	return nil
}

// Validate validates a nameserver entry.
func (n NameServer) Validate(path string) error {
	if strings.TrimSpace(n.IP) == "" {
		return fmt.Errorf("%s.IP: must be set", path)
	}
	if _, _, err := net.SplitHostPort(n.IP); err != nil {
		return fmt.Errorf("%s.IP: must be host:port (err: %v)", path, err)
	}
	return nil
}

// Validate validates a host block (used in Hosts, Domains.Hosts, Resolve).
func (h Host) Validate(path string) error {
	if strings.TrimSpace(h.Name) == "" {
		return fmt.Errorf("%s.Name: must be set", path)
	}
	for i, r := range h.Rules {
		if err := r.Validate(fieldPath(path+".Rules", i)); err != nil {
			return err
		}
	}
	return nil
}

// ValidateResolve validates a resolver override host.
func (h Host) ValidateResolve(path string) error {
	if strings.TrimSpace(h.Name) == "" {
		return fmt.Errorf("%s.Name: must be set", path)
	}
	if strings.TrimSpace(h.IpV4) == "" {
		return fmt.Errorf("%s.IpV4: must be set", path)
	}
	if parsed := net.ParseIP(h.IpV4); parsed == nil {
		return fmt.Errorf("%s.IpV4: invalid IP address", path)
	}
	return nil
}

// Validate validates a domain block.
func (d Domain) Validate(path string) error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("%s.Name: must be set", path)
	}
	for i, h := range d.Hosts {
		if err := h.Validate(fieldPath(path+".Hosts", i)); err != nil {
			return err
		}
	}
	return nil
}

// Validate validates a rule.
func (r Rule) Validate(path string) error {
	if err := validateAction(path+".Type", r.Type, false); err != nil {
		return err
	}
	if strings.TrimSpace(r.TimeSpan) != "" {
		if err := validateTimeSpan(path+".TimeSpan", r.TimeSpan); err != nil {
			return err
		}
	}
	return nil
}

// Validate validates a static device entry.
func (s StaticDevice) Validate(path string) error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("%s.Name: must be set", path)
	}
	if strings.TrimSpace(s.IP) == "" {
		return fmt.Errorf("%s.IP: must be set", path)
	}
	if parsed := net.ParseIP(s.IP); parsed == nil {
		return fmt.Errorf("%s.IP: invalid IP address", path)
	}
	if strings.TrimSpace(s.MAC) != "" {
		if _, err := net.ParseMAC(s.MAC); err != nil {
			return fmt.Errorf("%s.MAC: invalid MAC address", path)
		}
	}
	return nil
}

func (b BlockStrategy) Validate(path, defaultV4, defaultV6 string) error {
	action := strings.ToLower(strings.TrimSpace(b.Action))
	if action == "" {
		action = "nullroute"
	}
	switch action {
	case "nxdomain", "refuse", "nullroute", "sinkhole":
	default:
		return fmt.Errorf("%s.Action: invalid value", path)
	}
	if action == "sinkhole" || action == "nullroute" {
		v4 := b.SinkholeIPv4
		if v4 == "" {
			v4 = defaultV4
		}
		if net.ParseIP(v4) == nil {
			return fmt.Errorf("%s.SinkholeIPv4: invalid IP", path)
		}
		v6 := b.SinkholeIPv6
		if v6 == "" {
			v6 = defaultV6
		}
		if net.ParseIP(v6) == nil {
			return fmt.Errorf("%s.SinkholeIPv6: invalid IP", path)
		}
	}
	return nil
}

func validateRouterType(path string, rt RouterType) error {
	switch rt {
	case RouterTypeNone, RouterTypeNetGear, RouterTypeUnifi:
		return nil
	default:
		return fmt.Errorf("%s: invalid router engine", path)
	}
}

func validateAction(path string, action ActionType, required bool) error {
	switch action {
	case ActionTypePass,
		ActionTypeBlockedDevice,
		ActionTypeBlockedSiteBan,
		ActionTypeBlockedTimeSpan:
		return nil
	case ActionTypeNone:
		if required {
			return fmt.Errorf("%s: must be set to a non-None action", path)
		}
		return nil
	default:
		return fmt.Errorf("%s: invalid action", path)
	}
}

func validateTimeSpan(path, span string) error {
	// Supported forms:
	//   HH:MM-HH:MM
	//   mon,tue@HH:MM-HH:MM
	//   mon-fri@HH:MM-HH:MM|tz=Europe/Stockholm
	parts := strings.Split(span, "|")
	timePart := parts[0]
	if timePart == "" {
		return fmt.Errorf("%s: must include time window", path)
	}
	// strip weekdays if present
	timeRange := timePart
	if strings.Contains(timePart, "@") {
		partsWeek := strings.SplitN(timePart, "@", 2)
		if len(partsWeek) != 2 || strings.TrimSpace(partsWeek[0]) == "" {
			return fmt.Errorf("%s: invalid weekday section", path)
		}
		timeRange = partsWeek[1]
	}
	times := strings.Split(timeRange, "-")
	if len(times) != 2 {
		return fmt.Errorf("%s: must be in HH:MM-HH:MM", path)
	}
	start, err := time.Parse("15:04", times[0])
	if err != nil {
		return fmt.Errorf("%s: start time invalid (%v)", path, err)
	}
	end, err := time.Parse("15:04", times[1])
	if err != nil {
		return fmt.Errorf("%s: end time invalid (%v)", path, err)
	}
	if start == end {
		return fmt.Errorf("%s: start and end time cannot be equal", path)
	}
	// tz validation if provided
	if len(parts) > 1 {
		for _, opt := range parts[1:] {
			opt = strings.TrimSpace(opt)
			if opt == "" {
				continue
			}
			if strings.HasPrefix(opt, "tz=") {
				locName := strings.TrimPrefix(opt, "tz=")
				if _, err := time.LoadLocation(locName); err != nil {
					return fmt.Errorf("%s: invalid timezone %q", path, locName)
				}
			} else {
				return fmt.Errorf("%s: unknown option %q", path, opt)
			}
		}
	}
	return nil
}

func fieldPath(base string, idx int) string {
	return fmt.Sprintf("%s[%d]", base, idx)
}
