package natstrace

import (
	"errors"

	nats "github.com/nats-io/nats.go"
)

// ErrInitTracerRequired is returned when Connect is called before InitTracer.
var ErrInitTracerRequired = errors.New("natstrace: InitTracer must be called before Connect")

// Connect establishes a NATS connection with tracing. Signature aligns with nats.Connect.
// InitTracer must be called first or Connect returns ErrInitTracerRequired.
// TracerProvider and Propagators default to the global (otel.GetTracerProvider/GetTextMapPropagator).
// For custom provider or propagator, use ConnectWithOptions.
func Connect(url string, natsOpts ...nats.Option) (*Conn, error) {
	return ConnectWithOptions(url, filterNilOptions(natsOpts))
}

// ConnectWithOptions establishes a NATS connection with tracing and optional TracerProvider/Propagators.
// If tracer is not initialized, InitTracer("", nil) is called once.
func ConnectWithOptions(url string, natsOpts []nats.Option, traceOpts ...Option) (*Conn, error) {
	if !isTracerInitialized() {
		if err := InitTracer("", nil); err != nil {
			return nil, err
		}
	}
	nc, err := nats.Connect(url, natsOpts...)
	if err != nil {
		return nil, err
	}
	return newConn(nc, traceOpts...), nil
}

func filterNilOptions(natsOpts []nats.Option) []nats.Option {
	var out []nats.Option
	for _, o := range natsOpts {
		if o != nil {
			out = append(out, o)
		}
	}
	return out
}

// ConnectTLS establishes a TLS connection with tracing.
func ConnectTLS(url, certFile, keyFile, caFile string, natsOpts ...nats.Option) (*Conn, error) {
	return ConnectTLSWithOptions(url, certFile, keyFile, caFile, filterNilOptions(natsOpts), nil)
}

// ConnectTLSWithOptions is ConnectTLS with optional trace options.
func ConnectTLSWithOptions(url, certFile, keyFile, caFile string, natsOpts []nats.Option, traceOpts ...Option) (*Conn, error) {
	if !isTracerInitialized() {
		return nil, ErrInitTracerRequired
	}
	opts := make([]nats.Option, 0, len(natsOpts)+2)
	opts = append(opts, natsOpts...)
	opts = append(opts, nats.ClientCert(certFile, keyFile))
	if caFile != "" {
		opts = append(opts, nats.RootCAs(caFile))
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, err
	}
	return newConn(nc, traceOpts...), nil
}

// ConnectWithCredentials connects using a credentials file, with tracing.
func ConnectWithCredentials(url, credFile string, natsOpts ...nats.Option) (*Conn, error) {
	return ConnectWithCredentialsWithOptions(url, credFile, filterNilOptions(natsOpts), nil)
}

// ConnectWithCredentialsWithOptions is ConnectWithCredentials with optional trace options.
func ConnectWithCredentialsWithOptions(url, credFile string, natsOpts []nats.Option, traceOpts ...Option) (*Conn, error) {
	if !isTracerInitialized() {
		return nil, ErrInitTracerRequired
	}
	opts := make([]nats.Option, 0, len(natsOpts)+1)
	opts = append(opts, natsOpts...)
	opts = append(opts, nats.UserCredentials(credFile))
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, err
	}
	return newConn(nc, traceOpts...), nil
}
