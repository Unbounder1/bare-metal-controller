package power

import (
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

type RealSSHClient struct{}

func (s *RealSSHClient) Shutdown(host string, user string, key string) error {
	if key == "" {
		return fmt.Errorf("SSH private key is required")
	}

	signer, err := ssh.ParsePrivateKey([]byte(key))
	if err != nil {
		return fmt.Errorf("unable to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return fmt.Errorf("unable to connect to SSH server: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("unable to create SSH session: %w", err)
	}
	defer session.Close()

	err = session.Run("sudo shutdown -h now")
	if err != nil {
		// Connection drop during shutdown is expected
		if _, ok := err.(*ssh.ExitMissingError); ok {
			return nil
		}
		return fmt.Errorf("unable to execute shutdown command: %w", err)
	}

	return nil
}
