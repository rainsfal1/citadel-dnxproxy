package policy

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	_ "modernc.org/sqlite"
)

// SQLiteStore is a persisted SQLite-backed policy store.
type SQLiteStore struct {
	path string
	db   *sql.DB
	mu   sync.Mutex
}

// OpenSQLite opens (or creates) a SQLite store at path.
func OpenSQLite(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &SQLiteStore{path: path, db: db}
	if err := store.migrate(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) migrate() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	salt, hash := defaultAdminCreds()
	secret := randomSecret()

	// Base tables for a fresh install (latest schema).
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (id INTEGER PRIMARY KEY CHECK (id=1), version INTEGER);`,
		`INSERT INTO schema_version(id, version) VALUES (1, 1) ON CONFLICT(id) DO NOTHING;`,
		`CREATE TABLE IF NOT EXISTS admin_state (id INTEGER PRIMARY KEY CHECK (id=1), password_hash TEXT, password_salt TEXT, initial_secret TEXT, first_boot INTEGER, updated_at TEXT);`,
		`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, name TEXT, daily_budget_minutes INTEGER, allow_windows TEXT, notes TEXT, created_at TEXT, updated_at TEXT);`,
		`CREATE TABLE IF NOT EXISTS devices (id TEXT PRIMARY KEY, name TEXT, ip TEXT, mac TEXT, hostname TEXT, vendor TEXT, source TEXT, last_seen TEXT, user_id TEXT, created_at TEXT, updated_at TEXT);`,
		`CREATE TABLE IF NOT EXISTS domain_rules (id TEXT PRIMARY KEY, user_id TEXT, pattern TEXT, action TEXT, notes TEXT);`,
		`CREATE TABLE IF NOT EXISTS sessions (user_id TEXT PRIMARY KEY, day TEXT, active INTEGER, start_ts TEXT, last_seen TEXT);`,
		`CREATE TABLE IF NOT EXISTS usage (user_id TEXT PRIMARY KEY, day TEXT, seconds INTEGER);`,
		`CREATE TABLE IF NOT EXISTS settings (id INTEGER PRIMARY KEY CHECK (id=1), idle_timeout_minutes INTEGER, budget_reset_tz TEXT);`,
		`INSERT INTO settings(id, idle_timeout_minutes, budget_reset_tz) VALUES (1, 10, '') ON CONFLICT(id) DO NOTHING;`,
		`CREATE TABLE IF NOT EXISTS audit (id TEXT PRIMARY KEY, ts TEXT, actor TEXT, action TEXT, details TEXT);`,
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, st := range stmts {
		if _, err := tx.Exec(st, currentSchemaVersion); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	version := 1
	_ = s.db.QueryRow(`SELECT version FROM schema_version WHERE id=1`).Scan(&version)

	if version < 2 {
		if err := s.migrateToV2(); err != nil {
			return err
		}
		version = 2
	}

	if version < 3 {
		if err := s.migrateToV3(); err != nil {
			return err
		}
		version = 3
	}

	if version < 4 {
		if err := s.migrateToV4(); err != nil {
			return err
		}
		version = 4
	}

	if _, err := s.db.Exec(`INSERT INTO admin_state(id, password_hash, password_salt, initial_secret, first_boot) VALUES (1, ?, ?, ?, 1) ON CONFLICT(id) DO NOTHING`,
		hash, salt, secret); err != nil {
		return err
	}

	_, err = s.db.Exec(`UPDATE schema_version SET version=? WHERE id=1`, currentSchemaVersion)
	return err
}

func (s *SQLiteStore) migrateToV2() error {
	// Add hostname/vendor/source/last_seen to devices.
	ensure := func(column, typ string) error {
		has, err := s.hasColumn("devices", column)
		if err != nil {
			return err
		}
		if has {
			return nil
		}
		_, err = s.db.Exec(`ALTER TABLE devices ADD COLUMN ` + column + ` ` + typ)
		return err
	}
	if err := ensure("hostname", "TEXT"); err != nil {
		return err
	}
	if err := ensure("vendor", "TEXT"); err != nil {
		return err
	}
	if err := ensure("source", "TEXT"); err != nil {
		return err
	}
	if err := ensure("last_seen", "TEXT"); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) migrateToV3() error {
	if has, _ := s.hasColumn("admin_state", "initial_secret"); !has {
		if _, err := s.db.Exec(`ALTER TABLE admin_state ADD COLUMN initial_secret TEXT`); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) migrateToV4() error {
	// Drop device_id from domain_rules by recreating the table.
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS domain_rules_new (id TEXT PRIMARY KEY, user_id TEXT, pattern TEXT, action TEXT, notes TEXT);`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO domain_rules_new (id, user_id, pattern, action, notes) SELECT id, user_id, pattern, action, notes FROM domain_rules`); err != nil {
		return err
	}
	if _, err = tx.Exec(`DROP TABLE domain_rules`); err != nil {
		return err
	}
	if _, err = tx.Exec(`ALTER TABLE domain_rules_new RENAME TO domain_rules`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) hasColumn(table, column string) (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, nil
}

func (s *SQLiteStore) DataSnapshot() Data {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotUnsafe()
}

// snapshotUnsafe builds a Data struct without taking the mutex; caller must hold s.mu.
func (s *SQLiteStore) snapshotUnsafe() Data {
	data := defaultData()
	row := s.db.QueryRow(`SELECT version FROM schema_version WHERE id=1`)
	_ = row.Scan(&data.Version)

	// Admin
	var hash, salt, updated, initialSecret string
	var firstBootInt int
	_ = s.db.QueryRow(`SELECT password_hash, password_salt, initial_secret, first_boot, updated_at FROM admin_state WHERE id=1`).Scan(&hash, &salt, &initialSecret, &firstBootInt, &updated)
	data.Admin.PasswordHash = hash
	data.Admin.PasswordSalt = salt
	data.Admin.InitialSecret = initialSecret
	data.Admin.FirstBoot = firstBootInt != 0
	if updated != "" {
		if ts, err := time.Parse(time.RFC3339, updated); err == nil {
			data.Admin.UpdatedAt = ts
		}
	}

	// Settings
	var idle int
	var tz string
	_ = s.db.QueryRow(`SELECT idle_timeout_minutes, budget_reset_tz FROM settings WHERE id=1`).Scan(&idle, &tz)
	if idle > 0 {
		data.Settings.IdleTimeoutMinutes = idle
	}
	if tz != "" {
		data.Settings.BudgetResetTZ = tz
	}

	// Users
	data.Users = make(map[string]User)
	rows, err := s.db.Query(`SELECT id, name, daily_budget_minutes, allow_windows, notes, created_at, updated_at FROM users`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var u User
			var allowJSON, created, updated string
			if err := rows.Scan(&u.ID, &u.Name, &u.DailyBudgetMinutes, &allowJSON, &u.Notes, &created, &updated); err == nil {
				_ = json.Unmarshal([]byte(allowJSON), &u.AllowWindows)
				if created != "" {
					u.CreatedAt, _ = time.Parse(time.RFC3339, created)
				}
				if updated != "" {
					u.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
				}
				data.Users[u.ID] = u
			}
		}
	}

	// Devices
	data.Devices = make(map[string]Device)
	devRows, err := s.db.Query(`SELECT id, name, ip, mac, hostname, vendor, source, last_seen, user_id, created_at, updated_at FROM devices`)
	if err == nil {
		defer devRows.Close()
		for devRows.Next() {
			var d Device
			var created, updated, lastSeen string
			if err := devRows.Scan(&d.ID, &d.Name, &d.IP, &d.MAC, &d.Hostname, &d.Vendor, &d.Source, &lastSeen, &d.UserID, &created, &updated); err == nil {
				if created != "" {
					d.CreatedAt, _ = time.Parse(time.RFC3339, created)
				}
				if updated != "" {
					d.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
				}
				if lastSeen != "" {
					d.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
				}
				data.Devices[d.ID] = d
			}
		}
	}

	// Domain rules
	rulesRows, err := s.db.Query(`SELECT id, user_id, pattern, action, notes FROM domain_rules`)
	if err == nil {
		defer rulesRows.Close()
		for rulesRows.Next() {
			var r DomainRule
			if err := rulesRows.Scan(&r.ID, &r.UserID, &r.Pattern, &r.Action, &r.Notes); err == nil {
				data.DomainRules = append(data.DomainRules, r)
			}
		}
	}

	// Sessions
	data.Sessions = make(map[string]SessionState)
	sRows, err := s.db.Query(`SELECT user_id, day, active, start_ts, last_seen FROM sessions`)
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var st SessionState
			var start, last string
			var activeInt int
			if err := sRows.Scan(&st.UserID, &st.Day, &activeInt, &start, &last); err == nil {
				st.Active = activeInt != 0
				if start != "" {
					st.Start, _ = time.Parse(time.RFC3339, start)
				}
				if last != "" {
					st.LastSeen, _ = time.Parse(time.RFC3339, last)
				}
				data.Sessions[st.UserID] = st
			}
		}
	}

	// Usage
	data.Usage = make(map[string]UsageSummary)
	uRows, err := s.db.Query(`SELECT user_id, day, seconds FROM usage`)
	if err == nil {
		defer uRows.Close()
		for uRows.Next() {
			var us UsageSummary
			if err := uRows.Scan(&us.UserID, &us.Day, &us.Seconds); err == nil {
				data.Usage[us.UserID] = us
			}
		}
	}

	// Audit
	aRows, _ := s.db.Query(`SELECT id, ts, actor, action, details FROM audit ORDER BY ts DESC`)
	defer aRows.Close()
	for aRows.Next() {
		var a AuditEvent
		var ts string
		if err := aRows.Scan(&a.ID, &ts, &a.Actor, &a.Action, &a.Details); err == nil {
			if ts != "" {
				a.Timestamp, _ = time.Parse(time.RFC3339, ts)
			}
			data.Audit = append(data.Audit, a)
		}
	}

	return data
}

// UpdateMutate loads current data, applies fn, and writes back transactionally.
func (s *SQLiteStore) UpdateMutate(fn func(data *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := s.snapshotUnsafe()
	if err := fn(&data); err != nil {
		return err
	}
	return s.persist(&data)
}

func (s *SQLiteStore) persist(data *Data) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`UPDATE schema_version SET version=? WHERE id=1`, data.Version); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM admin_state`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO admin_state(id, password_hash, password_salt, initial_secret, first_boot, updated_at) VALUES (1, ?, ?, ?, ?, ?)`,
		data.Admin.PasswordHash, data.Admin.PasswordSalt, data.Admin.InitialSecret, boolToInt(data.Admin.FirstBoot), data.Admin.UpdatedAt.Format(time.RFC3339)); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM settings`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO settings(id, idle_timeout_minutes, budget_reset_tz) VALUES (1, ?, ?)`,
		data.Settings.IdleTimeoutMinutes, data.Settings.BudgetResetTZ); err != nil {
		return err
	}

	if _, err = tx.Exec(`DELETE FROM users`); err != nil {
		return err
	}
	for _, u := range data.Users {
		allowJSON, _ := json.Marshal(u.AllowWindows)
		if _, err = tx.Exec(`INSERT INTO users(id, name, daily_budget_minutes, allow_windows, notes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			u.ID, u.Name, u.DailyBudgetMinutes, string(allowJSON), u.Notes, u.CreatedAt.Format(time.RFC3339), u.UpdatedAt.Format(time.RFC3339)); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(`DELETE FROM devices`); err != nil {
		return err
	}
	for _, d := range data.Devices {
		lastSeen := ""
		if !d.LastSeen.IsZero() {
			lastSeen = d.LastSeen.Format(time.RFC3339)
		}
		if _, err = tx.Exec(`INSERT INTO devices(id, name, ip, mac, hostname, vendor, source, last_seen, user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.ID, d.Name, d.IP, d.MAC, d.Hostname, d.Vendor, d.Source, lastSeen, d.UserID, d.CreatedAt.Format(time.RFC3339), d.UpdatedAt.Format(time.RFC3339)); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(`DELETE FROM domain_rules`); err != nil {
		return err
	}
	for _, r := range data.DomainRules {
		if _, err = tx.Exec(`INSERT INTO domain_rules(id, user_id, pattern, action, notes) VALUES (?, ?, ?, ?, ?)`,
			r.ID, r.UserID, r.Pattern, r.Action, r.Notes); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(`DELETE FROM sessions`); err != nil {
		return err
	}
	for _, sst := range data.Sessions {
		if _, err = tx.Exec(`INSERT INTO sessions(user_id, day, active, start_ts, last_seen) VALUES (?, ?, ?, ?, ?)`,
			sst.UserID, sst.Day, boolToInt(sst.Active), sst.Start.Format(time.RFC3339), sst.LastSeen.Format(time.RFC3339)); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(`DELETE FROM usage`); err != nil {
		return err
	}
	for _, us := range data.Usage {
		if _, err = tx.Exec(`INSERT INTO usage(user_id, day, seconds) VALUES (?, ?, ?)`, us.UserID, us.Day, us.Seconds); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(`DELETE FROM audit`); err != nil {
		return err
	}
	for _, a := range data.Audit {
		if _, err = tx.Exec(`INSERT INTO audit(id, ts, actor, action, details) VALUES (?, ?, ?, ?, ?)`,
			a.ID, a.Timestamp.Format(time.RFC3339), a.Actor, a.Action, a.Details); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// SetAdminPassword stores a salted hash and clears first-boot.
func (s *SQLiteStore) SetAdminPassword(password string) error {
	return s.UpdateMutate(func(d *Data) error {
		return (&JSONStore{}).setAdmin(d, password)
	})
}

// ValidatePassword checks password against the stored salted hash.
func (s *SQLiteStore) ValidatePassword(password string) bool {
	data := s.DataSnapshot()
	return (&JSONStore{}).validate(data, password)
}

// FactoryReset removes the database file.
func (s *SQLiteStore) FactoryReset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.db.Close()
	return os.Remove(s.path)
}

// helper to reuse hashing logic
func (s *JSONStore) setAdmin(d *Data, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	d.Admin.PasswordSalt = ""
	d.Admin.PasswordHash = string(hash)
	d.Admin.InitialSecret = ""
	d.Admin.FirstBoot = false
	d.Admin.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *JSONStore) validate(data Data, password string) bool {
	return validatePasswordRecord(data.Admin, password)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Ensure SQLiteStore implements Store.
var _ Store = (*SQLiteStore)(nil)

// Ensure JSONStore implements Store.
var _ Store = (*JSONStore)(nil)

// ErrNotFound used for missing rows.
var ErrNotFound = errors.New("not found")
