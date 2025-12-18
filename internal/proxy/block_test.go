package proxy

import (
	"testing"

	cfg "dnsproxy/internal/config"

	"github.com/miekg/dns"
)

func TestBlockStrategies(t *testing.T) {
	msg := new(dns.Msg)
	q := dns.Question{Name: "blocked.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}
	msg.Question = []dns.Question{q}

	base := cfg.Config{
		IPv4BlockResolve: "0.0.0.0",
		IPv6BlockResolve: "::",
	}

	tests := []struct {
		name      string
		strategy  cfg.BlockStrategy
		qtype     int
		expectR   int
		expectAns int
	}{
		{"nxdomain", cfg.BlockStrategy{Action: "nxdomain"}, _IP4Query, dns.RcodeNameError, 0},
		{"refuse", cfg.BlockStrategy{Action: "refuse"}, _IP4Query, dns.RcodeRefused, 0},
		{"nullroute A", cfg.BlockStrategy{Action: "nullroute"}, _IP4Query, dns.RcodeSuccess, 1},
		{"nullroute AAAA", cfg.BlockStrategy{Action: "nullroute"}, _IP6Query, dns.RcodeSuccess, 1},
		{"sinkhole A", cfg.BlockStrategy{Action: "sinkhole", SinkholeIPv4: "1.2.3.4"}, _IP4Query, dns.RcodeSuccess, 1},
		{"non-ip nxdomain", cfg.BlockStrategy{Action: "nxdomain"}, notIPQuery, dns.RcodeNameError, 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			conf := base
			conf.BlockStrategy = tt.strategy
			res := buildBlockedResponse(&conf, msg, q, tt.qtype)
			if res.Rcode != tt.expectR {
				t.Fatalf("rcode expected %d got %d", tt.expectR, res.Rcode)
			}
			if len(res.Answer) != tt.expectAns {
				t.Fatalf("answers expected %d got %d", tt.expectAns, len(res.Answer))
			}
		})
	}
}
