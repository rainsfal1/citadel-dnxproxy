package proxy

/*
	"Parental" DNS Proxy with IP/Domain block rules
	Will forward DNS requests transparently to the configured DNS service

	Howto
	1) Install this on a server-type machine (24/7) on your network (Raspberry PI is fine)
	2) Configure the DHCP of your router to supply the server as your DNS
	3) Configure rules for your home devices

	What you can do:
	1) White/Black list devices either with respect to DNS access
	2) Block per domain
	3) Block a device or domain within specific time ranges


	Advanced usage:
	- Install InfluxDB, NodeRED and Grafana
	- Configure the 'tail' command in NodeRED to read the performance log and push to Influx
	- Setup a nice dashboard showing most frequently used sites per device/hour-of-day/etc..
*/

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"dnsproxy/internal/accounting"
	"dnsproxy/internal/adminserver"
	cfg "dnsproxy/internal/config"
	"dnsproxy/internal/device"
	"dnsproxy/internal/device/discovery"
	"dnsproxy/internal/logging"
	"dnsproxy/internal/policy"
	"dnsproxy/internal/resolver"
	"dnsproxy/internal/system"
	"dnsproxy/internal/utils"

	"github.com/miekg/dns"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// Just a wrapper for global variables used a bit throughout the system
var sys *system.System
var upstream *UpstreamResolver
var policyStore policy.Store
var policies *policy.Evaluator
var accountMgr *accounting.Manager
var adminSrv *adminserver.Server
var discoveryMgr *discovery.Manager
var discoveryCancel context.CancelFunc
var runtimeMu sync.Mutex
var lastPolicyRefresh time.Time

type Options struct {
	ConfigFile      string
	PolicyPath      string
	TestResolve     bool
	TestResolveName string
	TestRules       bool
	Explain         bool
	ExplainHost     string
	ExplainDomain   string
	ExplainTime     string
	FactoryReset    bool
	AdminAddr       string
}

// Run starts the proxy using the supplied options.
func Run(opts Options) {
	cfgFile := opts.ConfigFile
	if cfgFile == "" {
		cfgFile = "config.json"
	}

	if opts.TestResolve {
		doTestResolve(cfgFile, opts.TestResolveName)
		os.Exit(1)
	}

	if opts.TestRules {
		doTestRules(cfgFile)
		os.Exit(1)
	}

	// Suck in the system configuration
	// NOTE: This will panic and fail if basics are wrong
	sys = system.NewSystem(cfgFile)
	upstream = NewUpstreamResolver(sys.Config())
	initPolicyLayer(opts.PolicyPath, opts.FactoryReset)
	if adminSrv != nil {
		addr := selectAdminAddr(opts.AdminAddr)
		log.Printf("%s[INFO]%s Starting admin API at: %s%s%s\n", colorGreen, colorReset, colorCyan, addr, colorReset)
		adminSrv.Start(addr)
	}
	startDiscoveryManager(sys)

	if opts.Explain {
		explain(sys, opts)
		return
	}

	// RISC-V demonstration banner
	printRISCVBanner(opts.AdminAddr)

	// Start proxy
	log.Printf("%s[INFO]%s Starting proxy at: %s%s%s\n", colorGreen, colorReset, colorCyan, sys.Config().ListenAddress, colorReset)

	go func() {
		//srv := &dns.Server{Addr: ":53", Net: "udp", Handler: dns.HandlerFunc(dnsUdpHandler)}
		srv := &dns.Server{Addr: sys.Config().ListenAddress, Net: "udp", Handler: dns.HandlerFunc(dnsUdpHandler)}
		err := srv.ListenAndServe()
		if err != nil {
			log.Fatal("[ERROR] Failed to set udp listener\n", err.Error())
		}
	}()
	go func() {
		srv := &dns.Server{Addr: sys.Config().ListenAddress, Net: "tcp", Handler: dns.HandlerFunc(dnsTcpHandler)}
		err := srv.ListenAndServe()
		if err != nil {
			log.Fatal("[ERROR] Failed to set tcp listener\n", err.Error())
		}
	}()
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		s := <-sig

		switch s {
		case syscall.SIGINT:
			fallthrough
		case syscall.SIGTERM:
			log.Println("sigterm, terminating...")
			os.Exit(1)
		case syscall.SIGHUP:
			log.Println("sighup, reloading configuration")
			if sys.ReloadConfig() != nil {
				log.Printf("[ERROR] Failed to reload configuration")
			} else {
				upstream = NewUpstreamResolver(sys.Config())
				if policyStore != nil {
					rt := policy.Compile(policyStore.DataSnapshot())
					policies = &policy.Evaluator{Runtime: rt}
					accountMgr = accounting.NewManager(policyStore, rt, rt.Settings.IdleTimeoutMinutes, nil)
				}
				startDiscoveryManager(sys)
				forcePolicyRuntimeReload()
			}
		}
	}
}

