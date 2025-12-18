package policy

import "time"

// AdminState tracks portal authentication and first-boot status.
type AdminState struct {
	PasswordHash  string    `json:"password_hash"`
	PasswordSalt  string    `json:"password_salt"` // legacy sha256 salt retained for backward compat
	InitialSecret string    `json:"initial_secret"`
	FirstBoot     bool      `json:"first_boot"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// User represents a household member with a daily time budget and optional allow windows.
type User struct {
	ID                 string       `json:"id"`
	Name               string       `json:"name"`
	DailyBudgetMinutes int          `json:"daily_budget_minutes"`
	AllowWindows       []TimeWindow `json:"allow_windows"`
	DomainRules        []DomainRule `json:"domain_rules"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
	Notes              string       `json:"notes,omitempty"`
	DeviceIDs          []string     `json:"device_ids,omitempty"`
}

// Device represents a device on the LAN mapped to a user.
type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IP        string    `json:"ip"`
	MAC       string    `json:"mac,omitempty"`
	Hostname  string    `json:"hostname,omitempty"`
	Vendor    string    `json:"vendor,omitempty"`
	Source    string    `json:"source,omitempty"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TimeWindow uses the same "HH:mm-HH:mm|tz=Area/City" syntax as rules timespans.
type TimeWindow struct {
	Name     string `json:"name"`
	TimeSpan string `json:"timespan"`
}

// DomainAction is the effect of a domain rule.
type DomainAction string

const (
	DomainActionBlock DomainAction = "block"
	DomainActionAllow DomainAction = "allow"
)

// DomainRule applies to a user (and optionally a specific device) with wildcard patterns.
type DomainRule struct {
	ID      string       `json:"id"`
	UserID  string       `json:"user_id"`
	Pattern string       `json:"pattern"`
	Action  DomainAction `json:"action"`
	Notes   string       `json:"notes,omitempty"`
}

// SessionState tracks active session timing to ensure we accrue correctly across restarts.
type SessionState struct {
	UserID   string    `json:"user_id"`
	Day      string    `json:"day"` // YYYY-MM-DD in configured timezone
	Active   bool      `json:"active"`
	Start    time.Time `json:"start"`
	LastSeen time.Time `json:"last_seen"`
}

// UsageSummary tracks daily totals per user keyed by day.
type UsageSummary struct {
	UserID string `json:"user_id"`
	Day    string `json:"day"`
	// Seconds accrued in the given day (does not include active in-flight session time).
	Seconds int64 `json:"seconds"`
}

// Settings are household-level policy knobs.
type Settings struct {
	IdleTimeoutMinutes int    `json:"idle_timeout_minutes"`
	BudgetResetTZ      string `json:"budget_reset_tz"`
}

// AuditEvent records administrative changes.
type AuditEvent struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Details   string    `json:"details,omitempty"`
}
