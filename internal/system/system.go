package system

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"

	cfg "dnsproxy/internal/config"
	dev "dnsproxy/internal/device"
	"dnsproxy/internal/logging"
	"dnsproxy/internal/resolver"
	"dnsproxy/internal/rules"
)

type System struct {
	performanceLog logging.LogClient
	config         *cfg.Config
	rulesEngine    *rules.RulesEngine
	resolver       *resolver.Resolver
	routerClient   dev.RouterClient
	deviceCache    *dev.DeviceCache
	mutex          sync.Mutex
}

// NewSystem creates a new system object and initializes the sub systems
func NewSystem(cfgFileName string) *System {

	sys := System{}
	conf, err := sys.loadConfig(cfgFileName)
	if err != nil {
		log.Panic(err)
	}
	sys.config = conf
	err = sys.validateConfig(conf)
	if err != nil {
		log.Panic(err)
	}

	re, err := rules.NewRulesEngine(sys.config)
	if err != nil {
		log.Println("[ERROR] failed create rules engine: ", err.Error())
		os.Exit(1)
	}

	logger, err := logging.NewLogFileClient(sys.config.Logfile)
	if err != nil {
		log.Panic(err.Error())
	}
	sys.performanceLog = logger

	// Setup router and device cache - this will download local lan device names from the router
	// Add's support for 'names' instead of IP for the proxy host rules
	if sys.config.Router.Engine != cfg.RouterTypeNone {
		log.Printf("[INFO] Router configuration found - trying...")
		err = sys.initializeRouter(sys.config.Router)
		if err != nil {
			log.Printf("[ERROR] Router initialization failed: %s\n", err.Error())
			log.Printf("[WARN] Device Name lookup disabled - requires working router connection\n")
		} else {
			dc := dev.NewDeviceCache(sys.routerClient, sys.config.Router)
			err = dc.Initialize()
			if err != nil {
				log.Printf("[ERROR] Device Cache initialization failed: %s\n", err.Error())
			} else {
				log.Printf("[INFO] Ok, device list downloaded")
				sys.deviceCache = dc
				dc.Dump()
				if sys.Config().Router.PollChanges {
					log.Printf("[INFO] Starting router auto refresh, interval: %d sec", sys.Config().Router.PollInterval)
					dc.StartAutoRefresh(sys.Config().Router.PollInterval)
				}
			}
		}
	}

	// Load static devices if configured (for testing/demo without router hardware)
	if len(sys.config.StaticDevices) > 0 {
		if sys.deviceCache == nil {
			// No router - create empty device cache for static devices
			log.Printf("[INFO] No router configured, using static device mapping\n")
			sys.deviceCache = dev.NewDeviceCache(nil, sys.config.Router)
		}
		err = sys.deviceCache.LoadStaticDevices(sys.config.StaticDevices)
		if err != nil {
			log.Printf("[ERROR] Failed to load static devices: %s\n", err.Error())
		} else {
			log.Printf("[INFO] Static devices loaded successfully\n")
			sys.deviceCache.Dump()
		}
	}

	// Attach device cache to the rules engine
	re.SetDeviceCache(sys.deviceCache)
	sys.rulesEngine = re
	sys.resolver = resolver.New(sys.config, sys.deviceCache)

	return &sys
}

// Tests the system configuration
func TestSystemConfig(cfgFileName string) (*System, error) {

	sys := System{}
	conf, err := sys.loadConfig(cfgFileName)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	sys.config = conf
	err = sys.validateConfig(conf)
	if err != nil {
		log.Panic(err)
		return nil, err
	}

	_, err = rules.NewRulesEngine(sys.config)
	if err != nil {
		log.Println("[ERROR] failed create rules engine: ", err.Error())
		//os.Exit(1)
		return nil, err
	}

	if sys.config.Router.Engine != cfg.RouterTypeNone {
		log.Printf("[INFO] Router configuration found - trying...")
		err = sys.initializeRouter(sys.config.Router)
		if err != nil {
			log.Printf("[ERROR] Router initialization failed: %s\n", err.Error())
			log.Printf("[WARN] Device Name lookup disabled - requires working router connection\n")
			// Don't return error in test mode - allow static devices to be used
			if len(sys.config.StaticDevices) == 0 {
				return nil, err
			}
		} else {
			dc := dev.NewDeviceCache(sys.routerClient, sys.config.Router)
			err = dc.Initialize()
			if err != nil {
				log.Printf("[ERROR] Device Cache initialization failed: %s\n", err.Error())
			} else {
				log.Printf("[INFO] Ok, device list downloaded")
				sys.deviceCache = dc
				dc.Dump()
			}
		}
	}

	// Load static devices if configured (for testing/demo without router hardware)
	if len(sys.config.StaticDevices) > 0 {
		if sys.deviceCache == nil {
			log.Printf("[INFO] No router configured, using static device mapping\n")
			sys.deviceCache = dev.NewDeviceCache(nil, sys.config.Router)
		}
		err = sys.deviceCache.LoadStaticDevices(sys.config.StaticDevices)
		if err != nil {
			log.Printf("[ERROR] Failed to load static devices: %s\n", err.Error())
		} else {
			log.Printf("[INFO] Static devices loaded successfully\n")
			sys.deviceCache.Dump()
		}
	}

	sys.resolver = resolver.New(sys.config, sys.deviceCache)

	return &sys, nil
}

