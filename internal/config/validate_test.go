package config

import (
	"strings"
	"testing"
)

func TestValidateConfig_InvalidCases(t *testing.T) {
	type testcase struct {
		name   string
		config Config
		expect string
	}

	base := Config{
		Logfile:       "-",
		ListenAddress: ":2053",
		DefaultRule:   ActionTypePass,
		OnErrorRule:   ActionTypeBlockedSiteBan,
		NameServers:   []NameServer{{IP: "8.8.8.8:53"}},
	}

	makeRules := func(rules ...Rule) []Rule { return rules }

	cases := []testcase{
		{
			name:   "missing default rule",
			config: func() Config { c := base; c.DefaultRule = ActionTypeNone; return c }(),
			expect: "DefaultRule",
		},
		{
			name:   "missing on error rule",
			config: func() Config { c := base; c.OnErrorRule = ActionTypeNone; return c }(),
			expect: "OnErrorRule",
		},
		{
			name:   "empty logfile",
			config: func() Config { c := base; c.Logfile = ""; return c }(),
			expect: "Logfile",
		},
		{
			name:   "empty listen address",
			config: func() Config { c := base; c.ListenAddress = ""; return c }(),
			expect: "ListenAddress",
		},
		{
			name:   "no nameservers",
			config: func() Config { c := base; c.NameServers = nil; return c }(),
			expect: "NameServers",
		},
		{
			name:   "nameserver missing ip",
			config: func() Config { c := base; c.NameServers = []NameServer{{IP: ""}}; return c }(),
			expect: "NameServers[0].IP",
		},
		{
			name:   "nameserver bad hostport",
			config: func() Config { c := base; c.NameServers = []NameServer{{IP: "8.8.8.8"}}; return c }(),
			expect: "NameServers[0].IP",
		},
		{
			name: "host missing name",
			config: func() Config {
				c := base
				c.Hosts = []Host{{Rules: makeRules(Rule{Type: ActionTypePass})}}
				return c
			}(),
			expect: "Hosts[0].Name",
		},
		{
			name: "host rule invalid timespan format",
			config: func() Config {
				c := base
				c.Hosts = []Host{{Name: "x", Rules: makeRules(Rule{Type: ActionTypePass, TimeSpan: "bad"})}}
				return c
			}(),
			expect: "Hosts[0].Rules[0].TimeSpan",
		},
		{
			name: "host rule invalid timespan start after end",
			config: func() Config {
				c := base
				c.Hosts = []Host{{Name: "x", Rules: makeRules(Rule{Type: ActionTypePass, TimeSpan: "10:00-10:00"})}}
				return c
			}(),
			expect: "Hosts[0].Rules[0].TimeSpan",
		},
		{
			name: "host rule invalid hour",
			config: func() Config {
				c := base
				c.Hosts = []Host{{Name: "x", Rules: makeRules(Rule{Type: ActionTypePass, TimeSpan: "24:00-25:00"})}}
				return c
			}(),
			expect: "Hosts[0].Rules[0].TimeSpan",
		},
		{
			name: "host rule invalid timezone opt",
			config: func() Config {
				c := base
				c.Hosts = []Host{{Name: "x", Rules: makeRules(Rule{Type: ActionTypePass, TimeSpan: "10:00-11:00|tz=Bad/Zone"})}}
				return c
			}(),
			expect: "Hosts[0].Rules[0].TimeSpan",
		},
		{
			name: "domain missing name",
			config: func() Config {
				c := base
				c.Domains = []Domain{{Hosts: []Host{{Name: "*", Rules: makeRules(Rule{Type: ActionTypePass})}}}}
				return c
			}(),
			expect: "Domains[0].Name",
		},
		{
			name: "domain host missing name",
			config: func() Config {
				c := base
				c.Domains = []Domain{{Name: "*.x", Hosts: []Host{{Rules: makeRules(Rule{Type: ActionTypePass})}}}}
				return c
			}(),
			expect: "Domains[0].Hosts[0].Name",
		},
		{
			name: "resolve missing name",
			config: func() Config {
				c := base
				c.Resolve = []Host{{IpV4: "1.1.1.1"}}
				return c
			}(),
			expect: "Resolve[0].Name",
		},
		{
			name: "resolve missing ip",
			config: func() Config {
				c := base
				c.Resolve = []Host{{Name: "x"}}
				return c
			}(),
			expect: "Resolve[0].IpV4",
		},
		{
			name: "resolve invalid ip",
			config: func() Config {
				c := base
				c.Resolve = []Host{{Name: "x", IpV4: "not-an-ip"}}
				return c
			}(),
			expect: "Resolve[0].IpV4",
		},
		{
			name: "static device missing name",
			config: func() Config {
				c := base
				c.StaticDevices = []StaticDevice{{IP: "1.2.3.4"}}
				return c
			}(),
			expect: "StaticDevices[0].Name",
		},
		{
			name: "static device missing ip",
			config: func() Config {
				c := base
				c.StaticDevices = []StaticDevice{{Name: "dev"}}
				return c
			}(),
			expect: "StaticDevices[0].IP",
		},
		{
			name: "static device invalid ip",
			config: func() Config {
				c := base
				c.StaticDevices = []StaticDevice{{Name: "dev", IP: "bad"}}
				return c
			}(),
			expect: "StaticDevices[0].IP",
		},
		{
			name: "static device invalid mac",
			config: func() Config {
				c := base
				c.StaticDevices = []StaticDevice{{Name: "dev", IP: "1.2.3.4", MAC: "xx-yy"}}
				return c
			}(),
			expect: "StaticDevices[0].MAC",
		},
		{
			name: "router enabled missing host",
			config: func() Config {
				c := base
				c.Router = Router{Engine: RouterTypeUnifi, Port: "8443", User: "u", Password: "p"}
				return c
			}(),
			expect: "Router.Host",
		},
		{
			name: "router poll interval too low",
			config: func() Config {
				c := base
				c.Router = Router{Engine: RouterTypeUnifi, Host: "h", Port: "p", User: "u", Password: "p", PollChanges: true, PollInterval: 5}
				return c
			}(),
			expect: "Router.PollInterval",
		},
		{
			name: "invalid router engine",
			config: func() Config {
				c := base
				c.Router = Router{Engine: RouterType(99)}
				return c
			}(),
			expect: "Router.Engine",
		},
		{
			name: "invalid rule action",
			config: func() Config {
				c := base
				c.Hosts = []Host{{Name: "x", Rules: makeRules(Rule{Type: ActionType(99)})}}
				return c
			}(),
			expect: "Hosts[0].Rules[0].Type",
		},
		{
			name: "timespan with unknown option",
			config: func() Config {
				c := base
				c.Hosts = []Host{{Name: "x", Rules: makeRules(Rule{Type: ActionTypePass, TimeSpan: "10:00-11:00|foo=bar"})}}
				return c
			}(),
			expect: "Hosts[0].Rules[0].TimeSpan",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.expect)
			}
			if !strings.Contains(err.Error(), tc.expect) {
				t.Fatalf("expected error to mention %q, got %q", tc.expect, err.Error())
			}
		})
	}
}
