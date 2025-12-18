package proxy

import (
	"net"
	"strings"

	cfg "dnsproxy/internal/config"

	"github.com/miekg/dns"
)

func buildBlockedResponse(conf *cfg.Config, message *dns.Msg, q dns.Question, IPQuery int) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(message)

	strategy := strings.ToLower(strings.TrimSpace(conf.BlockStrategy.Action))
	if strategy == "" {
		strategy = "nullroute"
	}

	// Rcodes for non-address strategies
	switch strategy {
	case "nxdomain":
		m.SetRcode(message, dns.RcodeNameError)
		return m
	case "refuse":
		m.SetRcode(message, dns.RcodeRefused)
		return m
	}

	// address-based strategies
	ipv4 := conf.BlockStrategy.SinkholeIPv4
	if ipv4 == "" {
		ipv4 = conf.IPv4BlockResolve
	}
	if ipv4 == "" {
		ipv4 = "0.0.0.0"
	}
	ipv6 := conf.BlockStrategy.SinkholeIPv6
	if ipv6 == "" {
		ipv6 = conf.IPv6BlockResolve
	}
	if ipv6 == "" {
		ipv6 = "::"
	}

	switch IPQuery {
	case _IP4Query:
		rrHeader := dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    10,
		}
		a := &dns.A{Hdr: rrHeader, A: net.ParseIP(ipv4)}
		m.Answer = append(m.Answer, a)
	case _IP6Query:
		rrHeader := dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    10,
		}
		a := &dns.AAAA{Hdr: rrHeader, AAAA: net.ParseIP(ipv6)}
		m.Answer = append(m.Answer, a)
	default:
		// For non-A/AAAA, respond NXDOMAIN to indicate blocked
		m.SetRcode(message, dns.RcodeNameError)
	}
	return m
}
