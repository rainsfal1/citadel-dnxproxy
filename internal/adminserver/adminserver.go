package adminserver

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"dnsproxy/internal/policy"

	"github.com/google/uuid"
)

//go:embed ui/*
var uiFS embed.FS

// Server exposes a local-only admin API for managing policy state.
type Server struct {
	store      policy.Store
	tokenTTL   time.Duration
	sessionTTL time.Duration
	tokens     map[string]time.Time // legacy bearer support
	sessions   map[string]session
	mu         sync.Mutex
	onChange   func()
	loggedSeed bool
}

type session struct {
	Expires  time.Time
	CSRF     string
	Username string
}

// New builds a Server with the given policy store.
func New(store policy.Store) *Server {
	return &Server{
		store:      store,
		tokenTTL:   time.Hour,
		sessionTTL: 12 * time.Hour,
		tokens:     make(map[string]time.Time),
		sessions:   make(map[string]session),
	}
}

// SetOnChange sets a callback invoked after successful policy mutations.
func (s *Server) SetOnChange(fn func()) {
	s.onChange = fn
}

// Start starts the admin HTTP server on the given address.
func (s *Server) Start(addr string) *http.Server {
	mux := http.NewServeMux()
	s.StartHandlers(mux)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[WARN] admin server stopped: %v", err)
		}
	}()
	return server
}

// StartHandlers registers handlers on the provided mux (useful for tests).
func (s *Server) StartHandlers(mux *http.ServeMux) {
	s.maybeLogInitialSecret()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/login", s.login)
	mux.HandleFunc("/logout", s.logout)
	mux.Handle("/admin/password", s.auth(http.HandlerFunc(s.setPassword)))
	mux.Handle("/policy", s.auth(http.HandlerFunc(s.getPolicy)))
	mux.Handle("/users", s.auth(http.HandlerFunc(s.usersHandler)))
	mux.Handle("/users/", s.auth(http.HandlerFunc(s.userHandler)))
	mux.Handle("/devices", s.auth(http.HandlerFunc(s.devicesHandler)))
	mux.Handle("/devices/", s.auth(http.HandlerFunc(s.deviceHandler)))
	mux.Handle("/domainrules", s.auth(http.HandlerFunc(s.domainRulesHandler)))
	mux.Handle("/domainrules/", s.auth(http.HandlerFunc(s.domainRuleHandler)))
	// Static UI
	assetFS, err := fs.Sub(uiFS, "ui")
	if err != nil {
		log.Printf("[WARN] admin UI assets unavailable: %v", err)
	} else {
		fileServer := http.FileServer(http.FS(assetFS))
		mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
		})
		mux.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
			// Serve index for base path; defer to file server for assets.
			if r.URL.Path == "/admin/" || r.URL.Path == "/admin/index.html" {
				data, err := fs.ReadFile(assetFS, "index.html")
				if err != nil {
					writeError(w, http.StatusInternalServerError, "ui not found")
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}
			http.StripPrefix("/admin/", fileServer).ServeHTTP(w, r)
		})
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Username != "" && req.Username != "admin" {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !s.store.ValidatePassword(req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	snap := s.store.DataSnapshot()
	mustChange := snap.Admin.FirstBoot || policy.IsDefaultAdmin(snap.Admin) || snap.Admin.InitialSecret != ""
	token := uuid.NewString()
	s.mu.Lock()
	s.tokens[token] = time.Now().Add(s.tokenTTL)
	// session cookie
	sessID := uuid.NewString()
	csrf := randomToken()
	s.sessions[sessID] = session{
		Expires:  time.Now().Add(s.sessionTTL),
		CSRF:     csrf,
		Username: "admin",
	}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    sessID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"token":       token,
		"csrf_token":  csrf,
		"first_boot":  snap.Admin.FirstBoot,
		"must_change": mustChange,
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if c, err := r.Cookie("admin_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) setPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password required")
		return
	}
	if err := s.store.SetAdminPassword(req.Password); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.notifyChange()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if admin := s.store.DataSnapshot().Admin; admin.FirstBoot && r.URL.Path != "/admin/password" && r.URL.Path != "/login" && r.URL.Path != "/logout" {
			writeError(w, http.StatusForbidden, "change password to continue (first boot)")
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			// Try session cookie
			if sess, ok := s.sessionFromRequest(r); ok {
				if r.Method != http.MethodGet && r.Method != http.MethodHead {
					if csrf := r.Header.Get("X-CSRF-Token"); csrf == "" || csrf != sess.CSRF {
						writeError(w, http.StatusForbidden, "csrf required")
						return
					}
				}
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		token := strings.TrimSpace(auth[len("bearer "):])
		if !s.validToken(token) {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) validToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.tokens, token)
		return false
	}
	return true
}

