package rules_test

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	cfg "dnsproxy/internal/config"
	dev "dnsproxy/internal/device"
	"dnsproxy/internal/rules"
	"dnsproxy/internal/utils"
)

var testRules *rules.RulesEngine
var testDeviceCache *dev.DeviceCache

func buildTestConfig() cfg.Config {
	return cfg.Config{
		DefaultRule:   cfg.ActionTypePass,
		OnErrorRule:   cfg.ActionTypeNone,
		Logfile:       "-",
		ListenAddress: ":2053",
		Hosts: []cfg.Host{
			{
				Name: "127.0.0.1",
				Rules: []cfg.Rule{
					{Type: cfg.ActionTypePass},
				},
			},
		},
		Domains: []cfg.Domain{
			{
				Name: "*.rules.test",
				Hosts: []cfg.Host{
					{
						Name: "127.0.0.2",
						Rules: []cfg.Rule{
							{Type: cfg.ActionTypeBlockedTimeSpan, TimeSpan: "17:00-19:00"},
							{Type: cfg.ActionTypeNone},
						},
					},
					{
						Name: "127.0.0.3",
						Rules: []cfg.Rule{
							{Type: cfg.ActionTypeBlockedDevice},
						},
					},
					{
						Name: "*",
						Rules: []cfg.Rule{
							{Type: cfg.ActionTypeBlockedDevice},
						},
					},
				},
			},
			{
				Name: "*",
				Hosts: []cfg.Host{
					{
						Name: "*",
						Rules: []cfg.Rule{
							{Type: cfg.ActionTypeBlockedSiteBan},
						},
					},
				},
			},
		},
		StaticDevices: []cfg.StaticDevice{
			{Name: "nagini", IP: "192.168.1.8"},
		},
		Resolve: []cfg.Host{
			{
				Name: "*.office",
				IpV4: "1.2.3.4",
			},
		},
	}
}

func setup() {
	conf := buildTestConfig()
	testRules, _ = rules.NewRulesEngine(&conf)
	testRules.Debugging(true)

	testDeviceCache = dev.NewDeviceCache(nil, cfg.Router{})
	if err := testDeviceCache.LoadStaticDevices(conf.StaticDevices); err != nil {
		log.Fatalf("failed to load static devices: %v", err)
	}
	testRules.SetDeviceCache(testDeviceCache)
}

func TestCreateRulesEngine(t *testing.T) {
	conf := buildTestConfig()
	_, err := rules.NewRulesEngine(&conf)
	if err != nil {
		t.Error(err)
	}
}

func TestStripPort(t *testing.T) {
	addr := utils.StripPortFromAddr("127.0.0.1:2323")
	if addr != "127.0.0.1" {
		t.Error(errors.New("Address is wrong"))
	}
}

func TestLocalHostPass(t *testing.T) {
	action, err := testRules.Evaluate("*.rules.test", "127.0.0.1")
	if err != nil {
		t.Error(err)
	}
	if action != cfg.ActionTypePass {
		t.Errorf("Wrong action, expected 'ActionTypePass' got '%s'\n", action.String())
	}
}

func TestPassWithTimeSpan(t *testing.T) {
	r := cfg.Rule{
		Type:     cfg.ActionTypePass,
		TimeSpan: "00:00-23:59",
	}

	action, err := rules.EvaluateRule(r)
	if err != nil {
		t.Error(err)
	}
	if action != cfg.ActionTypePass {
		t.Errorf("Wrong action, expected 'ActionTypePAss' got %s\n", action.String())
	}

}

func TestLocalHostNone(t *testing.T) {
	action, err := testRules.Evaluate("site2.rules.test", "127.0.0.2")
	if err != nil {
		t.Error(err)
	}
	log.Printf("Time: %s, action: %s\n", time.Now().Format("15:04"), action.String())

	// Note: Action depends on time

	// if action != ActionTypeNone {
	// 	t.Errorf("Wrong action, expected 'ActionTypeNone' got '%s'\n", action.String())
	// }
}

func TestLocalHostBlock(t *testing.T) {
	action, err := testRules.Evaluate("site3.rules.test", "127.0.0.3")
	if err != nil {
		t.Error(err)
	}
	if action != cfg.ActionTypeBlockedDevice {
		t.Errorf("Wrong action, exptected 'ActionTypeBlockedDevice' got '%s'", action.String())
	}
}

func TestNameRulePass(t *testing.T) {
	action, err := testRules.Evaluate("site1.rules.test", "192.168.1.8")
	if err != nil {
		t.Error(err)
	}

	log.Printf("Time: %s, action: %s\n", time.Now().Format("15:04"), action.String())

	// Note: The action here depends on the time.
	// 16:00 - 20:00 this will give a ban
	// All other times it will give a pass
	//
	// RulesEngine don't give an option to modify input time
	//

	// if action != ActionTypePass {
	// 	t.Errorf("Wrong action, exptected 'ActionTypePass' got '%s'", action.String())
	// }
}

func TestNameRuleLastBlock(t *testing.T) {
	action, err := testRules.Evaluate("www.aftonbladet.se", "192.168.1.23")
	if err != nil {
		t.Error(err)
	}
	if action != cfg.ActionTypeBlockedSiteBan {
		t.Errorf("Wrong action, exptected 'ActionTypeBlockedSiteBan' got '%s'", action.String())
	}
}

func TestNameRuleBlock(t *testing.T) {
	action, err := testRules.Evaluate("*.rules.test", "192.168.1.17")
	if err != nil {
		t.Error(err)
	}
	if action != cfg.ActionTypeBlockedDevice {
		t.Errorf("Wrong action, exptected 'ActionTypeBlockedDevice' got '%s'", action.String())
	}
}

// Note: This requires router initialization
func TestNameToIP(t *testing.T) {
	addr, err := testDeviceCache.NameToIP("nagini")
	if err != nil {
		t.Error(err)
	}

	if addr.String() != "192.168.1.8" {
		t.Error(fmt.Errorf("Expected '192.168.1.8' for 'nagini' - got: %s\n", addr.String()))
	}
}

// Note: This requires router initialization
func TestIPToName(t *testing.T) {
	name, err := testDeviceCache.IPToName("192.168.1.8")
	if err != nil {
		t.Error(err)
	}

	if strings.ToLower(name) != "nagini" {
		t.Error(fmt.Errorf("Expected 'nagini' for 192.168.1.8' - got: %s\n", name))
	}
}

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags
	setup()
	os.Exit(m.Run())
}
