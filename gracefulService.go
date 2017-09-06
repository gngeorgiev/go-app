package app

import (
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
