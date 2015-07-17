package redis

import (
	"fmt"
	"log"
	"sync"

	"time"

	"io/ioutil"

	"strconv"

	"strings"

	"github.com/achilleasa/usrv-service-adapters"
	"github.com/achilleasa/usrv-service-adapters/dial"
	redisDriver "github.com/garyburd/redigo/redis"
)

// Adapter is a singleton instance of a redis service
var Adapter *Redis

// Initialize the service using default values
func init() {
	Adapter = &Redis{
		endpoint:          "localhost:3679",
		password:          "",
		db:                0,
		connectionTimeout: time.Second * 1,
		logger:            log.New(ioutil.Discard, "", log.LstdFlags),
		dialPolicy:        dial.Periodic(1, time.Second),
		closeNotifier:     adapters.NewNotifier(),
	}
}

type Redis struct {

	// The redis endpoint to connect to. Set manually by the user or discovered
	// by a configuration service (e.g. etcd)
	endpoint string

	// Redis password (used if non-empty)
	password string

	// Redis DB number
	db int

	// Connection timeout
	connectionTimeout time.Duration

	// A logger for service events.
	logger *log.Logger

	// A mutex protecting dial attempts.
	sync.Mutex

	// The dial policy to use.
	dialPolicy dial.Policy

	// Connection status.
	connected bool

	// Redis pool
	pool *redisDriver.Pool

	// A notifier for close events.
	closeNotifier *adapters.Notifier
}

// Connect to the service. If a dial policy has been specified,
// the service will keep trying to reconnect until a connection
// is established or the dial policy aborts the reconnection attempt.
func (s *Redis) Dial() error {
	s.Lock()
	defer s.Unlock()

	// We are already connected
	if s.connected {
		return adapters.ErrAlreadyConnected
	}

	s.setupPool()

	return nil
}

// Setup the connection pool. This method is not thread-safe
// so it should be invoked while holding the service lock.
func (s *Redis) setupPool() {

	// Create a new pool
	s.pool = &redisDriver.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial:        s.dialPoolConnection,
		TestOnBorrow: func(c redisDriver.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}

	s.connected = true
	s.dialPolicy.ResetAttempts()
}

// Redis pool dialer. This method is invoked whenever the redis pool allocates a new connection
func (s *Redis) dialPoolConnection() (redisDriver.Conn, error) {
	s.Lock()
	defer s.Unlock()

	var err error
	var wait time.Duration
	var c redisDriver.Conn
	wait, err = s.dialPolicy.NextRetry()
	for {
		c, err = redisDriver.DialTimeout("tcp", s.endpoint, s.connectionTimeout, 0, 0)
		if err == nil {
			break
		}

		wait, err = s.dialPolicy.NextRetry()
		if err != nil {
			s.logger.Printf("Could not connect to REDIS endpoint %s after %d attempt(s)\n", s.endpoint, s.dialPolicy.CurAttempt())
			return nil, dial.ErrTimeout
		}
		s.logger.Printf("Could not connect to REDIS endpoint %s; retrying in %v\n", s.endpoint, wait)
		<-time.After(wait)
	}

	if s.password != "" {
		if _, err = c.Do("AUTH", s.password); err != nil {
			c.Close()
			return nil, err
		}
	}
	if s.db > 0 {
		if _, err = c.Do("SELECT", s.db); err != nil {
			c.Close()
			return nil, err
		}
	}

	return c, err
}

// Disconnect.
func (s *Redis) Close() {
	s.Lock()
	defer s.Unlock()

	if !s.connected {
		return
	}

	// Close connection and notify any registered listeners
	s.closeNotifier.NotifyAll(adapters.ErrConnectionClosed)
	s.pool.Close()
	s.connected = false
}

// Register a listener for receiving close notifications. The service adapter will emit an error and
// close the channel if the service is cleanly shut down or close the channel if the connection is reset.
func (s *Redis) NotifyClose(c adapters.CloseListener) {
	s.closeNotifier.Add(c)
}

// Apply a list of options to the service.
func (s *Redis) SetOptions(opts ...adapters.ServiceOption) error {
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return err
		}
	}
	return nil
}

// Register a logger instance for service events.
func (s *Redis) SetLogger(logger *log.Logger) {
	s.logger = logger
}

// Set a dial policy for this service.
func (s *Redis) SetDialPolicy(policy dial.Policy) {
	s.dialPolicy = policy
}

// Set the service configuration. Changing the configuration settings for an already connected
// service will trigger a service shutdown. The service consumer is responsible for handing
// service close events and triggering a re-dial.
func (s *Redis) Config(params map[string]string) error {
	s.Lock()
	defer s.Unlock()

	needsReset := false

	endpoint, exists := params["endpoint"]
	if exists {
		s.endpoint = endpoint
		needsReset = true
	}

	password, exists := params["password"]
	if exists {
		s.password = password
		needsReset = true
	}

	dbVal, exists := params["db"]
	if exists {
		db, err := strconv.Atoi(dbVal)
		if err != nil {
			err := fmt.Errorf("invalid value for setting 'db': %s\n", dbVal)
			s.logger.Println("[REDIS] Configuration error: %s", err.Error())
			return err
		}
		s.db = db
		needsReset = true
	}

	timeoutVal, exists := params["connTimeout"]
	if exists {
		timeout, err := strconv.Atoi(timeoutVal)
		if err != nil {
			err := fmt.Errorf("invalid value for setting 'connTimeout': %s\n", timeoutVal)
			s.logger.Println("[REDIS] Configuration error: %s", err.Error())
			return err
		}
		s.connectionTimeout = time.Duration(timeout) * time.Second
		needsReset = true
	}

	if needsReset {
		s.logger.Printf("[REDIS] Configuration changed; new settings:  endpoint=%s, password=%s, db=%d, connTimeout=%v\n",
			s.endpoint,
			strings.Repeat("*", len(s.password)),
			s.db,
			s.connectionTimeout,
		)

		// Re-init connection pool
		s.setupPool()

		if s.connected {
			s.closeNotifier.NotifyAll(nil)
		}
	}

	return nil
}

// Fetch a connection from the pool.
func (s *Redis) GetConnection() (redisDriver.Conn, error) {
	s.Lock()
	if !s.connected {
		s.Unlock()
		return nil, adapters.ErrConnectionClosed
	}
	s.Unlock()

	conn := s.pool.Get()
	if conn.Err() != nil {
		return nil, conn.Err()
	}
	return conn, nil
}
