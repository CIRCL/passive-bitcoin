package adaptor

import (
	"net"
)

type Peer interface {
	String() string
	Addr() *net.TCPAddr
	Connect()
	Start()
	Stop()
	Greet()
	Poll()
	Wait()
}
