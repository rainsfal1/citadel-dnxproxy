package policy

import (
	"strings"
	"time"

	"dnsproxy/internal/utils"
)

type Reason string

const (
	ReasonNone           Reason = ""
	ReasonBudgetExceeded Reason = "budget_exceeded"
	ReasonOutsideWindow  Reason = "outside_allow_window"
	ReasonPolicyBlock    Reason = "policy_domain_block"
	ReasonConfigBlock    Reason = "config_rule_block"
)

// Decision captures the combined view for a request.
type Decision struct {
	UserID    string
	UserName  string
	DeviceID  string
	DeviceIP  string
	Blocked   bool
	Reason    Reason
	PolicyHit string // matching policy rule pattern if any
}

// Evaluator applies compiled policy to a request.
type Evaluator struct {
	Runtime *Runtime
}

// MatchDevice finds device by IP if present in runtime.
func (e *Evaluator) MatchDevice(ip string) (Device, bool) {
	if e == nil || e.Runtime == nil {
		return Device{}, false
	}
	d, ok := e.Runtime.DevicesByIP[utils.StripPortFromAddr(strings.ToLower(ip))]
	return d, ok
}

// MatchDeviceByMAC allows callers to resolve an identity directly by MAC.
func (e *Evaluator) MatchDeviceByMAC(mac string) (Device, bool) {
	if e == nil || e.Runtime == nil {
		return Device{}, false
	}
	mac = strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
	d, ok := e.Runtime.DevicesByMAC[mac]
	return d, ok
}

// MatchUser resolves a user by device or explicit id.
func (e *Evaluator) MatchUser(device Device) (User, bool) {
	if e == nil || e.Runtime == nil {
		return User{}, false
	}
	if device.UserID == "" {
		return User{}, false
	}
	u, ok := e.Runtime.UsersByID[device.UserID]
	return u, ok
}

// MatchDomainRule picks the most specific domain rule for the user.
func (e *Evaluator) MatchDomainRule(userID, domain string) (DomainRule, bool) {
	if e == nil || e.Runtime == nil {
		return DomainRule{}, false
	}
	rules := e.Runtime.RulesByUser[userID]
	var best DomainRule
	var found bool
	bestScore := -1
	for _, r := range rules {
		if !utils.WildcardPatternMatch(domain, strings.ToLower(r.Pattern)) {
			continue
		}
		// Score: fewer wildcards and longer pattern preferred.
		wildcards := strings.Count(r.Pattern, "*") + strings.Count(r.Pattern, "?")
		score := (len(r.Pattern) * 2) - wildcards
		if score > bestScore {
			bestScore = score
			best = r
			found = true
		}
	}
	return best, found
}

// InAllowWindow returns true if the current time is within any of the user's allow windows.
func (e *Evaluator) InAllowWindow(userID string, now time.Time, checker func(ts string, now time.Time) bool) bool {
	if e == nil || e.Runtime == nil {
		return true
	}
	windows := e.Runtime.AllowWindows[userID]
	if len(windows) == 0 {
		return true
	}
	for _, w := range windows {
		if checker(w.TimeSpan, now) {
			return true
		}
	}
	return false
}
