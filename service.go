package adapters

import (
	"errors"
	"log"

	"github.com/achilleasa/usrv-service-adapters/dial"
)

// Common service errors

var (
	ErrConnectionClosed = errors.New("Connection closed")
)

// A close listener is a channel that receives errors.
type CloseListener chan error

// All services in this package should implement this interface.
type Service interface {

	// Connect to the service. If a dial policy has been specified,
	// the service will keep trying to reconnect until a connection
	// is established or the dial policy aborts the reconnection attempt.
	Dial() error

	// Disconnect.
	Close()

	// Register a listener for receiving close notifications. The service adapter will emit an error and
	// close the channel if the service is cleanly shut down or close the channel if the connection is reset.
	NotifyClose(c CloseListener)

	// Apply service options. This is a convenience method for initializing the service
	// without invoking multiple methods.
	SetOptions(opts ...ServiceOption) error

	// Register a logger instance for service events.
	SetLogger(logger *log.Logger)

	// Set a dial policy for this service.
	SetDialPolicy(policy dial.Policy)

	// Set the service configuration. Changing the configuration settings for an already connected
	// service will trigger a service shutdown. The service consumer is responsible for handing
	// service close events and triggering a re-dial.
	Config(params map[string]string) error
}