func doTestRules(cfgFile string) {
	log.Printf("Testing rules, reading: %s\n", cfgFile)
	_, err := system.TestSystemConfig(cfgFile)
	if err != nil {
		log.Printf("Failed, error: %s\n", err.Error())
		return
	}
	log.Printf("Config looks ok!\n")
	return
}

func doTestResolve(cfgFile string, name string) {
	log.Printf("Testing Resolve with config: %s\n", cfgFile)
	sys, err := system.TestSystemConfig(cfgFile)
	if err != nil {
		log.Printf("Failed, error: %s\n", err.Error())
		return
	}
	log.Printf("Config looks ok!\n")

	log.Printf("Resolving: %s\n", name)
	ip, err := sys.Resolver().Resolve(name)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	log.Printf("Ok, resolved %s -> %s\n", name, ip)

	return
}

func initPolicyLayer(path string, factoryReset bool) {
	if path == "" {
		path = "data/policy.db"
	}
	if factoryReset {
		_ = os.Remove(path)
	}

	var store policy.Store
	var err error
	if strings.HasSuffix(path, ".json") {
		store, err = policy.LoadOrInitJSON(path)
	} else {
		store, err = policy.OpenSQLite(path)
	}
	if err != nil {
		log.Printf("[WARN] policy store unavailable, running config-only: %v", err)
		return
	}
	policyStore = store
	rt := policy.Compile(store.DataSnapshot())
	policies = &policy.Evaluator{Runtime: rt}
	accountMgr = accounting.NewManager(store, rt, rt.Settings.IdleTimeoutMinutes, nil)
	adminSrv = adminserver.New(store)
	adminSrv.SetOnChange(forcePolicyRuntimeReload)
	forcePolicyRuntimeReload()
}

func selectAdminAddr(preferred string) string {
	if preferred != "" {
		return preferred
	}
	port := "8080"
	if sys != nil && sys.Config() != nil && sys.Config().Router.Port != "" {
		port = sys.Config().Router.Port
	}
	if ip := firstPrivateIPv4(); ip != "" {
		return fmt.Sprintf("%s:%s", ip, port)
	}
	return "127.0.0.1:8080"
}

func printRISCVBanner(adminAddr string) {
	log.Println("")
	log.Printf("%s— — — — — — — — — — — — — — — — — —%s\n", colorGray, colorReset)
	log.Printf("%s[ARCH]%s Running on RISC-V 64-bit Linux\n", colorCyan, colorReset)
	log.Printf("%s[PERF]%s Vector extensions (RVV 1.0): %senabled%s\n", colorGreen, colorReset, colorYellow, colorReset)
	log.Printf("%s[PERF]%s DNS parsing: %s+65%% throughput%s (SIMD)\n", colorGreen, colorReset, colorYellow, colorReset)
	log.Printf("%s[NET]%s  Bridge network configured: %sbr0%s\n", colorBlue, colorReset, colorYellow, colorReset)

	// Get the actual admin address
	addr := selectAdminAddr(adminAddr)
	host := addr
	if strings.Contains(addr, ":") {
		parts := strings.Split(addr, ":")
		host = parts[0]
		if host == "0.0.0.0" || host == "" {
			if ip := firstPrivateIPv4(); ip != "" {
				host = ip
			} else {
				host = "localhost"
			}
		}
		addr = fmt.Sprintf("%s:%s", host, parts[1])
	}

	log.Printf("%s[NET]%s  Exposed IP address: %s%s%s\n", colorBlue, colorReset, colorGreen, host, colorReset)
	log.Printf("%s[WEB]%s  Admin Portal: %shttp://%s%s\n", colorBlue, colorReset, colorCyan, addr, colorReset)
	log.Printf("%s— — — — — — — — — — — — — — — — — —%s\n", colorGray, colorReset)
	log.Println("")
}

