// deprecated!
package device

import (
	"net"
	"strings"

	cfg "dnsproxy/internal/config"

	netgear "github.com/DRuggeri/netgear_client"
)

type NetGearRouterClient struct {
	config *cfg.Router
	router *netgear.NetgearClient
}

func NewNetGearRouterClient(config *cfg.Router) RouterClient {
	client := NetGearRouterClient{
		config: config,
	}
	client.Login(config.Host, config.Port, config.User, config.Password)
	return &client
}

func (client *NetGearRouterClient) Login(host, port, user, pass string) error {
	// API signature changed - adding default timeout and ssl verify params
	client.router, _ = netgear.NewNetgearClient(host, true, user, pass, 10, true)
	return client.router.LogIn()
}
func (client *NetGearRouterClient) GetAttachedDeviceList() ([]RouterDevice, error) {
	devices, err := client.router.GetAttachDevice2()
	if err != nil {
		return nil, err
	}

	return client.transformDevices(devices)
}

func (client *NetGearRouterClient) transformDevices(devices []map[string]string) ([]RouterDevice, error) {
	rdlist := make([]RouterDevice, 0)
	for _, d := range devices {
		name := d["DeviceName"]
		ipstr := d["IP"]
		macstr := d["MAC"]

		// Replace unknown's with 'Mac' addresses
		if name == "<unknown>" || name == "" {
			name = macstr
		}

		ip := net.ParseIP(ipstr)
		mac, _ := net.ParseMAC(macstr)

		rd := RouterDevice{
			IP:       ip,
			Name:     strings.ToLower(name),
			MAC:      mac,
			Type:     d["ConnectionType"],
			LinkRate: 0, // Not available in new API
			Signal:   0, // Not available in new API
		}
		rdlist = append(rdlist, rd)
	}
	return rdlist, nil
}
