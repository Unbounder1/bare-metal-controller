package power

import (
	"fmt"
	"net"
)

type RealWolSender struct {
	DefaultPort             int
	DefaultBroadcastAddress string
}

func (w *RealWolSender) Wake(macAddress string, port int, broadcastAddress string) error {
	// Implementation to send Wake-on-LAN magic packet
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return err
	}

	packet := make([]byte, 102)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+(i*6):], mac)
	}

	if broadcastAddress == "" {
		broadcastAddress = w.DefaultBroadcastAddress
	}
	if port == 0 {
		port = w.DefaultPort
	}

	conn, err := net.Dial("udp", broadcastAddress+fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to dial UDP broadcast: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to send magic packet: %w", err)
	}

	return nil
}