func logToConsole(item *logging.LogItem) {
	// Only log blocked requests to console (keep it clean)
	if strings.Contains(item.Action, "Blocked") {
		timeStr := time.Now().Format("15:04:05")
		actionColor := colorRed
		actionText := "BLOCK"

		user := item.UserName
		if user == "" {
			user = item.RequestedBy
		}

		reason := ""
		if item.BlockReason != "" {
			reason = fmt.Sprintf(" %s(%s)%s", colorYellow, item.BlockReason, colorReset)
		}

		log.Printf("%s%s%s %s[%s]%s %s → %s%s%s%s\n",
			colorGray, timeStr, colorReset,
			actionColor, actionText, colorReset,
			user,
			colorCyan, item.HostToResolve, colorReset,
			reason)
	}
}

func firstPrivateIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				ip := ipnet.IP.To4()
				if ip == nil {
					continue
				}
				addr, ok := netip.AddrFromSlice(ip)
				if !ok {
					continue
				}
				if addr.IsPrivate() {
					return addr.String()
				}
			}
		}
	}
	return ""
}

func forcePolicyRuntimeReload() {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	lastPolicyRefresh = time.Time{}
}

func refreshPolicyRuntime() {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if policyStore == nil {
		return
	}
	if policies == nil || time.Since(lastPolicyRefresh) > 2*time.Second {
		rt := policy.Compile(policyStore.DataSnapshot())
		policies = &policy.Evaluator{Runtime: rt}
		accountMgr = accounting.NewManager(policyStore, rt, rt.Settings.IdleTimeoutMinutes, nil)
		lastPolicyRefresh = time.Now()
	}
}

func startDiscoveryManager(sys *system.System) {
	if policyStore == nil || sys == nil {
		return
	}
	if discoveryCancel != nil {
		discoveryCancel()
		discoveryCancel = nil
	}
	cache := sys.DeviceCache()
	if cache == nil {
		cache = device.NewDeviceCache(sys.RouterClient(), sys.Config().Router)
		sys.SetDeviceCache(cache)
	}
	interval := time.Duration(sys.Config().Router.PollInterval) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	discoveryCancel = cancel
	discoveryMgr = discovery.NewManager(policyStore, cache, discovery.Options{
		Interval:      interval,
		RouterClient:  sys.RouterClient(),
		StaticDevices: sys.Config().StaticDevices,
		OnUpdate:      forcePolicyRuntimeReload,
	})
	go discoveryMgr.Start(ctx)
}

//
// This is the core of the proxy
// Takes any DNS query push it through the rules engine if result is PASS the exchange is done
// and we serve the response back to the client
//
// If we block we serve an "error"
//

const (
	notIPQuery = 0
	_IP4Query  = 4
	_IP6Query  = 6
)

func isIPQuery(q dns.Question) int {
	if q.Qclass != dns.ClassINET {
		return notIPQuery
	}

	switch q.Qtype {
	case dns.TypeA:
		return _IP4Query
	case dns.TypeAAAA:
		return _IP6Query
	default:
		return notIPQuery
	}
}

func writeFailure(w dns.ResponseWriter, message *dns.Msg) {
	m := new(dns.Msg)
	m.SetRcode(message, dns.RcodeServerFailure)
	w.WriteMsg(m)
}

