package policy

import (
	"strings"
	"time"

	"dnsproxy/internal/utils"
)

// Runtime holds compiled lookup tables for fast evaluation.
type Runtime struct {
	UsersByID    map[string]User
	DevicesByID  map[string]Device
	DevicesByIP  map[string]Device
	DevicesByMAC map[string]Device
	RulesByUser  map[string][]DomainRule
	Settings     Settings
	BudgetTz     *time.Location
	AllowWindows map[string][]TimeWindow
	Audit        []AuditEvent
}

// Compile builds a Runtime snapshot from persisted data.
func Compile(data Data) *Runtime {
	rt := &Runtime{
		UsersByID:    make(map[string]User),
		DevicesByID:  make(map[string]Device),
		DevicesByIP:  make(map[string]Device),
		DevicesByMAC: make(map[string]Device),
		RulesByUser:  make(map[string][]DomainRule),
		AllowWindows: make(map[string][]TimeWindow),
		Settings:     data.Settings,
		BudgetTz:     time.Local,
	}
	if data.Settings.BudgetResetTZ != "" {
		if loc, err := time.LoadLocation(data.Settings.BudgetResetTZ); err == nil {
			rt.BudgetTz = loc
		}
	}
	for id, u := range data.Users {
		rt.UsersByID[id] = u
		if len(u.AllowWindows) > 0 {
			rt.AllowWindows[id] = append(rt.AllowWindows[id], u.AllowWindows...)
		}
	}
	for id, d := range data.Devices {
		rt.DevicesByID[id] = d
		if d.IP != "" {
			rt.DevicesByIP[utils.StripPortFromAddr(d.IP)] = d
		}
		if mac := normalizeMACString(d.MAC); mac != "" {
			rt.DevicesByMAC[mac] = d
		}
		if d.UserID != "" {
			u := rt.UsersByID[d.UserID]
			u.DeviceIDs = append(u.DeviceIDs, d.ID)
			rt.UsersByID[d.UserID] = u
		}
	}
	for _, r := range data.DomainRules {
		if r.UserID == "" {
			continue
		}
		rt.RulesByUser[r.UserID] = append(rt.RulesByUser[r.UserID], r)
	}
	rt.Audit = data.Audit
	return rt
}

func normalizeMACString(mac string) string {
	mac = strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
	return strings.TrimSpace(mac)
}
