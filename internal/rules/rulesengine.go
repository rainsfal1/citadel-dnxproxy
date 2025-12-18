package rules

/*
	Rules engine for the DNS proxy
*/
import (
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	cfg "dnsproxy/internal/config"
	dev "dnsproxy/internal/device"
	"dnsproxy/internal/utils"
)

// Precedence model (deterministic):
// 1) Host rules are evaluated before domain rules.
// 2) Within a set of host patterns, the most specific pattern wins:
//   - Exact match beats wildcard.
//   - Fewer wildcards beats more wildcards; tie-breaker is longer pattern.
//
// 3) Within a domain, the most specific domain pattern wins using the same scoring.
// 4) Within a selected host's rule list, rules are processed in order; first non-None result wins.
// 5) If nothing matches, DefaultRule is returned.
type RulesEngine struct {
	conf        *cfg.Config
	debug       bool
	deviceCache *dev.DeviceCache
	clock       Clock
}

// Clock allows injecting time for testing.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func NewRulesEngine(conf *cfg.Config) (*RulesEngine, error) {
	re := RulesEngine{
		conf:        conf,
		debug:       false,
		deviceCache: nil,
		clock:       realClock{},
	}
	return &re, nil
}

// SetDeviceCache assigns the cache object for device name to/from ip lookup
func (r *RulesEngine) SetDeviceCache(dc *dev.DeviceCache) {
	r.deviceCache = dc
}

// Debugging set to true to enable debug logging specifically from this component
func (r *RulesEngine) Debugging(enable bool) {
	log.Printf("[INFO] RulesEngine Debugging: %v", enable)
	r.debug = enable
}

// WithClock overrides the clock (for tests).
func (r *RulesEngine) WithClock(clock Clock) {
	r.clock = clock
}

// Evaluate takes a domain and a host and evaluates if any rules apply
//
// For the DNS proxy the "domain" is the question and the host is the originator
func (r *RulesEngine) Evaluate(domain, host string) (cfg.ActionType, error) {
	trace, act, err := r.evaluateInternal(domain, host, false)
	if err != nil {
		return r.conf.OnErrorRule, err
	}
	if trace != nil {
		return act, nil
	}
	return act, nil
}

// Explain returns a detailed decision trace for a given domain/host.
func (r *RulesEngine) Explain(domain, host string) (*Trace, cfg.ActionType, error) {
	return r.evaluateInternal(domain, host, true)
}

type Trace struct {
	DeviceName          string
	HostPattern         string
	HostRuleIndex       int
	DomainPattern       string
	DomainHostPattern   string
	DomainHostRuleIndex int
	TimeSpan            string
	TimeSpanMatched     bool
	FinalAction         cfg.ActionType
	DefaultRuleFallback bool
	Reason              string
}

func (r *RulesEngine) evaluateInternal(domain, host string, explain bool) (*Trace, cfg.ActionType, error) {
	action := r.conf.DefaultRule
	tr := &Trace{FinalAction: action}

	if r.deviceCache != nil {
		if name, err := r.deviceCache.IPToName(host); err == nil {
			tr.DeviceName = name
		}
	}

	if r.debug {
		log.Printf("Evaluate, domain: %s, host: %s", domain, host)
	}

	// 1) Host rules (most specific host pattern)
	if bestHost := r.pickBestHost(host, r.conf.Hosts); bestHost != nil {
		if r.debug {
			log.Printf("Hostmatch, %s -> %s (most specific), evaluating rules...\n", host, bestHost.Name)
		}
		tr.HostPattern = bestHost.Name
		act, idx, res, err := r.EvaluateRules(bestHost.Rules)
		if err != nil {
			if r.debug {
				log.Printf("[WARN] Rules evaluation error: %s\n", err.Error())
			}
			return tr, r.conf.OnErrorRule, err
		}
		if act != cfg.ActionTypeNone {
			tr.HostRuleIndex = idx
			tr.TimeSpan = res.timeSpan
			tr.TimeSpanMatched = res.timeMatched
			tr.FinalAction = act
			tr.DefaultRuleFallback = false
			return result(traceOrNil(explain, tr), act, nil)
		}
	}

	// 2) Domain rules (pick most specific domain pattern, then most specific host within that domain)
	if bestDomain, bestDomainMatch := r.pickBestDomain(domain); bestDomain != nil {
		if r.debug {
			log.Printf("Domainmatch, %s -> %s (most specific), evaluating host rules...\n", domain, bestDomainMatch.pattern)
		}
		tr.DomainPattern = bestDomainMatch.pattern
		if bestHost := r.pickBestHost(host, bestDomain.Hosts); bestHost != nil {
			if r.debug {
				log.Printf("Hostmatch, %s -> %s (within domain), evaluating rules...\n", host, bestHost.Name)
			}
			tr.DomainHostPattern = bestHost.Name
			act, idx, res, err := r.EvaluateRules(bestHost.Rules)
			if err != nil {
				return tr, r.conf.OnErrorRule, err
			}
			tr.DomainHostRuleIndex = idx
			tr.TimeSpan = res.timeSpan
			tr.TimeSpanMatched = res.timeMatched
			tr.FinalAction = act
			tr.DefaultRuleFallback = false
			return result(traceOrNil(explain, tr), act, nil)
		}
	}

	tr.DefaultRuleFallback = true
	return result(traceOrNil(explain, tr), action, nil)
}

