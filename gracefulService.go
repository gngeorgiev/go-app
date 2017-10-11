package app

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	gracefulTimeout = time.Minute * 1
)

//GracefulService knows how to shut itself down, synchronously
type GracefulService interface {
	Shutdown() error
	String() string
}

type gracefulChannelShutdown struct {
	name string
	ch   chan chan error
}

type gracefulServiceWrapper struct {
	s        GracefulService
	priority ShutdownPriority
}

func (g *gracefulServiceWrapper) Shutdown() error {
	return g.s.Shutdown()
}

func (g *gracefulServiceWrapper) String() string {
	return g.s.String()
}

//RegisterServices registers GracefulServices to the app, then each service is stopped,
//when the app is stopped
func RegisterServices(services ...GracefulService) {
	RegisterServicesPriority(ShutdownPriorityNormal, services...)
}

func RegisterServicesPriority(priority ShutdownPriority, services ...GracefulService) {
	app.servicesMutex.Lock()
	defer app.servicesMutex.Unlock()

	for _, s := range services {
		app.services = append(app.services, &gracefulServiceWrapper{
			s:        s,
			priority: priority,
		})
	}
}

//ShutdownSignalReceived is used to get notified when an os terminate signal is received
//by the application to clean up resources before exiting
//each channel call will block with a timeout
func ShutdownSignalReceived(shutdown chan chan error, identifier string) {
	ShutdownSignalReceivedPriority(shutdown, identifier, ShutdownPriorityNormal)
}

//ShutdownSignalReceived is used to get notified when an os terminate signal is received
//by the application to clean up resources before exiting
//each channel call will block with a timeout
func ShutdownSignalReceivedPriority(shutdown chan chan error, identifier string, priority ShutdownPriority) {
	adhocShutdown := &gracefulServiceWrapper{
		s: &gracefulChannelShutdown{
			name: identifier,
			ch:   shutdown,
		},
		priority: priority,
	}

	RegisterServicesPriority(priority, adhocShutdown)
}

func handleGracefulShutdown() {
	shutdownCh := make(chan os.Signal)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		shutdownSignal := <-shutdownCh
		app.stoppedMutex.Lock()
		app.stopped = true
		app.stoppedMutex.Unlock()
		Log().Infof("Received graceful shutdown message %s", shutdownSignal)

		app.servicesMutex.Lock()
		services := app.services[:]
		app.servicesMutex.Unlock()

		if len(services) == 0 {
			Log().Infof("No services to stop, exiting.")
			app.shutdown <- shutdownSignal
			return
		}

		priorities := []ShutdownPriority{
			ShutdownPriorityFirst,
			ShutdownPriorityNormal,
			ShutdownPriorityLast,
		}

		prioritizedServices := make([][]*gracefulServiceWrapper, 0)
		for i, p := range priorities {
			if prioritizedServices[i] == nil {
				prioritizedServices[i] = make([]*gracefulServiceWrapper, 0)
			}

			for _, s := range services {
				if s.priority == p {
					prioritizedServices[i] = append(prioritizedServices[i], s)
				}
			}
		}

		Log().Infof("Stopping a total of %d services: %s", len(services), services)

		var wg sync.WaitGroup
		for i, services := range prioritizedServices {
			Log().Infof("Stopping services %d with priority: %s", len(services), shutdownPriorityMap[priorities[i]])

			for _, s := range services {
				wg.Add(1)
				go func(s GracefulService) {
					Log().Infof("Stopping service %s", s)
					defer wg.Done()

					complete := make(chan struct{})
					go func(s GracefulService, complete chan struct{}) {
						err := s.Shutdown()
						if err != nil {
							app.log.Errorf("Error during graceful shutdown %s", err)
						}

						complete <- struct{}{}
					}(s, complete)

					select {
					case <-complete:
						app.log.Infof("Stopped service %s", s)
					case <-time.After(app.gracefulTimeout):
						app.log.Infof("The service: %s did not shutdown gracefully in the %d timeout, skiping", s, gracefulTimeout)
					}
				}(s)
			}

			wg.Wait()
		}

		app.shutdown <- shutdownSignal
	}()
}
