package power

// MockWolSender is a mock implementation of WolSender
type MockWolSender struct {
	WakeCalled    bool
	WakeCallCount int
	LastMAC       string
	LastIP        string
	LastPort      int
	ReturnError   error
}

func (m *MockWolSender) Wake(macAddress string, port int, broadcastIP string) error {
	m.WakeCalled = true
	m.WakeCallCount++
	m.LastMAC = macAddress
	m.LastIP = broadcastIP
	m.LastPort = port
	return m.ReturnError
}

// MockSSHClient is a mock implementation of SSHClient
type MockSSHClient struct {
	ShutdownCalled    bool
	ShutdownCallCount int
	LastHost          string
	LastUser          string
	ReturnError       error
}

func (m *MockSSHClient) Shutdown(host string, user string, key string) error {
	m.ShutdownCalled = true
	m.ShutdownCallCount++
	m.LastHost = host
	m.LastUser = user
	return m.ReturnError
}

// MockIPMIClient is a mock implementation of IPMIClient
type MockIPMIClient struct {
	PowerOnCalled   bool
	PowerOffCalled  bool
	GetStatusCalled bool
	LastAddress     string
	LastUsername    string
	LastPassword    string
	PowerStatus     bool
	ReturnError     error
}

func (m *MockIPMIClient) PowerOn(address string, username string, password string) error {
	m.PowerOnCalled = true
	m.LastAddress = address
	m.LastUsername = username
	m.LastPassword = password
	return m.ReturnError
}

func (m *MockIPMIClient) PowerOff(address string, username string, password string) error {
	m.PowerOffCalled = true
	m.LastAddress = address
	m.LastUsername = username
	m.LastPassword = password
	return m.ReturnError
}

func (m *MockIPMIClient) GetPowerStatus(address string, username string, password string) (bool, error) {
	m.GetStatusCalled = true
	m.LastAddress = address
	m.LastUsername = username
	m.LastPassword = password
	return m.PowerStatus, m.ReturnError
}

// MockPinger is a mock implementation of Pinger
type MockPinger struct {
	Reachable     bool
	LastAddress   string
	PingCallCount int
}

func (m *MockPinger) IsReachable(address string) bool {
	m.PingCallCount++
	m.LastAddress = address
	return m.Reachable
}
