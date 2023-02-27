package utils

import (
	"net"
	"testing"
)

func TestDial(t *testing.T) {
	conn1, err := net.DialTCP("tcp",
		//&net.TCPAddr{IP: net.IP{192, 168, 1, 129}, Port: 35004},
		nil,
		&net.TCPAddr{IP: net.IP{144, 168, 1, 115}, Port: 5004},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Close()
	conn2, err := net.DialTCP("tcp",
		&net.TCPAddr{IP: net.IP{192, 168, 1, 129}, Port: 35004},
		&net.TCPAddr{IP: net.IP{192, 168, 1, 115}, Port: 5006},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
}

func TestDialTcp(t *testing.T) {
	conn, err := net.DialTCP("tcp", nil,
		&net.TCPAddr{IP: net.IP{101, 42, 30, 193}, Port: 5004},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(conn.LocalAddr())
	defer conn.Close()
}

func TestDialUdp(t *testing.T) {
	conn, err := net.DialUDP("udp", nil,
		&net.UDPAddr{IP: net.IP{192, 168, 81, 115}, Port: 5004},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(conn.LocalAddr())
	defer conn.Close()
}
