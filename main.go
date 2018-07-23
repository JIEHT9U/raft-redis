package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	i "github.com/JIEHT9U/raft-redis/init"
	"github.com/JIEHT9U/raft-redis/logger"
	"github.com/JIEHT9U/raft-redis/server"
	"github.com/JIEHT9U/raft-redis/signal"

	"github.com/joho/godotenv"
)

var (
	// Version of alertmanager-bot.
	Version = "dev"
	// Revision or Commit this binary was built from.
	Revision string
	// BuildDate this binary was built.
	BuildDate string
	// GoVersion running this binary.
	GoVersion = runtime.Version()
	// StartTime has the time this was started.
	StartTime = time.Now()
)

func main() {
	godotenv.Load()

	logging, err := logger.Init(Version == "dev")
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	defer logging.Sync()

	initParams, err := i.Param()
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	server := server.New(initParams, logging.Sugar(), signal.SetupSignalHandler())

	if err := server.Run(); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}
