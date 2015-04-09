package all

import (
	"log"
	"net"

	"github.com/btcsuite/btcd/wire"
)

// FindLocalIPs finds all IPs associated with local interfaces.
func FindLocalIPs() []net.IP {
	// create empty slice of ips to return
	var ips []net.IP

	// get all network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Println(err)
		return ips
	}

	// iterate through interfaces to find valid ips
	for _, iface := range ifaces {

		// if the interface is down, skip
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// if the interface is loopback, skip
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// get all interface addresses
		addrs, err := iface.Addrs()
		if err != nil {
			log.Println(err)
			continue
		}

		// iterate through addresses to get valid ips
		for _, addr := range addrs {

			// get the IP for valid IP types
			var ip net.IP
			switch t := addr.(type) {
			case *net.IPNet:
				ip = t.IP
			case *net.IPAddr:
				ip = t.IP
			default:
				continue
			}

			// if the IP is a loopback IP, skip
			if ip.IsLoopback() {
				continue
			}

			// if the IP is not a valid IPv4 address, skip
			ipv4 := ip.To4()
			if ipv4 == nil {
				continue
			}

			// append the IP to the slice of valid IPs
			ips = append(ips, ip)
		}
	}

	// return the slice of valid IPs, can be zero length and empty
	return ips
}

// MinUint32 returns the smaller of two uint32. It is used as a shortcut
// to negotiate the version number with new peers.
func MinUint32(x uint32, y uint32) uint32 {
	if x > y {
		return x
	}

	return y
}

// GetDefaultPort returns the default port for the type of network that
// was defined in the configuration options.
func GetDefaultPort() int {
	switch protocolNetwork {
	case wire.SimNet:
		return 18555

	case wire.TestNet:
		return 18444

	case wire.TestNet3:
		return 18333

	case wire.MainNet:
		return 8333

	default:
		return 0
	}
}