func traceOrNil(explain bool, tr *Trace) *Trace {
	if explain {
		return tr
	}
	return nil
}

func result(tr *Trace, act cfg.ActionType, err error) (*Trace, cfg.ActionType, error) {
	return tr, act, err
}

func (r *RulesEngine) DomainMatch(domain, pattern string) bool {
	return utils.WildcardPatternMatch(domain, pattern)
}

func (r *RulesEngine) HostMatch(host, pattern string) bool {

	// host is IP and pattern comes from config
	// check if they are equal - we have a match
	if host == pattern {
		return true
	}

	// Now match against device cache
	if r.deviceCache != nil {
		// Translate host IP to device name
		name, err := r.deviceCache.IPToName(host)
		if err == nil {
			// No error - let's match with it
			return utils.WildcardPatternMatch(name, pattern)
		}
	}
	return utils.WildcardPatternMatch(host, pattern)
}

type patternMatch struct {
	pattern      string
	wildcards    int
	length       int
	exact        bool
	originalHost cfg.Host
}

func (r *RulesEngine) pickBestHost(host string, hosts []cfg.Host) *cfg.Host {
	var matches []patternMatch
	for _, h := range hosts {
		if !r.HostMatch(host, h.Name) {
			continue
		}
		matches = append(matches, patternMatch{
			pattern:      h.Name,
			wildcards:    strings.Count(h.Name, "*") + strings.Count(h.Name, "?"),
			length:       len(h.Name),
			exact:        !strings.ContainsAny(h.Name, "*?") && host == h.Name,
			originalHost: h,
		})
	}
	if len(matches) == 0 {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		// More specific first
		if matches[i].exact != matches[j].exact {
			return matches[i].exact
		}
		if matches[i].wildcards != matches[j].wildcards {
			return matches[i].wildcards < matches[j].wildcards
		}
		if matches[i].length != matches[j].length {
			return matches[i].length > matches[j].length
		}
		return matches[i].pattern < matches[j].pattern
	})
	return &matches[0].originalHost
}

type domainMatch struct {
	pattern string
	cfg     cfg.Domain
	score   matchScore
}

type matchScore struct {
	exact     bool
	wildcards int
	length    int
}

func scorePattern(pattern string, exact bool) matchScore {
	return matchScore{
		exact:     exact,
		wildcards: strings.Count(pattern, "*") + strings.Count(pattern, "?"),
		length:    len(pattern),
	}
}

func compareScore(a, b matchScore) bool {
	if a.exact != b.exact {
		return a.exact
	}
	if a.wildcards != b.wildcards {
		return a.wildcards < b.wildcards
	}
	if a.length != b.length {
		return a.length > b.length
	}
	return false
}

