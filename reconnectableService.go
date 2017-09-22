package app

import (
	"sync/atomic"
	"time"

	"sync"

	"github.com/cenkalti/backoff"
	"github.com/go-errors/errors"
)

//ReconnectableService is a service that can reconnect in case of an error,
//employing backoff strategy. The reconnectable service methods are not thread safe,
//they shouldn't be called from multiple goroutines.
type ReconnectableService struct {
	mu sync.Mutex

	backoff   backoff.BackOff
	name      string
	connected int32
	stopped   int32
	timeout   time.Duration

	connectFunc func() error
	notifyFunc  func(err error, d time.Duration)
}

//NewReconnectableService creates a new reconnectable service with a specified backoff and timeout
func NewReconnectableService(name string, b backoff.BackOff, timeout time.Duration) *ReconnectableService {
	return &ReconnectableService{
		mu:        sync.Mutex{},
		backoff:   b,
		name:      name,
		timeout:   timeout,
		connected: 0,
		stopped:   0,
	}
}

//NewDefaultReconnectableService creates a new reconnectable service with a default backoff
//MaxInterval of 5 seconds and timeout of 5 seconds
func NewDefaultReconnectableService(name string) *ReconnectableService {
	b := backoff.NewExponentialBackOff()
	b.MaxInterval = time.Second * 5
	return NewReconnectableService(name, b, time.Second*5)
}

//Connect connects a specific service
func (r *ReconnectableService) Connect(f func() error) error {
	return r.ConnectNotify(f, nil)
}

//ConnectNotify connects a specific service and notifies on each error
func (r *ReconnectableService) ConnectNotify(f func() error, notify func(err error, d time.Duration)) error {
	r.mu.Lock()
	r.connectFunc = f
	r.notifyFunc = notify
	r.mu.Unlock()
	atomic.StoreInt32(&r.stopped, 0)

	tries := 0
	err := backoff.RetryNotify(func() error {
		stopped := atomic.LoadInt32(&r.stopped) == 1
		if stopped || Stopped() {
			return nil
		}

		connectChan := make(chan error)

		go func() {
			connectChan <- f()
		}()

		select {
		case <-time.After(r.timeout):
			return errors.New("Timeout")
		case err := <-connectChan:
			return err
		}
	}, r.backoff, func(err error, d time.Duration) {
		tries++
		Log().Errorf("Error in service %s connection: %s, after: %s time and %d tries",
			r.name, err, d, tries)

		if notify != nil {
			notify(err, d)
		}
	})

	if err != nil {
		Log().Errorf("Failed connecting to service: %s, error: %s", r.name, err)
		return err
	}

	Log().Infof("Successfully connected service %s", r.name)
	atomic.StoreInt32(&r.connected, 1)
	return nil
}

//Reconnect reconnects the service with the supplied func in Connect
func (r *ReconnectableService) Reconnect() error {
	atomic.StoreInt32(&r.connected, 0)
	return r.ConnectNotify(r.connectFunc, r.notifyFunc)
}

//IsConnected returns whether the service is connected
func (r *ReconnectableService) IsConnected() bool {
	return atomic.LoadInt32(&r.connected) == 1
}

//Name returns the service name
func (r *ReconnectableService) Name() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.name
}

//Stop stops the service reconnection
func (r *ReconnectableService) Stop() {
	Log().Infof("Stopping %s reconnection", r.Name())
	atomic.StoreInt32(&r.stopped, 1)
}
