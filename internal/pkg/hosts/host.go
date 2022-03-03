package hosts

import "github.com/packethost/packngo"

type Host interface {
	Id() string
	Name() string
	IPv4() string
}

type equinixWrapper struct {
	device packngo.Device
}

func (ew *equinixWrapper) Name() string {
	return ew.device.Hostname
}

func (ew *equinixWrapper) Id() string {
	return ew.device.ID
}

func (ew *equinixWrapper) IPv4() string {
	return ew.device.GetNetworkInfo().PublicIPv4
}

func NewEquinixWrapper(device packngo.Device) Host {
	return &equinixWrapper{
		device: device,
	}
}