func (r *RulesEngine) pickBestDomain(domain string) (*cfg.Domain, *domainMatch) {
	var matches []domainMatch
	for _, d := range r.conf.Domains {
		if !r.DomainMatch(domain, d.Name) {
			continue
		}
		matches = append(matches, domainMatch{
			pattern: d.Name,
			cfg:     d,
			score:   scorePattern(d.Name, domain == d.Name),
		})
	}
	if len(matches) == 0 {
		return nil, nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if compareScore(matches[i].score, matches[j].score) {
			return true
		}
		if compareScore(matches[j].score, matches[i].score) {
			return false
		}
		return matches[i].pattern < matches[j].pattern
	})
	return &matches[0].cfg, &matches[0]
}

func (re *RulesEngine) EvaluateRules(rules []cfg.Rule) (cfg.ActionType, int, evalResult, error) {
	for idx, r := range rules {
		res, err := evaluateRule(r, timeOption{clock: re.clock})
		if err != nil {
			if re.debug {
				log.Printf("[ERROR] EvaluateRules, failed with error: %s\n", err.Error())
			}
			return re.conf.OnErrorRule, idx, res, err
		}

		if re.debug {
			log.Printf("EvaluateRules: %s -> %s (result)\n", r.Type.String(), res.action.String())
		}

		// None means is used to signal the 'false' from evaluation
		// Any other evaluation means a rule was it - and we break further evaluation
		if res.action != cfg.ActionTypeNone {
			return res.action, idx, res, nil
		}
	}
	return cfg.ActionTypeNone, -1, evalResult{}, nil
}

type evalResult struct {
	action        cfg.ActionType
	timeSpan      string
	timeMatched   bool
	timeEvaluated bool
}

func evaluateRule(r cfg.Rule, opts ...timeOption) (evalResult, error) {

	var clock Clock = realClock{}
	loc := time.Local
	for _, o := range opts {
		if o.clock != nil {
			clock = o.clock
		}
		if o.loc != nil {
			loc = o.loc
		}
	}

	// All rules can have a timespan
	if r.TimeSpan != "" {
		act, matched, err := evaluateTimeSpanBlock(r.TimeSpan, r.Type, clock, loc)
		return evalResult{action: act, timeSpan: r.TimeSpan, timeMatched: matched, timeEvaluated: true}, err
	}

	switch r.Type {
	case cfg.ActionTypeBlockedDevice:
		return evalResult{action: cfg.ActionTypeBlockedDevice}, nil
	case cfg.ActionTypeBlockedSiteBan:
		return evalResult{action: cfg.ActionTypeBlockedSiteBan}, nil
	case cfg.ActionTypeBlockedTimeSpan:
		// Note: This should not happen!!!
		act, matched, err := evaluateTimeSpanBlock(r.TimeSpan, r.Type, clock, loc)
		return evalResult{action: act, timeSpan: r.TimeSpan, timeMatched: matched, timeEvaluated: true}, err
	case cfg.ActionTypePass:
		return evalResult{action: cfg.ActionTypePass}, nil
	case cfg.ActionTypeNone:
		return evalResult{action: cfg.ActionTypeNone}, nil
	}
	log.Printf("[Warninig] Rule::EvaluateRule, invalid rule type: %v\n", r)
	return evalResult{action: cfg.ActionTypeNone}, nil
}

// EvaluateRule is exported for tests and wraps the internal evaluator.
func EvaluateRule(r cfg.Rule) (cfg.ActionType, error) {
	res, err := evaluateRule(r)
	return res.action, err
}

