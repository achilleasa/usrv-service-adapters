package etcd

import (
	"io/ioutil"
	"log"
	"regexp"
	"strings"

	"github.com/achilleasa/service-adapters"
	etcdPkg "github.com/coreos/go-etcd/etcd"
)

var (
	etcdValRe = regexp.MustCompile("(\\S+)=(\\S+)")
)

type Etcd struct {
	// The etcd client instance
	client *etcdPkg.Client

	// A logger for service events.
	logger *log.Logger
}

// Create a new etcd client using the specified list of comma-delimited hosts.
func New(etcdHosts string) *Etcd {
	hosts := strings.Split(etcdHosts, ",")

	return &Etcd{
		client: etcdPkg.NewClient(hosts),
		logger: log.New(ioutil.Discard, "", log.LstdFlags),
	}
}

// Register a logger instance for service events.
func (s *Etcd) SetLogger(logger *log.Logger) {
	s.logger = logger
}

// Configuration middleware for service adaptors. It returns a ServiceOption that
// monitors an etcd path and triggers a service reconfiguration when it changes.
func Config(etcdSrv *Etcd, etcdKey string) adapters.ServiceOption {
	// Create a monitor for the path
	monitorChan := make(chan *etcdPkg.Response)
	go etcdSrv.client.Watch(etcdKey, 0, false, monitorChan, nil)

	return func(s adapters.Service) error {
		// Fetch initial settings
		cur, err := etcdSrv.client.Get(etcdKey, false, false)
		if err != nil {
			etcdSrv.logger.Printf("[ETCD] Error retrieving current settings for key '%s': %v\n", etcdKey, err)
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
