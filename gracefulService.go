package app

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	gracefulTimeout = time.Minute * 4
)

//GracefulService knows how to shut itself down, synchronously
type GracefulService interface {
	Shutdown() error
	String() string
}

//RegisterServices registers GracefulServices to the app, then each service is stopped,
//when the app is stopped
func RegisterServices(services ...GracefulService) {
	app.servicesMutex.Lock()
	defer app.servicesMutex.Unlock()

	app.services = append(app.services, services...)
}

func handleGracefulShutdown() {
	shutdownCh := make(chan os.Signal)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		shutdownSignal := <-shutdownCh
		app.stopped = true
		Log().Infof("Received graceful shutdown message %s", shutdownSignal)

		if len(app.services) == 0 {
			Log().Infof("No services to stop, exiting.")
			app.shutdown <- shutdownSignal
			return
		}

		Log().Infof("Stopping a total of %d services: %s", len(app.services), app.services)

		var wg sync.WaitGroup
		for _, s := range app.services {
			wg.Add(1)
			go func(s GracefulService) {
				app.log.Infof("Stopping service %s", s)
				defer wg.Done()

				complete := make(chan struct{})
				go func(s GracefulService, complete chan struct{}) {
					err := s.Shutdown()
					if err != nil {
						app.log.Errorf("Error during graceful shutdown %s", err)
					}

					complete <- struct{}{}
				}(s, complete)

				t := time.NewTimer(gracefulTimeout)

				select {
				case <-complete:
					app.log.Infof("Stopped service %s", s)
				case <-t.C:
					app.log.Infof("A service did not shutdown gracefully in the %s timeout, skiping", gracefulTimeout)
				}
			}(s)
		}

		wg.Wait()

		app.shutdown <- shutdownSignal
	}()
}
