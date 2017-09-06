package app

import (
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-errors/errors"
)

//ReconnectableService is a service that can reconnect in case of an error,
//employing backoff strategy. The reconnectable service methods are not thread safe,
//they shouldn't be called from multiple goroutines.
type ReconnectableService struct {
	backoff   backoff.BackOff
	name      string
	connected bool
	stopped   bool
	timeout   time.Duration

	connectFunc func() error
	notifyFunc  func(err error, d time.Duration)
}

//NewReconnectableService creates a new reconnectable service with a specified backoff and timeout
func NewReconnectableService(name string, b backoff.BackOff, timeout time.Duration) *ReconnectableService {
	return &ReconnectableService{
		backoff: b,
		name:    name,
		timeout: timeout,
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
	r.stopped = false
	r.connectFunc = f
	r.notifyFunc = notify

	tries := 0
	err := backoff.RetryNotify(func() error {
		if r.stopped || Stopped() {
			return nil
		}

		timer := time.NewTimer(r.timeout)
		defer timer.Stop()

		connectChan := make(chan error)

		go func() {
			connectChan <- f()
		}()

		select {
		case <-timer.C:
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
	r.connected = true
	return nil
}

//Reconnect reconnects the service with the supplied func in Connect
func (r *ReconnectableService) Reconnect() error {
	r.connected = false
	return r.ConnectNotify(r.connectFunc, r.notifyFunc)
}

//IsConnected returns whether the service is connected
func (r *ReconnectableService) IsConnected() bool {
	return r.connected
}

//Name returns the service name
func (r *ReconnectableService) Name() string {
	return r.name
}

//Stop stops the service reconnection
func (r *ReconnectableService) Stop() {
	Log().Infof("Stopping %s reconnection", r.Name())
	r.stopped = true
}
