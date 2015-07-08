package adapters

import "sync"

type Notifier struct {

	// A mutex protecting the listeners.
	sync.Mutex

	// A list of listeners to be notified.
	listeners []chan error
}

// Create new notifier.
func NewNotifier() *Notifier {
	return &Notifier{
		listeners: make([]chan error, 0),
	}
}

// Register a listener.
func (n *Notifier) Add(listener chan error) {
	n.Lock()
	defer n.Unlock()

	n.listeners = append(n.listeners, listener)
}

// Notify all listeners, close their channels and remove them from the notification list. If err is not nil, it
// will be emitted to each listener before closing their channels.
func (n *Notifier) NotifyAll(err error) {
	n.Lock()
	defer n.Unlock()

	for _, listener := range n.listeners {
		if err != nil {
			listener <- err
		}
		close(listener)
	}

	// empty list
	n.listeners = make([]chan error, 0)
}
