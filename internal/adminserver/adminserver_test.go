package adminserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dnsproxy/internal/policy"
)

func newServer(t *testing.T) *Server {
	store, err := policy.LoadOrInitJSON(t.TempDir() + "/policy.json")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// Set initial password
	if err := store.SetAdminPassword("secret"); err != nil {
		t.Fatalf("set pwd: %v", err)
	}
	// Seed a device to allow assignment in tests.
	_ = store.UpdateMutate(func(d *policy.Data) error {
		if d.Devices == nil {
			d.Devices = make(map[string]policy.Device)
		}
		d.Devices["dev1"] = policy.Device{ID: "dev1", Name: "tablet"}
		return nil
	})
	return New(store)
}

func TestAuthFlow(t *testing.T) {
	srv := newServer(t)
	mux := http.NewServeMux()
	srv.StartHandlers(mux)

	// login
	loginBody := `{"password":"secret"}`
	req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(loginBody))
	w := httptest.NewRecorder()
	srv.login(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	token, _ := resp["token"].(string)
	csrf, _ := resp["csrf_token"].(string)
	if token == "" || csrf == "" {
		t.Fatalf("missing token or csrf")
	}
	cookie := w.Result().Cookies()[0]

	req2 := httptest.NewRequest("GET", "/policy", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("policy status %d body=%s", w2.Code, w2.Body.String())
	}

	// Password change with CSRF + cookie
	req3 := httptest.NewRequest("POST", "/admin/password", bytes.NewBufferString(`{"password":"newsecret"}`))
	req3.AddCookie(cookie)
	req3.Header.Set("X-CSRF-Token", csrf)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("password change status %d body=%s", w3.Code, w3.Body.String())
	}
}

func TestUIRoot(t *testing.T) {
	srv := newServer(t)
	mux := http.NewServeMux()
	srv.StartHandlers(mux)
	req := httptest.NewRequest("GET", "/admin/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ui root status %d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthRequired(t *testing.T) {
	srv := newServer(t)
	mux := http.NewServeMux()
	srv.StartHandlers(mux)
	req := httptest.NewRequest("GET", "/policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPolicyChangeTriggersReload(t *testing.T) {
	srv := newServer(t)
	triggered := make(chan struct{}, 1)
	srv.SetOnChange(func() {
		triggered <- struct{}{}
	})
	mux := http.NewServeMux()
	srv.StartHandlers(mux)
	// login
	req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(`{"password":"secret"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	cookie := w.Result().Cookies()[0]
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	csrf := resp["csrf_token"].(string)

	// add domain rule
	req2 := httptest.NewRequest("POST", "/domainrules", bytes.NewBufferString(`{"user_id":"u","pattern":"*.test","action":"block"}`))
	req2.AddCookie(cookie)
	req2.Header.Set("X-CSRF-Token", csrf)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("domain rule status %d body=%s", w2.Code, w2.Body.String())
	}
	select {
	case <-triggered:
	case <-time.After(time.Second):
		t.Fatalf("onChange not triggered")
	}
}
