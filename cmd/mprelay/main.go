package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"mprelay/internal/server"
)

func main() {
	configPath := flag.String("config", "relay.json", "path to JSON configuration")
	flag.Parse()

	logger := log.New(os.Stdout, "mprelay ", log.LstdFlags|log.LUTC)
	config := server.DefaultConfig()
	if err := loadConfig(*configPath, &config); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Fatalf("load config: %v", err)
		}
		logger.Printf("config %s not found; using defaults", *configPath)
	}

	listener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		logger.Fatalf("listen on %s: %v", config.ListenAddress, err)
	}
	relay := server.New(config, logger)
	logger.Printf("listening on %s", listener.Addr())

	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopped
		_ = relay.Close()
	}()
	if err := relay.Serve(listener); err != nil {
		logger.Fatalf("serve: %v", err)
	}
}

func loadConfig(path string, config *server.Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, config)
}
