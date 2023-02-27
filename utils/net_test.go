package utils

import (
	"fmt"
	"gitee.com/sy_183/common/flag"
	"net"
	"testing"
)

func TestNetInterface(t *testing.T) {
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if flag.TestFlag(int(iface.Flags), int(net.FlagUp)) &&
			!flag.TestFlag(int(iface.Flags), int(net.FlagLoopback)) {
			addresses, err := iface.Addrs()
			if err != nil {
				t.Fatal(err)
			}
			for _, address := range addresses {
				fmt.Println(address.Network() + ":" + address.String())
			}
		}
	}
}