func (sys *System) ReloadConfig() error {

	conf, err := sys.loadConfig("config.json")
	if err != nil {
		log.Printf("[Error] Unable to PARSE configuration\n")
		return err
	}

	// Take full mutex!!
	sys.mutex.Lock()
	defer sys.mutex.Unlock()

	err = sys.validateConfig(conf)
	if err != nil {
		log.Printf("[Error] Invalid configuration\n")
		return err
	}

	re, err := rules.NewRulesEngine(conf)
	if err != nil {
		log.Println("[ERROR] failed create rules engine: ", err.Error())
		return err
	}

	// Create new log file if it changed, otherwise keep old
	if conf.Logfile != sys.config.Logfile {
		err = sys.performanceLog.Close()
		if err != nil {
			log.Println("[ERROR] Unable to close old log file: ", err.Error())
			return err
		}

		logger, err := logging.NewLogFileClient(conf.Logfile)
		if err != nil {
			log.Panic(err.Error())
		}
		// Set it..
		sys.performanceLog = logger

	}

	re.SetDeviceCache(sys.deviceCache)
	sys.rulesEngine = re
	sys.config = conf

	return nil
}

// Config return internal config object
func (sys *System) Config() *cfg.Config {
	sys.mutex.Lock()
	defer sys.mutex.Unlock()
	return sys.config
}

// RulesEngine return internal RulesEngine object
func (sys *System) RulesEngine() *rules.RulesEngine {
	sys.mutex.Lock()
	defer sys.mutex.Unlock()
	return sys.rulesEngine
}

func (sys *System) Resolver() *resolver.Resolver {
	return sys.resolver
}

// RouterClient returns the internal/global RouterClient object
func (sys *System) RouterClient() dev.RouterClient {
	sys.mutex.Lock()
	defer sys.mutex.Unlock()
	return sys.routerClient
}

// DeviceCache returns the internal/global DeviceCache object
func (sys *System) DeviceCache() *dev.DeviceCache {
	sys.mutex.Lock()
	defer sys.mutex.Unlock()
	return sys.deviceCache
}

// SetDeviceCache installs a new device cache and wires it to dependent subsystems.
func (sys *System) SetDeviceCache(dc *dev.DeviceCache) {
	sys.mutex.Lock()
	defer sys.mutex.Unlock()
	sys.deviceCache = dc
	if sys.rulesEngine != nil {
		sys.rulesEngine.SetDeviceCache(dc)
	}
	if sys.resolver != nil {
		sys.resolver.SetDeviceCache(dc)
	}
}

func (sys *System) PerfLog() logging.LogClient {
	sys.mutex.Lock()
	defer sys.mutex.Unlock()
	return sys.performanceLog
}

// Support functions to get all subsystems up and running
func (sys *System) initializeRouter(router cfg.Router) error {
	var routerClient dev.RouterClient
	switch router.Engine {
	//case RouterTypeNetGear:
	//	routerClient = NewNetGearRouterClient(&router)
	//	break
	case cfg.RouterTypeUnifi:
		routerClient = dev.NewUnifiRouterClient(&router)
		break
	default:
		return fmt.Errorf("Unknown router type '%s', check configuration", router.Engine.String())
	}

	devices, err := routerClient.GetAttachedDeviceList()
	if err != nil {
		log.Printf("[ERROR] System::initializeRouter, failed to get attached device list, %s\n", err.Error())
		return err
	}
	log.Printf("[OK] Router is working, devices:\n")
	for _, d := range devices {
		log.Printf("  %s - %s\n", d.IP, d.Name)
	}

	sys.routerClient = routerClient

	return nil
}

func (sys *System) validateConfig(config *cfg.Config) error {
	// defaults
	if config.BlockStrategy.Action == "" {
		config.BlockStrategy.Action = "nullroute"
	}
	if config.IPv4BlockResolve == "" {
		config.IPv4BlockResolve = "0.0.0.0"
	}
	if config.IPv6BlockResolve == "" {
		config.IPv6BlockResolve = "::"
	}
	if config.UpstreamTimeoutMs == 0 {
		config.UpstreamTimeoutMs = 2000
	}
	if config.UpstreamRetries == 0 {
		config.UpstreamRetries = 2
	}
	if config.UpstreamFailWindowSec == 0 {
		config.UpstreamFailWindowSec = 30
	}
	if config.NegativeCacheTTL == 0 {
		config.NegativeCacheTTL = 30
	}
	return config.Validate()
}

func (sys *System) loadConfig(filename string) (*cfg.Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var conf = new(cfg.Config)
	err = json.Unmarshal(data, &conf)
	if err != nil {
		return nil, err
	}
	return conf, nil

}
