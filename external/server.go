package protos

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/Unbounder1/bare-metal-controller/protos"
)

// Options contains configuration for the gRPC server.
type Options struct {
	// Address is the address to listen on (e.g., ":8086")
	Address string

	// CertFile is the path to the TLS certificate file
	CertFile string

	// KeyFile is the path to the TLS key file
	KeyFile string

	// CAFile is the path to the CA certificate file
	CAFile string
}

// DefaultOptions returns the default server options.
func DefaultOptions() Options {
	return Options{
		Address:  ":8086",
		CertFile: "",
		KeyFile:  "",
		CAFile:   "",
	}
}

// BindFlags binds the gRPC server options to command line flags.
// The flagPrefix can be used to namespace the flags (e.g., "grpc-").
func (o *Options) BindFlags(fs *flag.FlagSet, prefix string) {
	fs.StringVar(&o.Address, prefix+"address", o.Address,
		"The address the gRPC cloud provider server binds to.")
	fs.StringVar(&o.CertFile, prefix+"cert", o.CertFile,
		"Path to TLS certificate file for gRPC server. Empty for insecure.")
	fs.StringVar(&o.KeyFile, prefix+"key", o.KeyFile,
		"Path to TLS key file for gRPC server. Empty for insecure.")
	fs.StringVar(&o.CAFile, prefix+"ca", o.CAFile,
		"Path to CA certificate file for gRPC client verification. Empty for insecure.")
}

// Validate validates the options.
func (o *Options) Validate() error {
	if o.Address == "" {
		return fmt.Errorf("address is required")
	}

	// If any TLS option is set, all must be set
	tlsOptions := []string{o.CertFile, o.KeyFile, o.CAFile}
	setCount := 0
	for _, opt := range tlsOptions {
		if opt != "" {
			setCount++
		}
	}

	if setCount > 0 && setCount < 3 {
		return fmt.Errorf("all TLS options (cert, key, ca) must be set together, or none")
	}

	return nil
}

// IsTLSEnabled returns true if TLS is configured.
func (o *Options) IsTLSEnabled() bool {
	return o.CertFile != "" && o.KeyFile != "" && o.CAFile != ""
}

// Server implements manager.Runnable for the gRPC cloud provider server.
type Server struct {
	options    Options
	client     client.Client
	grpcServer *grpc.Server
	listener   net.Listener
}

// Ensure Server implements manager.Runnable
var _ manager.Runnable = &Server{}

// NewServer creates a new gRPC server runnable.
func NewServer(opts Options, mgr manager.Manager) (*Server, error) {
	return &Server{
		options: opts,
		client:  mgr.GetClient(),
	}, nil
}

// Start implements manager.Runnable and starts the gRPC server.
// It blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	// Create gRPC server
	grpcServer, err := s.createGRPCServer()
	if err != nil {
		return fmt.Errorf("failed to create gRPC server: %w", err)
	}
	s.grpcServer = grpcServer

	// Register the bare metal provider
	bareMetalProvider := &protos.BareMetalProviderServer{
		Client: s.client,
	}
	protos.RegisterCloudProviderServer(s.grpcServer, bareMetalProvider)

	// Create listener
	listener, err := net.Listen("tcp", s.options.Address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.options.Address, err)
	}
	s.listener = listener

	// Start serving in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			errChan <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		s.grpcServer.GracefulStop()
		return nil
	case err := <-errChan:
		return fmt.Errorf("gRPC server error: %w", err)
	}
}

// createGRPCServer creates the gRPC server with optional TLS.
func (s *Server) createGRPCServer() (*grpc.Server, error) {
	// Check if TLS is configured
	if s.options.CertFile == "" || s.options.KeyFile == "" || s.options.CAFile == "" {
		return grpc.NewServer(), nil
	}

	// Load server certificate
	certificate, err := tls.LoadX509KeyPair(s.options.CertFile, s.options.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	// Load CA certificate
	certPool := x509.NewCertPool()
	caBytes, err := os.ReadFile(s.options.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	if !certPool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS12,
	}

	return grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig))), nil
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
// Returns false so the gRPC server runs on all replicas, not just the leader.
func (s *Server) NeedLeaderElection() bool {
	return false
}