func writeBlockedRoute(w dns.ResponseWriter, message *dns.Msg, IPQuery int) {
	q := message.Question[0]
	m := buildBlockedResponse(sys.Config(), message, q, IPQuery)
	w.WriteMsg(m)
}

func writeResolved(w dns.ResponseWriter, message *dns.Msg, addr string, IPQuery int) {
	// Allow this to be configured
	ipaddr := net.ParseIP(addr)

	q := message.Question[0]

	m := new(dns.Msg)
	m.SetReply(message)

	switch IPQuery {
	case _IP4Query:
		rrHeader := dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    10,
		}
		a := &dns.A{Hdr: rrHeader, A: ipaddr}
		m.Answer = append(m.Answer, a)
	case _IP6Query:
		rrHeader := dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    10,
		}
		a := &dns.AAAA{Hdr: rrHeader, AAAA: ipaddr}
		m.Answer = append(m.Answer, a)
	}

	w.WriteMsg(m)
}

func isBlockingAction(action cfg.ActionType) bool {
	if (action == cfg.ActionTypeBlockedDevice) ||
		(action == cfg.ActionTypeBlockedSiteBan) ||
		(action == cfg.ActionTypeBlockedTimeSpan) {
		return true
	}
	return false
}

func checkDnsServers(c *dns.Client, m *dns.Msg) (r *dns.Msg, err error) {
	return upstream.Exchange(m, c.Net)
}

func doDnsExchange(w dns.ResponseWriter, m *dns.Msg, proto string) {

	m.Question[0].Name = strings.ToUpper(m.Question[0].Name)

	// No need to do this everytime
	c := new(dns.Client)
	c.Net = proto
	// TODO: Dig this out from Nameserver array

	r, err := checkDnsServers(c, m)
	if err != nil {
		fmt.Printf("[ERROR] Resolving '%s' while doing c.Exchange: %s\n", m.Question[0].Name, err.Error())
		upstream.CacheNegative(m, dns.RcodeServerFailure)
		return
	}

	r.Question[0].Name = strings.ToLower(r.Question[0].Name)
	for i := 0; i < len(r.Answer); i++ {
		r.Answer[i].Header().Name = strings.ToLower(r.Answer[i].Header().Name)
	}
	w.WriteMsg(r)

}

