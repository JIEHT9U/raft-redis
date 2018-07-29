package signal

import (
	"os"
	"os/signal"
	"syscall"
)

var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

//SetupSignalHandler sdfsdf
func SetupSignalHandler() chan os.Signal {
	stop := make(chan os.Signal, 2)
	signal.Notify(stop, shutdownSignals...)
	return stop
}
