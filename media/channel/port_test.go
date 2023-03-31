package channel

import (
	"fmt"
	"gitee.com/sy_183/common/uns"
	netUtils "gitee.com/sy_183/common/utils/net"
	"net"
	"testing"
	"time"
)

func TestPort(t *testing.T) {
	socket, port, err := netUtils.AllocPort("tcp", net.IP{192, 168, 80, 1})
	if err != nil {
		panic(err)
	}

	fmt.Println(port)
	time.Sleep(time.Second)

	if err := socket.Close(); err != nil {
		panic(err)
	}

	tcpConn, err := net.DialTCP("tcp",
		&net.TCPAddr{IP: net.IP{192, 168, 80, 1}, Port: port},
		&net.TCPAddr{IP: net.IP{192, 168, 80, 1}, Port: 5004},
	)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := tcpConn.Close(); err != nil {
			panic(err)
		}
	}()

	if _, err := tcpConn.Write(uns.StringToBytes("hello world\n")); err != nil {
		panic(err)
	}
}
