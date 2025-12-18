package device

import "net"

type RouterClient interface {
	Login(host, port, user, pass string) error
	GetAttachedDeviceList() ([]RouterDevice, error)
}

type RouterDevice struct {
	IP       net.IP
	Name     string
	MAC      net.HardwareAddr
	Type     string
	LinkRate int
	Signal   int
}
