package power

import (
	"net"
	"time"
)

type RealPinger struct{}

func (p *RealPinger) IsReachable(address string) bool {
	const maxAttempts = 3
	const retryDelay = 500 * time.Millisecond

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if p.ping(address) {
			return true
		}
		if attempt < maxAttempts-1 {
			time.Sleep(retryDelay)
		}
	}
	return false
}

func (p *RealPinger) ping(address string) bool {
	netAddr, err := net.ResolveIPAddr("ip", address)
	if err != nil {
		return false
	}

	conn, err := net.DialIP("ip4:icmp", nil, netAddr)
	if err != nil {
		return false
	}
	defer conn.Close()

	// Send ICMP Echo Request
	msg := []byte{
		8, 0, 0, 0, 0, 0, 0, 0, // Type, Code, Checksum, Identifier, Sequence Number
	}
	checksum := 0
	for i := 0; i < len(msg); i += 2 {
		checksum += int(msg[i])<<8 + int(msg[i+1])
	}
	checksum = (checksum >> 16) + (checksum & 0xFFFF)
	checksum = ^checksum
	msg[2] = byte(checksum >> 8)
	msg[3] = byte(checksum & 0xFF)

	_, err = conn.Write(msg)
	if err != nil {
		return false
	}

	// Set a read deadline
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// Wait for ICMP Echo Reply
	reply := make([]byte, 1024)
	_, err = conn.Read(reply)
	return err == nil
}
