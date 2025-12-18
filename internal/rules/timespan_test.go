package rules

import (
	"testing"
	"time"

	cfg "dnsproxy/internal/config"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

func TestTimeSpanCrossMidnight(t *testing.T) {
	conf := cfg.Config{
		DefaultRule: cfg.ActionTypePass,
		OnErrorRule: cfg.ActionTypeBlockedSiteBan,
		Hosts: []cfg.Host{
			{
				Name: "client",
				Rules: []cfg.Rule{
					{Type: cfg.ActionTypeBlockedDevice, TimeSpan: "22:00-06:00"},
				},
			},
		},
		NameServers: []cfg.NameServer{{IP: "8.8.8.8:53"}},
	}
	re, _ := NewRulesEngine(&conf)
	re.WithClock(fakeClock{now: time.Date(2023, 11, 10, 23, 0, 0, 0, time.Local)})
	act, _ := re.Evaluate("x", "client")
	if act != cfg.ActionTypeBlockedDevice {
		t.Fatalf("expected block during overnight window, got %v", act)
	}
	re.WithClock(fakeClock{now: time.Date(2023, 11, 11, 7, 0, 0, 0, time.Local)})
	act, _ = re.Evaluate("x", "client")
	if act != cfg.ActionTypePass {
		t.Fatalf("expected pass outside overnight window, got %v", act)
	}
}

func TestTimeSpanWeekdays(t *testing.T) {
	conf := cfg.Config{
		DefaultRule: cfg.ActionTypePass,
		OnErrorRule: cfg.ActionTypeBlockedSiteBan,
		Hosts: []cfg.Host{
			{
				Name: "client",
				Rules: []cfg.Rule{
					{Type: cfg.ActionTypeBlockedDevice, TimeSpan: "mon-fri@09:00-17:00"},
				},
			},
		},
		NameServers: []cfg.NameServer{{IP: "8.8.8.8:53"}},
	}
	re, _ := NewRulesEngine(&conf)
	re.WithClock(fakeClock{now: time.Date(2023, 11, 6, 10, 0, 0, 0, time.Local)}) // Monday
	act, _ := re.Evaluate("x", "client")
	if act != cfg.ActionTypeBlockedDevice {
		t.Fatalf("expected block on weekday inside window, got %v", act)
	}
	re.WithClock(fakeClock{now: time.Date(2023, 11, 5, 10, 0, 0, 0, time.Local)}) // Sunday
	act, _ = re.Evaluate("x", "client")
	if act != cfg.ActionTypePass {
		t.Fatalf("expected pass on weekend, got %v", act)
	}
}

func TestTimeSpanTimezone(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	conf := cfg.Config{
		DefaultRule: cfg.ActionTypePass,
		OnErrorRule: cfg.ActionTypeBlockedSiteBan,
		Hosts: []cfg.Host{
			{
				Name: "client",
				Rules: []cfg.Rule{
					{Type: cfg.ActionTypeBlockedDevice, TimeSpan: "22:00-23:59|tz=America/New_York"},
				},
			},
		},
		NameServers: []cfg.NameServer{{IP: "8.8.8.8:53"}},
	}
	re, _ := NewRulesEngine(&conf)
	re.WithClock(fakeClock{now: time.Date(2023, 11, 10, 3, 0, 0, 0, loc)}) // 3am UTC? but loc handles
	act, _ := re.Evaluate("x", "client")
	// 3am ET is outside window
	if act != cfg.ActionTypePass {
		t.Fatalf("expected pass outside timezone window, got %v", act)
	}
	re.WithClock(fakeClock{now: time.Date(2023, 11, 10, 22, 30, 0, 0, loc)})
	act, _ = re.Evaluate("x", "client")
	if act != cfg.ActionTypeBlockedDevice {
		t.Fatalf("expected block inside timezone window, got %v", act)
	}
}