func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	data := s.store.DataSnapshot()
	policy.PopulateUserDeviceRefs(&data)
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var u policy.User
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if u.ID == "" {
			u.ID = uuid.NewString()
		}
		u.CreatedAt = time.Now().UTC()
		u.UpdatedAt = u.CreatedAt
		err := s.store.UpdateMutate(func(d *policy.Data) error {
			if d.Users == nil {
				d.Users = make(map[string]policy.User)
			}
			d.Users[u.ID] = u
			// Assign devices to this user if provided.
			for _, devID := range u.DeviceIDs {
				dev, ok := d.Devices[devID]
				if !ok {
					return errors.New("device not found: " + devID)
				}
				dev.UserID = u.ID
				dev.UpdatedAt = time.Now().UTC()
				d.Devices[devID] = dev
			}
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.notifyChange()
		writeJSON(w, http.StatusCreated, u)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) userHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/users/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req policy.User
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		err := s.store.UpdateMutate(func(d *policy.Data) error {
			cur, ok := d.Users[id]
			if !ok {
				return errors.New("user not found")
			}
			if req.Name != "" {
				cur.Name = req.Name
			}
			if req.DailyBudgetMinutes > 0 {
				cur.DailyBudgetMinutes = req.DailyBudgetMinutes
			}
			if req.AllowWindows != nil {
				cur.AllowWindows = req.AllowWindows
			}
			if req.Notes != "" {
				cur.Notes = req.Notes
			}
			// Reassign devices if a device list is provided.
			if req.DeviceIDs != nil {
				// Clear current assignments
				for id, dev := range d.Devices {
					if dev.UserID == cur.ID {
						dev.UserID = ""
						dev.UpdatedAt = time.Now().UTC()
						d.Devices[id] = dev
					}
				}
				// Apply new assignments
				for _, devID := range req.DeviceIDs {
					dev, ok := d.Devices[devID]
					if !ok {
						return errors.New("device not found: " + devID)
					}
					dev.UserID = cur.ID
					dev.UpdatedAt = time.Now().UTC()
					d.Devices[devID] = dev
				}
				cur.DeviceIDs = req.DeviceIDs
			}
			cur.UpdatedAt = time.Now().UTC()
			d.Users[id] = cur
			return nil
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.notifyChange()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case http.MethodDelete:
		err := s.store.UpdateMutate(func(d *policy.Data) error {
			delete(d.Users, id)
			for devID, dev := range d.Devices {
				if dev.UserID == id {
					dev.UserID = ""
					dev.UpdatedAt = time.Now().UTC()
					d.Devices[devID] = dev
				}
			}
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.notifyChange()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) devicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var d policy.Device
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	d.CreatedAt = time.Now().UTC()
	d.UpdatedAt = d.CreatedAt
	if d.Source == "" {
		d.Source = "manual"
	}
	if d.LastSeen.IsZero() {
		d.LastSeen = d.CreatedAt
	}
	err := s.store.UpdateMutate(func(data *policy.Data) error {
		if data.Devices == nil {
			data.Devices = make(map[string]policy.Device)
		}
		data.Devices[d.ID] = d
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.notifyChange()
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) deviceHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/devices/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req policy.Device
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		err := s.store.UpdateMutate(func(d *policy.Data) error {
			cur, ok := d.Devices[id]
			if !ok {
				return errors.New("device not found")
			}
			if req.Name != "" {
				cur.Name = req.Name
			}
			if req.IP != "" {
				cur.IP = req.IP
			}
			if req.MAC != "" {
				cur.MAC = req.MAC
			}
			if req.UserID != "" {
				cur.UserID = req.UserID
			}
			if req.Hostname != "" {
				cur.Hostname = req.Hostname
			}
			if req.Vendor != "" {
				cur.Vendor = req.Vendor
			}
			if req.Source != "" {
				cur.Source = req.Source
			}
			if !req.LastSeen.IsZero() {
				cur.LastSeen = req.LastSeen
			} else {
				cur.LastSeen = time.Now().UTC()
			}
			cur.UpdatedAt = time.Now().UTC()
			d.Devices[id] = cur
			return nil
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.notifyChange()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case http.MethodDelete:
		err := s.store.UpdateMutate(func(d *policy.Data) error {
			delete(d.Devices, id)
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.notifyChange()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) domainRulesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var dr policy.DomainRule
	if err := json.NewDecoder(r.Body).Decode(&dr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if dr.ID == "" {
		dr.ID = uuid.NewString()
	}
	if dr.UserID == "" || dr.Pattern == "" || dr.Action == "" {
		writeError(w, http.StatusBadRequest, "user_id, pattern, action required")
		return
	}
	err := s.store.UpdateMutate(func(d *policy.Data) error {
		d.DomainRules = append(d.DomainRules, dr)
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.notifyChange()
	writeJSON(w, http.StatusCreated, dr)
}

func (s *Server) domainRuleHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/domainrules/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		err := s.store.UpdateMutate(func(d *policy.Data) error {
			filtered := d.DomainRules[:0]
			for _, r := range d.DomainRules {
				if r.ID == id {
					continue
				}
				filtered = append(filtered, r)
			}
			d.DomainRules = filtered
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.notifyChange()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case http.MethodPut:
		var req policy.DomainRule
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		err := s.store.UpdateMutate(func(d *policy.Data) error {
			updated := false
			for i, r := range d.DomainRules {
				if r.ID != id {
					continue
				}
				if req.UserID != "" {
					r.UserID = req.UserID
				}
				if req.Pattern != "" {
					r.Pattern = req.Pattern
				}
				if req.Action != "" {
					r.Action = req.Action
				}
				if req.Notes != "" {
					r.Notes = req.Notes
				}
				d.DomainRules[i] = r
				updated = true
				break
			}
			if !updated {
				return errors.New("rule not found")
			}
			return nil
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.notifyChange()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) notifyChange() {
	if s.onChange != nil {
		go s.onChange()
	}
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return uuid.NewString()
	}
	return hex.EncodeToString(b)
}

func (s *Server) sessionFromRequest(r *http.Request) (session, bool) {
	c, err := r.Cookie("admin_session")
	if err != nil {
		return session{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[c.Value]
	if !ok {
		return session{}, false
	}
	if time.Now().After(sess.Expires) {
		delete(s.sessions, c.Value)
		return session{}, false
	}
	return sess, true
}

func (s *Server) maybeLogInitialSecret() {
	if s.store == nil || s.loggedSeed {
		return
	}
	data := s.store.DataSnapshot()
	if data.Admin.FirstBoot && data.Admin.InitialSecret == "" {
		_ = s.store.UpdateMutate(func(d *policy.Data) error {
			if d.Admin.InitialSecret == "" {
				d.Admin.InitialSecret = randomToken()
			}
			return nil
		})
		data = s.store.DataSnapshot()
	}
	if data.Admin.FirstBoot && data.Admin.InitialSecret != "" {
		log.Printf("[WARN] First boot admin secret: %s (change immediately)", data.Admin.InitialSecret)
		s.loggedSeed = true
	}
}
