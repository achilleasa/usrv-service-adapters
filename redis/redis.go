package redis

import (
	"fmt"
	"log"
	"sync"

	"time"

	"io/ioutil"

	"github.com/achilleasa/service-adapters"
	"github.com/garyburd/redigo/redis"
)

type Redis struct {

	// The redis endpoint to connect to. Set manually by the user or discovered
	// by a configuration service (e.g. etcd)
	endpoint string

	// Redis password (used if non-empty)
	password string

	// Redis DB number
	db uint

	// Connection timeout
	connectionTimeout time.Duration

	// A logger for service events.
	logger *log.Logger

	// A mutex protecting dial attempts.
	sync.Mutex

	// The dial policy to use.
	dialPolicy adapters.DialPolicy

	// Connection status.
	connected bool

	// Redis pool
	pool *redis.Pool

	// A notifier for close events.
	closeNotifier *adapters.Notifier
}

func New(redisEndpoint string, password string, db uint, connectionTimeout time.Duration) *Redis {
	return &Redis{
		endpoint:          redisEndpoint,
		password:          password,
		db:                db,
		connectionTimeout: connectionTimeout,
		logger:            log.New(ioutil.Discard, "", log.LstdFlags),
		dialPolicy:        adapters.PeriodicPolicy(1, time.Second),
		closeNotifier:     adapters.NewNotifier(),
	}
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

	// Create a new pool
	s.pool = &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial:        s.dialPoolConnection,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}

	s.connected = true
	s.dialPolicy.ResetAttempts()

	// Start watchdog
	go s.watchdog()

	return nil
}

// Redis pool dialer. This method is invoked whenever the redis pool allocates a new connection
func (s *Redis) dialPoolConnection() (redis.Conn, error) {
	s.Lock()
	defer s.Unlock()

	var err error
	var wait time.Duration
	var c redis.Conn
	wait, err = s.dialPolicy.NextRetry()
	for {
		c, err = redis.DialTimeout("tcp", s.endpoint, s.connectionTimeout, 0, 0)
		if err == nil {
			break
		}

		wait, err = s.dialPolicy.NextRetry()
		if err != nil {
			s.logger.Printf("Could not connect to REDIS endpoint %s after %d attempt(s)\n", s.endpoint, s.dialPolicy.CurAttempt())
			return nil, adapters.ErrTimeout
		}
		fmt.Errorf("Could not connect to REDIS endpoint %s; retrying in %v\n", s.endpoint, wait)
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

// Register a logger instance for service events.
func (s *Redis) SetLogger(logger *log.Logger) {
	s.logger = logger
}

// Fetch a connection from the pool.
func (s *Redis) GetConnection() (redis.Conn, error) {
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

// A worker that listens for service-related notifications or configuration changes.
func (s *Redis) watchdog() {
}
