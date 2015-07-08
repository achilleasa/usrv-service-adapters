package adapters

import (
	"errors"
	"log"
)

// Common service errors

var (
	ErrAlreadyConnected = errors.New("Already connected")
	ErrTimeout          = errors.New("Connection timeout")
	ErrConnectionClosed = errors.New("Connection closed")
)

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
	NotifyClose(c chan error)

	// Register a logger instance for service events.
	SetLogger(logger *log.Logger)
}
