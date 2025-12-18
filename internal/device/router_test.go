package device

import (
	"log"
	"testing"

	cfg "dnsproxy/internal/config"
)

var glbRouter RouterClient

func getRouter() RouterClient {
	if glbRouter != nil {
		return glbRouter
	}
	routerCfg := &cfg.Router{
		Host:         "192.168.1.30",
		Port:         "8443",
		User:         "Fredrik",
		Password:     "neger6slakt",
		Engine:       cfg.RouterTypeUnifi,
		PollChanges:  false,
		PollInterval: 60,
		TimeoutSec:   10,
	}
	glbRouter = NewUnifiRouterClient(routerCfg)
	return glbRouter
}

func TestRouterLogin(t *testing.T) {
	t.Skip("requires router hardware and network access")
	router := getRouter()
	err := router.Login("192.168.1.30", "8443", "Fredrik", "neger6slakt")
	if err != nil {
		t.Error(err)
	}
}

func TestRouterGetAttachedDevices(t *testing.T) {
	t.Skip("requires router hardware and network access")
	router := getRouter()
	if router == nil {
		t.Error("No router")
	}
	devices, err := router.GetAttachedDeviceList()
	if err != nil {
		t.Error(err)
	}
	for _, d := range devices {
		log.Printf("%s\t%s\t(%s)\n", d.MAC.String(), d.IP.String(), d.Name)
	}
	//	log.Println(devices)
}