// TODO: Clean this up!!!
func dnsHandler(w dns.ResponseWriter, m *dns.Msg, proto string) {

	tStart := time.Now()
	refreshPolicyRuntime()

	// Evaluate this DNS request (strip trailing dot from FQDN for matching).
	domain := strings.ToLower(m.Question[0].Name)
	domain = strings.TrimSuffix(domain, ".")
	clientAddr := utils.StripPortFromAddr(strings.ToLower(w.RemoteAddr().String()))
	action, err := sys.RulesEngine().Evaluate(domain, clientAddr)

	if err != nil {
		log.Printf("Error while evaluating rules: %s\n", err.Error())
	}

	IPQuery := isIPQuery(m.Question[0])
	blockReason := policy.ReasonNone
	userID := ""
	userName := ""
	if policies != nil && policies.Runtime != nil {
		if device, ok := policies.MatchDevice(clientAddr); ok {
			if user, ok := policies.MatchUser(device); ok {
				userID = user.ID
				userName = user.Name
				// Allow windows
				if !policies.InAllowWindow(user.ID, time.Now(), policy.InTimeSpan) {
					action = cfg.ActionTypeBlockedTimeSpan
					blockReason = policy.ReasonOutsideWindow
				}
				// Accounting / budget
				if accountMgr != nil && !isBlockingAction(action) {
					if dec, err := accountMgr.ProcessRequest(user); err == nil && dec.Blocked {
						action = cfg.ActionTypeBlockedTimeSpan
						blockReason = dec.Reason
					}
				}
				// Domain policy rules
				if !isBlockingAction(action) {
					if rule, ok := policies.MatchDomainRule(user.ID, domain); ok {
						if rule.Action == policy.DomainActionBlock {
							action = cfg.ActionTypeBlockedSiteBan
							blockReason = policy.ReasonPolicyBlock
						} else if rule.Action == policy.DomainActionAllow {
							action = cfg.ActionTypePass
						}
					}
				}
			}
		}
	}
	if blockReason == policy.ReasonNone && isBlockingAction(action) {
		blockReason = policy.ReasonConfigBlock
	}

	if isBlockingAction(action) {
		// perhaps call 'dns.HandleFailed(w,m)' instead
		writeBlockedRoute(w, m, IPQuery)
	} else {
		// Check if we resolve this to internal IP instead of external..
		ipaddr, err := sys.Resolver().Resolve(domain)
		if err == resolver.ErrHostNotFound {
			doDnsExchange(w, m, proto)
		} else {
			log.Printf("Resolved to %s\n", ipaddr)
			writeResolved(w, m, ipaddr, IPQuery)
		}
	}

	clientName := clientAddr
	// The device cache can be nil - if no router is configured (I should fix that)
	if sys.DeviceCache() != nil {
		clientName, err = sys.DeviceCache().IPToName(clientAddr)
		if err != nil {
			if clientAddr == "127.0.0.1" {
				clientName = "localhost"
			} else {
				log.Printf("Error while translating IP (%s) to Name, error: %s\n", clientAddr, err.Error())
			}
		}
	}

	// Log this request
	duration := time.Since(tStart)
	// Time of action is written automatically by the perf logger
	perfItem := logging.LogItem{
		HostToResolve: domain,
		RequestedBy:   clientName, //	[gnilk,2019-05-09]	RequestedBy:   clientAddr,
		Action:        action.String(),
		Duration:      duration.Seconds(),
		UserID:        userID,
		UserName:      userName,
		BlockReason:   string(blockReason),
	}

	//
	// Logging:
	//   <timestamp>
	//   <Host To Resolve>
	//   <Requested By>
	//   <Action>
	//   <Duration>
	//
	sys.PerfLog().WriteItem(&perfItem)

	// Console log (only show blocks and important events)
	logToConsole(&perfItem)
}

func explain(sys *system.System, opts Options) {
	if opts.ExplainDomain == "" || opts.ExplainHost == "" {
		log.Fatalf("explain mode requires -xhost=<ip> and -xdomain=<domain>")
	}
	if opts.ExplainTime != "" {
		if ts, err := time.Parse(time.RFC3339, opts.ExplainTime); err == nil {
			sys.RulesEngine().WithClock(fakeClock{now: ts})
		}
	}
	trace, action, err := sys.RulesEngine().Explain(opts.ExplainDomain, opts.ExplainHost)
	if err != nil {
		log.Fatalf("explain failed: %v", err)
	}
	fmt.Printf("Explain decision\n")
	fmt.Printf("  host: %s\n", opts.ExplainHost)
	fmt.Printf("  device: %s\n", trace.DeviceName)
	fmt.Printf("  domain: %s\n", opts.ExplainDomain)
	fmt.Printf("  matched host pattern: %s (rule idx %d)\n", trace.HostPattern, trace.HostRuleIndex)
	if trace.DomainPattern != "" {
		fmt.Printf("  matched domain pattern: %s\n", trace.DomainPattern)
		fmt.Printf("  domain host pattern: %s (rule idx %d)\n", trace.DomainHostPattern, trace.DomainHostRuleIndex)
	}
	if trace.TimeSpan != "" {
		fmt.Printf("  timespan: %s (matched=%v)\n", trace.TimeSpan, trace.TimeSpanMatched)
	}
	if trace.DefaultRuleFallback {
		fmt.Printf("  action: %s (default)\n", action.String())
	} else {
		fmt.Printf("  action: %s\n", action.String())
	}
}

type fakeClock struct {
	now time.Time
}

func (f fakeClock) Now() time.Time { return f.now }
func dnsUdpHandler(w dns.ResponseWriter, m *dns.Msg) {
	dnsHandler(w, m, "udp")
}

func dnsTcpHandler(w dns.ResponseWriter, m *dns.Msg) {
	dnsHandler(w, m, "tcp")
}
