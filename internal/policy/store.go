package policy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	currentSchemaVersion = 4
	CurrentSchemaVersion = currentSchemaVersion
)

// Data is the persisted representation of household policy.
type Data struct {
	Version     int                     `json:"version"`
	Admin       AdminState              `json:"admin"`
	Users       map[string]User         `json:"users"`
	Devices     map[string]Device       `json:"devices"`
	DomainRules []DomainRule            `json:"domain_rules"`
	Sessions    map[string]SessionState `json:"sessions"`
	Usage       map[string]UsageSummary `json:"usage"`
	Settings    Settings                `json:"settings"`
	Audit       []AuditEvent            `json:"audit"`
}

// Store defines required persistence operations.
type Store interface {
	DataSnapshot() Data
	UpdateMutate(func(data *Data) error) error
	SetAdminPassword(password string) error
	ValidatePassword(password string) bool
	FactoryReset() error
}

// JSONStore is a simple file-backed store used for tests/fallback.
type JSONStore struct {
	path string
	mu   sync.Mutex
	data Data
}

// LoadOrInitJSON opens the JSON store at the given path or creates a new one with defaults.
func LoadOrInitJSON(path string) (*JSONStore, error) {
	s := &JSONStore{path: path}
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			s.data = defaultData()
			return s, s.Save()
		}
		return nil, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		s.data = defaultData()
		return s, s.Save()
	}
	if err := json.Unmarshal(payload, &s.data); err != nil {
		return nil, err
	}
	if s.data.Version == 0 {
		s.data.Version = currentSchemaVersion
	}
	return s, nil
}

// Save writes the current data to disk atomically.
func (s *JSONStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tmp := filepath.Join(filepath.Dir(s.path), fmt.Sprintf(".%s.tmp", filepath.Base(s.path)))
	payload, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, payload, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// DataSnapshot safely returns a copy of the current data.
func (s *JSONStore) DataSnapshot() Data {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a shallow copy with cloned maps so callers cannot mutate in-memory state.
	d := s.data
	d.Users = cloneUsers(s.data.Users)
	d.Devices = cloneDevices(s.data.Devices)
	d.DomainRules = append([]DomainRule(nil), s.data.DomainRules...)
	d.Sessions = cloneSessions(s.data.Sessions)
	d.Usage = cloneUsage(s.data.Usage)
	d.Settings = s.data.Settings
	PopulateUserDeviceRefs(&d)
	return d
}

// UpdateMutate runs a mutation function under lock and saves.
func (s *JSONStore) UpdateMutate(fn func(data *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.data); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(s.path), fmt.Sprintf(".%s.tmp", filepath.Base(s.path)))
	payload, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, payload, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// FactoryReset removes the store file; callers can recreate defaults after calling this.
func (s *JSONStore) FactoryReset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(s.path)
}

// SetAdminPassword stores a salted hash and clears first-boot.
func (s *JSONStore) SetAdminPassword(password string) error {
	return s.UpdateMutate(func(d *Data) error {
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
	})
}

// ValidatePassword checks password against the stored salted hash.
func (s *JSONStore) ValidatePassword(password string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return validatePasswordRecord(s.data.Admin, password)
}

func (s *JSONStore) ensureDir() error {
	dir := filepath.Dir(s.path)
	return os.MkdirAll(dir, 0755)
}

func defaultData() Data {
	salt, hash := defaultAdminCreds()
	secret := randomSecret()
	return Data{
		Version: currentSchemaVersion,
		Admin: AdminState{
			FirstBoot:     true,
			PasswordSalt:  salt,
			PasswordHash:  hash,
			InitialSecret: secret,
		},
		Users:    make(map[string]User),
		Devices:  make(map[string]Device),
		Sessions: make(map[string]SessionState),
		Usage:    make(map[string]UsageSummary),
		Settings: Settings{
			IdleTimeoutMinutes: 10,
			BudgetResetTZ:      "",
		},
	}
}

func defaultAdminCreds() (string, string) {
	saltBytes := []byte("default-admin-salt")
	hash := sha256.Sum256(append(saltBytes, []byte("admin")...))
	return hex.EncodeToString(saltBytes), hex.EncodeToString(hash[:])
}

// IsDefaultAdmin reports whether the admin record is still using the factory default hash/salt.
func IsDefaultAdmin(admin AdminState) bool {
	salt, hash := defaultAdminCreds()
	if admin.PasswordHash == "" && admin.PasswordSalt == "" {
		return true
	}
	return admin.PasswordHash == hash && admin.PasswordSalt == salt
}

func randomSecret() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "changeme"
	}
	return hex.EncodeToString(buf)
}

func validatePasswordRecord(admin AdminState, password string) bool {
	if admin.PasswordHash == "" {
		return false
	}
	// Prefer bcrypt if present.
	if strings.HasPrefix(admin.PasswordHash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)) == nil
	}
	// Legacy sha256
	if admin.PasswordSalt == "" {
		return false
	}
	salt, err := hex.DecodeString(admin.PasswordSalt)
	if err != nil {
		return false
	}
	hash := sha256.Sum256(append(salt, []byte(password)...))
	if hex.EncodeToString(hash[:]) == admin.PasswordHash {
		return true
	}
	// Also allow initial secret for first boot.
	return admin.InitialSecret != "" && password == admin.InitialSecret
}

// PopulateUserDeviceRefs hydrates user.DeviceIDs based on device ownership.
func PopulateUserDeviceRefs(data *Data) {
	if data == nil {
		return
	}
	deviceIDsByUser := make(map[string][]string)
	for id, dev := range data.Devices {
		if dev.UserID == "" {
			continue
		}
		deviceIDsByUser[dev.UserID] = append(deviceIDsByUser[dev.UserID], id)
	}
	for uid, ids := range deviceIDsByUser {
		sort.Strings(ids)
		if u, ok := data.Users[uid]; ok {
			u.DeviceIDs = ids
			data.Users[uid] = u
		}
	}
	for uid, u := range data.Users {
		if _, ok := deviceIDsByUser[uid]; !ok {
			u.DeviceIDs = nil
			data.Users[uid] = u
		}
	}
}

func cloneUsers(src map[string]User) map[string]User {
	if src == nil {
		return nil
	}
	dst := make(map[string]User, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneDevices(src map[string]Device) map[string]Device {
	if src == nil {
		return nil
	}
	dst := make(map[string]Device, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneSessions(src map[string]SessionState) map[string]SessionState {
	if src == nil {
		return nil
	}
	dst := make(map[string]SessionState, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneUsage(src map[string]UsageSummary) map[string]UsageSummary {
	if src == nil {
		return nil
	}
	dst := make(map[string]UsageSummary, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
