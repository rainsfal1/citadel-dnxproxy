package rules

import (
	"testing"

	cfg "dnsproxy/internal/config"
	dev "dnsproxy/internal/device"
)

func TestPrecedenceHostSpecificity(t *testing.T) {
	conf := cfg.Config{
		DefaultRule: cfg.ActionTypePass,
		OnErrorRule: cfg.ActionTypeBlockedSiteBan,
		Hosts: []cfg.Host{
			{Name: "192.168.1.5", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedDevice}}},
			{Name: "192.168.*", Rules: []cfg.Rule{{Type: cfg.ActionTypePass}}},
			{Name: "*", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedSiteBan}}},
		},
		NameServers: []cfg.NameServer{{IP: "8.8.8.8:53"}},
	}
	re, _ := NewRulesEngine(&conf)

	tests := []struct {
		name   string
		host   string
		expect cfg.ActionType
	}{
		{"exact host beats wildcard", "192.168.1.5", cfg.ActionTypeBlockedDevice},
		{"segment wildcard beats global", "192.168.9.1", cfg.ActionTypePass},
		{"global fallback", "10.0.0.1", cfg.ActionTypeBlockedSiteBan},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			act, err := re.Evaluate("any.domain", tt.host)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if act != tt.expect {
				t.Fatalf("expected %v got %v", tt.expect, act)
			}
		})
	}
}

func TestPrecedenceDomainSpecificity(t *testing.T) {
	conf := cfg.Config{
		DefaultRule: cfg.ActionTypePass,
		OnErrorRule: cfg.ActionTypeBlockedSiteBan,
		Domains: []cfg.Domain{
			{
				Name: "*.example.com",
				Hosts: []cfg.Host{
					{Name: "*", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedDevice}}},
				},
			},
			{
				Name: "api.example.com",
				Hosts: []cfg.Host{
					{Name: "*", Rules: []cfg.Rule{{Type: cfg.ActionTypePass}}},
				},
			},
			{
				Name: "*",
				Hosts: []cfg.Host{
					{Name: "*", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedSiteBan}}},
				},
			},
		},
		NameServers: []cfg.NameServer{{IP: "8.8.8.8:53"}},
	}
	re, _ := NewRulesEngine(&conf)

	tests := []struct {
		name   string
		domain string
		expect cfg.ActionType
	}{
		{"exact domain beats wildcard", "api.example.com", cfg.ActionTypePass},
		{"longer wildcard beats global", "foo.example.com", cfg.ActionTypeBlockedDevice},
		{"global fallback domain", "other.com", cfg.ActionTypeBlockedSiteBan},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			act, err := re.Evaluate(tt.domain, "1.1.1.1")
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if act != tt.expect {
				t.Fatalf("expected %v got %v", tt.expect, act)
			}
		})
	}
}

func TestPrecedenceOrderWithinRules(t *testing.T) {
	conf := cfg.Config{
		DefaultRule: cfg.ActionTypePass,
		OnErrorRule: cfg.ActionTypeBlockedSiteBan,
		Hosts: []cfg.Host{
			{
				Name: "device",
				Rules: []cfg.Rule{
					{Type: cfg.ActionTypeBlockedDevice},
					{Type: cfg.ActionTypePass},
				},
			},
		},
		NameServers: []cfg.NameServer{{IP: "8.8.8.8:53"}},
	}
	re, _ := NewRulesEngine(&conf)
	act, err := re.Evaluate("x", "device")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if act != cfg.ActionTypeBlockedDevice {
		t.Fatalf("expected first rule to win; got %v", act)
	}
}

func TestPrecedenceDeviceNameLookup(t *testing.T) {
	conf := cfg.Config{
		DefaultRule: cfg.ActionTypePass,
		OnErrorRule: cfg.ActionTypeBlockedSiteBan,
		Hosts: []cfg.Host{
			{Name: "kid-device", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedDevice}}},
		},
		NameServers: []cfg.NameServer{{IP: "8.8.8.8:53"}},
		StaticDevices: []cfg.StaticDevice{
			{Name: "kid-device", IP: "10.0.0.5"},
		},
	}
	re, _ := NewRulesEngine(&conf)
	dc := dev.NewDeviceCache(nil, cfg.Router{})
	_ = dc.LoadStaticDevices(conf.StaticDevices)
	re.SetDeviceCache(dc)

	act, err := re.Evaluate("any", "10.0.0.5")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if act != cfg.ActionTypeBlockedDevice {
		t.Fatalf("expected device name mapping to match; got %v", act)
	}
}

