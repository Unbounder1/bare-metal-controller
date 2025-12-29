package power

// WolSender sends Wake-on-LAN magic packets
type WolSender interface {
	Wake(macAddress string, port int) error
}

// SSHClient executes commands over SSH
type SSHClient interface {
	Shutdown(host string, user string) error
}

// IPMIClient controls servers via IPMI
type IPMIClient interface {
	PowerOn(address string, username string, password string) error
	PowerOff(address string, username string, password string) error
	GetPowerStatus(address string, username string, password string) (bool, error)
}

// Pinger checks if a host is reachable
type Pinger interface {
	IsReachable(address string) bool
}
