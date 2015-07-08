package amqp

import (
	"fmt"
	"log"
	"sync"

	"time"

	"io/ioutil"

	"github.com/achilleasa/service-adapters"
	"github.com/achilleasa/usrv"
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
	dialPolicy adapters.DialPolicy

	// Connection status.
	connected bool

	// AMQP connection handle.
	conn *amqp.Connection

	// A notifier for close events.
	closeNotifier *adapters.Notifier
}

func New(amqpEndpoint string) *Amqp {
	return &Amqp{
		endpoint:      amqpEndpoint,
		logger:        log.New(ioutil.Discard, "", log.LstdFlags),
		dialPolicy:    adapters.PeriodicPolicy(1, time.Second),
		closeNotifier: adapters.NewNotifier(),
	}
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
		s.logger.Printf("Connecting to AMQP endpoint %s; attempt %d", s.endpoint, s.dialPolicy.CurAttempt())
		s.conn, err = amqp.Dial(s.endpoint)
		if err == nil {
			break
		}

		wait, err = s.dialPolicy.NextRetry()
		if err != nil {
			s.logger.Printf("Could not connect to AMQP endpoint %s after %d attempt(s)\n", s.endpoint, s.dialPolicy.CurAttempt())
			return usrv.ErrDialFailed
		}
		fmt.Errorf("Could not connect to AMQP endpoint %s; retrying in %v\n", s.endpoint, wait)
		<-time.After(wait)
	}

	s.connected = true
	s.dialPolicy.ResetAttempts()
	s.logger.Printf("Connected to AMQP endpoint %s\n", s.endpoint)

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
	s.closeNotifier.NotifyAll(adapters.ErrConnectionClosed)
	s.conn = nil
	s.connected = false
}

// Register a logger instance for service events.
func (s *Amqp) SetLogger(logger *log.Logger) {
	s.logger = logger
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
			s.logger.Printf("Disconnected from AMQP endpoint %s\n", s.endpoint)
		} else {
			s.closeNotifier.NotifyAll(nil)
			s.logger.Printf("Lost connection to AMQP endpoint %s\n", s.endpoint)

		}
	}
}
