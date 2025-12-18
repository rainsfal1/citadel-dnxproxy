package accounting

import (
	"fmt"
	"time"

	"dnsproxy/internal/policy"
)

// Clock allows deterministic testing.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Manager tracks user sessions and budgets.
type Manager struct {
	store       policy.Store
	runtime     *policy.Runtime
	clock       Clock
	idleTimeout time.Duration
}

// Decision captures accounting results.
type Decision struct {
	User          policy.User
	UsageSeconds  int64
	BudgetSeconds int64
	Blocked       bool
	Reason        policy.Reason
}

// NewManager builds a Manager from store and compiled policy runtime.
func NewManager(store policy.Store, rt *policy.Runtime, idleMinutes int, clock Clock) *Manager {
	if clock == nil {
		clock = realClock{}
	}
	if rt == nil {
		rt = &policy.Runtime{BudgetTz: time.Local}
	}
	if idleMinutes == 0 {
		idleMinutes = 10
	}
	return &Manager{
		store:       store,
		runtime:     rt,
		clock:       clock,
		idleTimeout: time.Duration(idleMinutes) * time.Minute,
	}
}

// ProcessRequest updates session state and returns whether the user is within budget.
func (m *Manager) ProcessRequest(user policy.User) (Decision, error) {
	now := m.clock.Now()
	decision := Decision{
		User:          user,
		BudgetSeconds: int64(user.DailyBudgetMinutes) * 60,
	}
	loc := time.Local
	if m.runtime != nil && m.runtime.BudgetTz != nil {
		loc = m.runtime.BudgetTz
	}
	day := now.In(loc).Format("2006-01-02")
	return m.applyForDay(user.ID, day, now, &decision)
}

func (m *Manager) applyForDay(userID, day string, now time.Time, decision *Decision) (Decision, error) {
	return *decision, m.store.UpdateMutate(func(d *policy.Data) error {
		usage := d.Usage[userID]
		if usage.Day != day {
			usage = policy.UsageSummary{UserID: userID, Day: day, Seconds: 0}
		}
		session := d.Sessions[userID]
		if session.UserID == "" {
			session = policy.SessionState{UserID: userID, Day: day}
		}
		// If the session is on a previous day, close it and reset usage/session timestamps.
		if session.Day != "" && session.Day != day {
			session.Active = false
			session.Start = time.Time{}
			session.LastSeen = time.Time{}
		}
		session.Day = day

		// Apply idle timeout accrual.
		if session.Active {
			delta := now.Sub(session.LastSeen)
			if delta < 0 {
				delta = 0
			}
			if delta > m.idleTimeout {
				// Close session at last_seen; time after idle timeout will be accounted when a new session starts.
				usage.Seconds += int64(session.LastSeen.Sub(session.Start).Seconds())
				session.Active = false
			} else {
				usage.Seconds += int64(delta.Seconds())
			}
		}

		// Check budget with in-flight time counted above.
		if decision.BudgetSeconds > 0 && usage.Seconds >= decision.BudgetSeconds {
			decision.Blocked = true
			decision.Reason = policy.ReasonBudgetExceeded
			d.Sessions[userID] = session
			d.Usage[userID] = usage
			return nil
		}

		if !session.Active {
			session.Active = true
			session.Start = now
		}
		session.LastSeen = now

		decision.UsageSeconds = usage.Seconds
		d.Sessions[userID] = session
		d.Usage[userID] = usage
		return nil
	})
}

// CurrentUsage returns the persisted totals for a user/day.
func (m *Manager) CurrentUsage(userID string) (policy.UsageSummary, policy.SessionState) {
	data := m.store.DataSnapshot()
	return data.Usage[userID], data.Sessions[userID]
}

// ResetForTest rewinds the manager with a runtime snapshot (for tests).
func (m *Manager) ResetForTest(rt *policy.Runtime) {
	m.runtime = rt
}

// DebugString helps tests display state.
func (m *Manager) DebugString(userID string) string {
	usage, sess := m.CurrentUsage(userID)
	return fmt.Sprintf("usage=%d day=%s active=%v start=%s last=%s", usage.Seconds, usage.Day, sess.Active, sess.Start.Format(time.RFC3339), sess.LastSeen.Format(time.RFC3339))
}
