package amqp

import (
	"fmt"
	"log"
	"sync"

	"time"

	"io/ioutil"

	"github.com/achilleasa/service-adapters"
	"github.com/achilleasa/service-adapters/dial"
	"github.com/streadway/amqp"
)

type Amqp struct {

	// The amqp endpoint to connect to. Set manually by the user or discovered
	// by a configuration service (e.g. etcd)
	endpoint string

	// A logger for service events.
	logger *log.Logger

	// A mutex protecting dial attempts.
	sync.Mutex

	// The dial policy to use.
	dialPolicy dial.Policy

	// Connection status.
	connected bool

	// AMQP connection handle.
	conn *amqp.Connection

	// A notifier for close events.
	closeNotifier *adapters.Notifier
}

// Create a new AMQP service adapter with default settings.
func New(options ...adapters.ServiceOption) (*Amqp, error) {
	amqpSrv := &Amqp{
		endpoint:      "localhost:55672",
		logger:        log.New(ioutil.Discard, "", log.LstdFlags),
		dialPolicy:    dial.Periodic(1, time.Second),
		closeNotifier: adapters.NewNotifier(),
	}

	// Apply any options
	for _, opt := range options {
		if err := opt(amqpSrv); err != nil {
			return nil, err
		}
	}

	return amqpSrv, nil
}

// Connect to the service. If a dial policy has been specified,
// the service will keep trying to reconnect until a connection
// is established or the dial policy aborts the reconnection attempt.
func (s *Amqp) Dial() error {
	s.Lock()
	defer s.Unlock()

	// We are already connected
	if s.connected {
		return adapters.ErrAlreadyConnected
	}

	var err error
	var wait time.Duration
	wait, err = s.dialPolicy.NextRetry()
	for {
		s.logger.Printf("[AMQP] Connecting to endpoint %s; attempt %d", s.endpoint, s.dialPolicy.CurAttempt())
		s.conn, err = amqp.Dial(s.endpoint)
		if err == nil {
			break
		}

		wait, err = s.dialPolicy.NextRetry()
		if err != nil {
			s.logger.Printf("[AMQP] Could not connect to endpoint %s after %d attempt(s)\n", s.endpoint, s.dialPolicy.CurAttempt())
			return dial.ErrTimeout
		}
		fmt.Errorf("[AMQP] Could not connect to endpoint %s; retrying in %v\n", s.endpoint, wait)
		<-time.After(wait)
	}

	s.connected = true
	s.dialPolicy.ResetAttempts()
	s.logger.Printf("[AMQP] Connected to endpoint %s\n", s.endpoint)

	// Start watchdog
	go s.watchdog()

	return nil
}

// Disconnect.
func (s *Amqp) Close() {
	s.Lock()
	defer s.Unlock()

	if !s.connected {
		return
	}

	// Close connection and notify any registered listeners
	s.conn.Close()
	s.closeNotifier.NotifyAll(adapters.ErrConnectionClosed)
	s.conn = nil
	s.connected = false
}

// Register a listener for receiving close notifications. The service adapter will emit an error and
// close the channel if the service is cleanly shut down or close the channel if the connection is reset.
func (s *Amqp) NotifyClose(c chan error) {
	s.closeNotifier.Add(c)
}

// Register a logger instance for service events.
func (s *Amqp) SetLogger(logger *log.Logger) {
	s.logger = logger
}

// Set a dial policy for this service.
func (s *Amqp) SetDialPolicy(policy dial.Policy) {
	s.dialPolicy = policy
}

// Set the service configuration. Changing the configuration settings for an already connected
// service will trigger a service shutdown. The service consumer is responsible for handing
// service close events and triggering a re-dial.
func (s *Amqp) Config(params map[string]string) error {
	s.Lock()
	defer s.Unlock()

	needsReset := false

	endpoint, exists := params["endpoint"]
	if exists {
		s.endpoint = endpoint
		needsReset = true
	}

	if needsReset {
		s.logger.Printf("[AMQP] Configuration changed; new settings: endpoint=%s\n", s.endpoint)
		if s.connected {
			s.conn.Close()
			s.closeNotifier.NotifyAll(nil)
			s.conn = nil
			s.connected = false
		}
	}

	return nil
}

// Allocate new amqp channel.
func (s *Amqp) NewChannel() (*amqp.Channel, error) {
	s.Lock()
	defer s.Unlock()

	if !s.connected {
		return nil, adapters.ErrConnectionClosed
	}

	return s.conn.Channel()
}

// A worker that listens for service-related notifications or configuration changes.
func (s *Amqp) watchdog() {
	amqpClose := make(chan *amqp.Error)
	s.conn.NotifyClose(amqpClose)

	select {
	case _, normalShutdown := <-amqpClose:
		if normalShutdown {
			s.closeNotifier.NotifyAll(adapters.ErrConnectionClosed)
			s.logger.Printf("[AMQP] Disconnected from endpoint %s\n", s.endpoint)
		} else {
			s.closeNotifier.NotifyAll(nil)
			s.logger.Printf("[AMQP] Lost connection to endpoint %s\n", s.endpoint)

		}
	}
}
