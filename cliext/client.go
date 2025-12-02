package cliext

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ClientOptions contains options for creating a Temporal client.
// This mirrors client.Options from the SDK but with extension-specific defaults.
type ClientOptions struct {
	// HostPort is the Temporal server address.
	HostPort string

	// Namespace is the Temporal namespace.
	Namespace string

	// ConnectionOptions contains gRPC connection options.
	ConnectionOptions client.ConnectionOptions

	// Credentials are used for authentication.
	Credentials client.Credentials

	// Logger is the logger to use.
	Logger interface{}

	// MetricsHandler handles metrics.
	MetricsHandler interface{}

	// Identity identifies this client.
	Identity string
}

// Dial creates a new Temporal client using the client flags.
//
// Example:
//
//	c, err := clientFlags.Dial(ctx)
//	if err != nil {
//	    return err
//	}
//	defer c.Close()
func (f *ClientFlags) Dial(ctx context.Context) (client.Client, error) {
	opts, err := f.DialOptions()
	if err != nil {
		return nil, err
	}

	c, err := client.DialContext(ctx, opts)
	if err != nil {
		return nil, &ConnectionError{
			Address: f.Address,
			Err:     err,
		}
	}

	return c, nil
}

// DialOptions returns the client.Options configured from the flags.
// This allows callers to modify options before dialing.
//
// Example:
//
//	opts, err := clientFlags.DialOptions()
//	if err != nil {
//	    return err
//	}
//	opts.Logger = myCustomLogger
//	c, err := client.Dial(opts)
func (f *ClientFlags) DialOptions() (client.Options, error) {
	opts := client.Options{
		HostPort:  f.Address,
		Namespace: f.Namespace,
	}

	// Configure TLS
	tlsConfig, err := f.TLSConfig()
	if err != nil {
		return opts, &TLSError{Err: err}
	}
	if tlsConfig != nil {
		opts.ConnectionOptions.TLS = tlsConfig
	}

	// Configure API key authentication
	if f.APIKey != "" {
		opts.Credentials = client.NewAPIKeyStaticCredentials(f.APIKey)
	}

	// Configure codec endpoint
	// Note: Codec endpoint support requires additional implementation.
	// For now, we just note that it's configured but don't set DataConverter.
	// A full implementation would use converter.NewRemotePayloadCodec.
	_ = f.CodecEndpoint
	_ = f.CodecAuth

	// Add gRPC interceptor for additional headers if needed
	if f.APIKey != "" {
		opts.ConnectionOptions.DialOptions = append(
			opts.ConnectionOptions.DialOptions,
			grpc.WithUnaryInterceptor(apiKeyInterceptor(f.APIKey)),
		)
	}

	return opts, nil
}

// apiKeyInterceptor creates a gRPC interceptor that adds the API key header.
func apiKeyInterceptor(apiKey string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ConnectionError represents a failure to connect to the Temporal server.
type ConnectionError struct {
	Address string
	Err     error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("failed to connect to Temporal server at %s: %v", e.Address, e.Err)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// TLSError represents a TLS configuration error.
type TLSError struct {
	Err error
}

func (e *TLSError) Error() string {
	return fmt.Sprintf("TLS configuration error: %v", e.Err)
}

func (e *TLSError) Unwrap() error {
	return e.Err
}

// IsConnectionError returns true if the error is a connection error.
func IsConnectionError(err error) bool {
	_, ok := err.(*ConnectionError)
	return ok
}

// IsTLSError returns true if the error is a TLS error.
func IsTLSError(err error) bool {
	_, ok := err.(*TLSError)
	return ok
}
