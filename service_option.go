package adapters

import (
	"log"

	"github.com/achilleasa/usrv-service-adapters/dial"
)

// Functional option arg type for services.
type ServiceOption func(Service) error

// Apply configuration settings to a service.
func Config(params map[string]string) ServiceOption {
	return func(s Service) error {
		return s.Config(params)
	}
}

// Attach a logger to a service.
func Logger(logger *log.Logger) ServiceOption {
	return func(s Service) error {
		s.SetLogger(logger)
		return nil
	}
}

// Attach a logger to a service.
func DialPolicy(policy dial.Policy) ServiceOption {
	return func(s Service) error {
		s.SetDialPolicy(policy)
		return nil
	}
}