// EvaluateTimeSpanBlock evaluates the current time with respect to definition.
// Supported format:
//
//	HH:MM-HH:MM
//	mon,tue@HH:MM-HH:MM
//	mon-fri@HH:MM-HH:MM|tz=Europe/Stockholm
//
// Cross-midnight windows are supported (e.g., 22:00-06:00).
func evaluateTimeSpanBlock(strTimeSpan string, action cfg.ActionType, clock Clock, loc *time.Location) (cfg.ActionType, bool, error) {
	ts, err := parseTimeSpan(strTimeSpan, loc)
	if err != nil {
		return cfg.ActionTypeNone, false, err
	}

	now := clock.Now().In(ts.loc)
	if len(ts.weekdays) > 0 {
		if !ts.weekdays[now.Weekday()] {
			return cfg.ActionTypeNone, false, nil
		}
	}

	current := time.Date(0, 1, 1, now.Hour(), now.Minute(), 0, 0, ts.loc)
	start := time.Date(0, 1, 1, ts.start.Hour(), ts.start.Minute(), 0, 0, ts.loc)
	end := time.Date(0, 1, 1, ts.end.Hour(), ts.end.Minute(), 0, 0, ts.loc)

	if ts.crossMidnight {
		if current.After(start) || current.Equal(start) {
			return action, true, nil
		}
		if current.Before(end) || current.Equal(end) {
			return action, true, nil
		}
		return cfg.ActionTypeNone, false, nil
	}

	if (current.After(start) || current.Equal(start)) && current.Before(end) {
		return action, true, nil
	}

	return cfg.ActionTypeNone, false, nil
}

type parsedTimeSpan struct {
	start         time.Time
	end           time.Time
	crossMidnight bool
	weekdays      map[time.Weekday]bool
	loc           *time.Location
}

type timeOption struct {
	clock Clock
	loc   *time.Location
}

func parseTimeSpan(def string, defaultLoc *time.Location) (*parsedTimeSpan, error) {
	if defaultLoc == nil {
		defaultLoc = time.Local
	}
	parts := strings.Split(def, "|")
	timePart := strings.TrimSpace(parts[0])
	if timePart == "" {
		return nil, errors.New("Timespan definition error, use: \"HH:mm-HH:mm\"")
	}
	weekdayPart := ""
	if strings.Contains(timePart, "@") {
		s := strings.SplitN(timePart, "@", 2)
		weekdayPart = s[0]
		timePart = s[1]
	}
	timeRange := strings.Split(timePart, "-")
	if len(timeRange) != 2 {
		return nil, errors.New("Timespan definition error, use: \"HH:mm-HH:mm\"")
	}
	start, err := time.Parse("15:04", timeRange[0])
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("15:04", timeRange[1])
	if err != nil {
		return nil, err
	}
	loc := defaultLoc
	if len(parts) > 1 {
		for _, opt := range parts[1:] {
			opt = strings.TrimSpace(opt)
			if opt == "" {
				continue
			}
			if strings.HasPrefix(opt, "tz=") {
				locName := strings.TrimPrefix(opt, "tz=")
				l, err := time.LoadLocation(locName)
				if err != nil {
					return nil, err
				}
				loc = l
			}
		}
	}
	weekdaySet, err := parseWeekdays(weekdayPart)
	if err != nil {
		return nil, err
	}
	return &parsedTimeSpan{
		start:         start,
		end:           end,
		crossMidnight: start.After(end),
		weekdays:      weekdaySet,
		loc:           loc,
	}, nil
}

func parseWeekdays(def string) (map[time.Weekday]bool, error) {
	result := make(map[time.Weekday]bool)
	def = strings.TrimSpace(def)
	if def == "" {
		return result, nil
	}
	parts := strings.Split(def, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "-") {
			r := strings.SplitN(p, "-", 2)
			if len(r) != 2 {
				return nil, errors.New("invalid weekday range")
			}
			start, err := parseWeekday(r[0])
			if err != nil {
				return nil, err
			}
			end, err := parseWeekday(r[1])
			if err != nil {
				return nil, err
			}
			for i := start; ; i = (i + 1) % 7 {
				result[i] = true
				if i == end {
					break
				}
			}
		} else {
			w, err := parseWeekday(p)
			if err != nil {
				return nil, err
			}
			result[w] = true
		}
	}
	return result, nil
}

func parseWeekday(s string) (time.Weekday, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sun", "sunday":
		return time.Sunday, nil
	case "mon", "monday":
		return time.Monday, nil
	case "tue", "tuesday":
		return time.Tuesday, nil
	case "wed", "wednesday":
		return time.Wednesday, nil
	case "thu", "thursday":
		return time.Thursday, nil
	case "fri", "friday":
		return time.Friday, nil
	case "sat", "saturday":
		return time.Saturday, nil
	default:
		return time.Sunday, errors.New("invalid weekday")
	}
}
