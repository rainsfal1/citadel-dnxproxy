package accounting

import (
	"testing"
	"time"

	"dnsproxy/internal/policy"
)

type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time          { return f.now }
func (f *fakeClock) Advance(d time.Duration) { f.now = f.now.Add(d) }

func newTestStore(t *testing.T) policy.Store {
	t.Helper()
	path := t.TempDir() + "/policy.json"
	store, err := policy.LoadOrInitJSON(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	return store
}

func TestSessionAccrualAndIdleTimeout(t *testing.T) {
	store := newTestStore(t)
	rt := policy.Compile(policy.Data{
		Version:  policy.CurrentSchemaVersion,
		Users:    map[string]policy.User{"u1": {ID: "u1", Name: "Kid", DailyBudgetMinutes: 5}},
		Settings: policy.Settings{IdleTimeoutMinutes: 1},
	})
	clock := &fakeClock{now: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)}
	mgr := NewManager(store, rt, rt.Settings.IdleTimeoutMinutes, clock)

	user := rt.UsersByID["u1"]

	if dec, err := mgr.ProcessRequest(user); err != nil || dec.Blocked {
		t.Fatalf("first request should pass, err=%v blocked=%v", err, dec.Blocked)
	}
	clock.Advance(30 * time.Second)
	if dec, err := mgr.ProcessRequest(user); err != nil || dec.Blocked {
		t.Fatalf("second request should pass, err=%v blocked=%v", err, dec.Blocked)
	}
	usage, _ := mgr.CurrentUsage("u1")
	if usage.Seconds < 29 || usage.Seconds > 31 {
		t.Fatalf("usage expected ~30s, got %d", usage.Seconds)
	}

	clock.Advance(2 * time.Minute)
	if dec, err := mgr.ProcessRequest(user); err != nil || dec.Blocked {
		t.Fatalf("after idle, request should pass, err=%v blocked=%v", err, dec.Blocked)
	}
	usage, _ = mgr.CurrentUsage("u1")
	if usage.Seconds < 29 || usage.Seconds > 90 {
		t.Fatalf("usage should include prior session, got %d", usage.Seconds)
	}
}

func TestBudgetBlocksAcrossDevices(t *testing.T) {
	store := newTestStore(t)
	rt := policy.Compile(policy.Data{
		Version: policy.CurrentSchemaVersion,
		Users:   map[string]policy.User{"u1": {ID: "u1", Name: "Kid", DailyBudgetMinutes: 1}},
		Settings: policy.Settings{
			IdleTimeoutMinutes: 5,
		},
	})
	clock := &fakeClock{now: time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC)}
	mgr := NewManager(store, rt, rt.Settings.IdleTimeoutMinutes, clock)
	user := rt.UsersByID["u1"]

	if dec, err := mgr.ProcessRequest(user); err != nil || dec.Blocked {
		t.Fatalf("first request blocked=%v err=%v", dec.Blocked, err)
	}
	clock.Advance(70 * time.Second)
	dec, err := mgr.ProcessRequest(user)
	if err != nil {
		t.Fatalf("process request: %v", err)
	}
	if !dec.Blocked {
		t.Fatalf("expected budget exhaustion to block")
	}
}

func TestDailyReset(t *testing.T) {
	store := newTestStore(t)
	rt := policy.Compile(policy.Data{
		Version:  policy.CurrentSchemaVersion,
		Users:    map[string]policy.User{"u1": {ID: "u1", Name: "Kid", DailyBudgetMinutes: 1}},
		Settings: policy.Settings{IdleTimeoutMinutes: 1, BudgetResetTZ: "UTC"},
	})
	clock := &fakeClock{now: time.Date(2024, 1, 1, 22, 0, 0, 0, time.UTC)}
	mgr := NewManager(store, rt, rt.Settings.IdleTimeoutMinutes, clock)
	user := rt.UsersByID["u1"]

	if dec, err := mgr.ProcessRequest(user); err != nil || dec.Blocked {
		t.Fatalf("request failed: %v blocked=%v", err, dec.Blocked)
	}
	clock.Advance(59 * time.Second)
	_, _ = mgr.ProcessRequest(user)
	clock.Advance(2 * time.Hour)
	beforeUsage, _ := mgr.CurrentUsage("u1")
	if beforeUsage.Seconds == 0 {
		t.Fatalf("expected usage accrued before reset")
	}
	if dec, err := mgr.ProcessRequest(user); err != nil {
		t.Fatalf("post-midnight request err=%v", err)
	} else if dec.Blocked {
		t.Fatalf("should reset budget after day change (usage=%d budget=%d reason=%s)", dec.UsageSeconds, dec.BudgetSeconds, dec.Reason)
	}
	afterUsage, _ := mgr.CurrentUsage("u1")
	if afterUsage.Seconds != 0 {
		t.Fatalf("usage should reset on new day, got %d", afterUsage.Seconds)
	}
}