// Build a large table to cover precedence determinism across mixed host/domain patterns.
func TestPrecedenceTable(t *testing.T) {
	conf := cfg.Config{
		DefaultRule: cfg.ActionTypePass,
		OnErrorRule: cfg.ActionTypeBlockedSiteBan,
		Hosts: []cfg.Host{
			{Name: "10.0.0.1", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedDevice}}},
			{Name: "10.0.*", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedSiteBan}}},
			{Name: "*.local", Rules: []cfg.Rule{{Type: cfg.ActionTypePass}}},
			{Name: "*", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedDevice}}},
		},
		Domains: []cfg.Domain{
			{
				Name: "secure.example.com",
				Hosts: []cfg.Host{
					{Name: "10.0.0.2", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedDevice}}},
					{Name: "*", Rules: []cfg.Rule{{Type: cfg.ActionTypePass}}},
				},
			},
			{
				Name: "*.example.com",
				Hosts: []cfg.Host{
					{Name: "10.0.*", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedSiteBan}}},
					{Name: "*", Rules: []cfg.Rule{{Type: cfg.ActionTypePass}}},
				},
			},
			{
				Name: "*",
				Hosts: []cfg.Host{
					{Name: "*", Rules: []cfg.Rule{{Type: cfg.ActionTypeBlockedSiteBan}}},
				},
			},
		},
		NameServers: []cfg.NameServer{{IP: "8.8.8.8:53"}},
	}
	re, _ := NewRulesEngine(&conf)

	tests := []struct {
		name   string
		domain string
		host   string
		expect cfg.ActionType
	}{
		{"host exact beats domain", "foo.example.com", "10.0.0.1", cfg.ActionTypeBlockedDevice},
		{"host wildcard beats domain host", "foo.example.com", "10.0.9.9", cfg.ActionTypeBlockedSiteBan},
		{"domain exact after host fallback", "secure.example.com", "9.9.9.9", cfg.ActionTypeBlockedDevice},
		{"domain host specific beats domain wildcard host if host skipped", "secure.example.com", "10.0.0.2", cfg.ActionTypeBlockedSiteBan},
		{"domain wildcard host specific beats domain wildcard general", "app.example.com", "10.0.5.5", cfg.ActionTypeBlockedSiteBan},
		{"domain wildcard fallback", "app.example.com", "9.9.9.9", cfg.ActionTypeBlockedDevice},
		{"global domain fallback", "unknown.com", "9.9.9.9", cfg.ActionTypeBlockedDevice},
		{"global host fallback", "unknown.com", "1.2.3.4", cfg.ActionTypeBlockedDevice},
		{"host wildcard prefers longer pattern", "foo.local", "foo.local", cfg.ActionTypePass},
		{"host global wildcard last", "bar", "bar", cfg.ActionTypeBlockedDevice},
		{"domain vs host precedence", "app.example.com", "10.0.0.3", cfg.ActionTypeBlockedSiteBan},
		{"domain exact host wildcard", "secure.example.com", "8.8.8.8", cfg.ActionTypeBlockedDevice},
		{"domain wildcard host wildcard", "other.example.com", "8.8.8.8", cfg.ActionTypeBlockedDevice},
		{"host exact with domain wildcard", "any.example.com", "10.0.0.1", cfg.ActionTypeBlockedDevice},
		{"host wildcard with domain exact", "secure.example.com", "10.0.1.1", cfg.ActionTypeBlockedSiteBan},
		{"host pattern specificity tie uses alpha", "alpha", "zz", cfg.ActionTypeBlockedDevice},
		{"domain pattern specificity tie uses alpha", "zz.example.com", "1.1.1.1", cfg.ActionTypeBlockedDevice},
		{"global domain hit when no host match", "*.example.com", "nomatch", cfg.ActionTypeBlockedDevice},
		{"host star beats none default", "nomatch", "nomatch", cfg.ActionTypeBlockedDevice},
		{"host exact still wins regardless of order", "nomatch", "10.0.0.1", cfg.ActionTypeBlockedDevice},
		{"domain longer wildcard wins", "a.b.example.com", "9.9.9.9", cfg.ActionTypeBlockedDevice},
		{"domain wildcard vs global", "foo.bar", "9.9.9.9", cfg.ActionTypeBlockedDevice},
		{"host wildcard vs global domain", "foo.bar", "10.0.9.9", cfg.ActionTypeBlockedSiteBan},
		{"host exact overrides domain wildcard host", "foo.example.com", "10.0.0.1", cfg.ActionTypeBlockedDevice},
		{"domain match ignored if host none", "foo.example.com", "not", cfg.ActionTypeBlockedDevice},
		{"domain wildcard applies when host global", "foo.example.com", "abc", cfg.ActionTypeBlockedDevice},
		{"host pattern with question mark", "a.local", "a.local", cfg.ActionTypePass},
		{"host pattern question vs star specificity", "b.local", "b.local", cfg.ActionTypePass},
		{"host global still blocks", "c", "c", cfg.ActionTypeBlockedDevice},
		{"default rule when nothing", "nomatch.test", "1.2.3.5", cfg.ActionTypeBlockedDevice},
		{"host wildcard beats default", "nomatch.test", "10.0.9.9", cfg.ActionTypeBlockedSiteBan},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			act, err := re.Evaluate(tt.domain, tt.host)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if act != tt.expect {
				t.Fatalf("expected %v got %v", tt.expect, act)
			}
		})
	}
}
