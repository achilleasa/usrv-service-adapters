package etcd

import (
	"io/ioutil"
	"log"
	"regexp"
	"strings"

	"errors"
	"sync"
	"time"

	"github.com/achilleasa/usrv-service-adapters"
	"github.com/achilleasa/usrv-service-adapters/dial"
	etcdPkg "github.com/coreos/go-etcd/etcd"
)

var (
	etcdValRe = regexp.MustCompile("(\\S+)=(\\S+)")
)

// Adapter is a singleton instance of a etcd service
var Adapter *Etcd = &Etcd{
	hosts:         make([]string, 0),
	client:        etcdPkg.NewClient(nil),
	logger:        log.New(ioutil.Discard, "", log.LstdFlags),
	dialPolicy:    dial.ExpBackoff(10, time.Millisecond),
	closeNotifier: adapters.NewNotifier(),
}

type Etcd struct {
	// The etcd hosts to connect to
	hosts []string

	// The etcd client instance
	client *etcdPkg.Client

	// A logger for service events.
	logger *log.Logger

	// A notifier for close events.
	closeNotifier *adapters.Notifier

	// Connection status.
	connected bool

	// The dial policy to use.
	dialPolicy dial.Policy

	// A mutex protecting the client
	sync.Mutex
}

// Connect to the service. If a dial policy has been specified,
// the service will keep trying to reconnect until a connection
// is established or the dial policy aborts the reconnection attempt.
func (s *Etcd) Dial() error {
	s.Lock()
	defer s.Unlock()

	// We are already connected
	if s.connected {
		return nil
	}

	if len(s.hosts) == 0 {
		return errors.New("No etcd hosts defined")
	}

	var err error
	var wait time.Duration
	s.dialPolicy.ResetAttempts()
	wait, err = s.dialPolicy.NextRetry()
	s.logger.Printf("[ETCD] Connecting to cluster hosts: %s\n", s.hosts)
	for {
		ok := s.client.SetCluster(s.hosts)
		if ok {
			break
		}

		wait, err = s.dialPolicy.NextRetry()
		if err != nil {
			s.logger.Printf("[ETCD] Could not connect any host in the cluster after %d attempt(s)\n", s.dialPolicy.CurAttempt())
			return dial.ErrTimeout
		}
		s.logger.Printf("[ETCD] Could not connect to any host in the cluster; retrying in %v\n", wait)
		<-time.After(wait)
	}

	s.connected = true
	s.dialPolicy.ResetAttempts()
	s.logger.Printf("[ETCD] Connected to cluster\n")

	return nil
}

// Disconnect.
func (s *Etcd) Close() {
	s.Lock()
	defer s.Unlock()

	s.client.Close()
	s.closeNotifier.NotifyAll(adapters.ErrConnectionClosed)
	s.connected = false
}

// Register a listener for receiving close notifications. The service adapter will emit an error and
// close the channel if the service is cleanly shut down or close the channel if the connection is reset.
func (s *Etcd) NotifyClose(c adapters.CloseListener) {
	s.closeNotifier.Add(c)
}

// Apply a list of options to the service.
func (s *Etcd) SetOptions(opts ...adapters.ServiceOption) error {
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return err
		}
	}
	return nil
}

// Register a logger instance for service events.
func (s *Etcd) SetLogger(logger *log.Logger) {
	s.logger = logger
	//etcdPkg.SetLogger(logger)
}

// Set a dial policy for this service.
func (s *Etcd) SetDialPolicy(policy dial.Policy) {
	s.dialPolicy = policy
}

// Set the service configuration. Changing the configuration settings for an already connected
// service will trigger a service shutdown. The service consumer is responsible for handing
// service close events and triggering a re-dial.
func (s *Etcd) Config(params map[string]string) error {
	s.Lock()
	defer s.Unlock()

	needsReset := false

	hosts, exists := params["hosts"]
	if exists {
		needsReset = true
		s.hosts = strings.Split(hosts, ",")
	}

	if needsReset {
		s.logger.Printf("[ETCD] Configuration changed; new settings: hosts=%s\n", hosts)
		s.client.SetCluster(s.hosts)
		s.client.SyncCluster()
		s.closeNotifier.NotifyAll(nil)
	}

	return nil
}

// Configuration middleware for service adaptors. It returns a ServiceOption that
// monitors an etcd path and triggers a service reconfiguration when it changes.
func AutoConf(etcdKey string) adapters.ServiceOption {
	// Create a monitor for the path
	monitorChan := make(chan *etcdPkg.Response)
	go Adapter.client.Watch(etcdKey, 0, false, monitorChan, nil)
	return func(s adapters.Service) error {
		// Fetch initial settings
		cur, err := Adapter.client.Get(etcdKey, false, false)
		if err != nil {
			Adapter.logger.Printf("[ETCD] Error retrieving current settings for key '%s': %v\n", etcdKey, err)
		} else if cur != nil {
			s.Config(tokenizeVal(cur.Node.Value))
		}

		// Wait for a path change
		go func() {
			for {
				r := <-monitorChan
				if r == nil {
					continue
				}

				s.Config(tokenizeVal(r.Node.Value))
			}
		}()

		return nil
	}
}

// Tokenize a received etcdValue with format k1=v1 k2=v2 into a map.
func tokenizeVal(etcdValue string) map[string]string {
	params := make(map[string]string)
	matches := etcdValRe.FindAllStringSubmatch(etcdValue, -1)

	// index 0 is the full capture
	// index 1 is the key
	// index 2 is the value
	for _, match := range matches {
		params[match[1]] = match[2]
	}

	return params
}
