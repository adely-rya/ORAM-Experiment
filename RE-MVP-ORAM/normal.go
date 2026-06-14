package main

import (
	"fmt"
	"os"
	"strconv"
)

func Normal() {
	const (
		defaultZ    = 4
		l           = 12
		n           = 1 << 12
		seed        = 542
		clientCount = 50
	)

	z := defaultZ
	if value := os.Getenv("RE_MVP_Z"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			z = parsed
		}
	}
	measuredClientCount := clientCount
	if value := os.Getenv("RE_MVP_CLIENT_COUNT"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			measuredClientCount = parsed
		}
	}
	if os.Getenv("RE_MVP_ACCESS_LOG") == "0" {
		accessLoggingEnabled = false
	}
	ConfigureMvpMaxSignatureFromEnv()

	server := NewMvpServer(z, l)
	if os.Getenv("RE_MVP_SYNC_SERVER") == "1" {
		server = NewSynchronizedMvpServer(z, l)
	}
	positionmap := server.InitializeRandomData(n, seed)

	go server.Run()

	errs := make(chan error, measuredClientCount)
	for clientID := 0; clientID < measuredClientCount; clientID++ {
		client := NewMvpClient(
			l,
			z,
			clientID,
			clonePositionMap(positionmap),
			server.Requests,
		)

		go func(client *MvpClient) {
			if err := client.Run(n); err != nil {
				errs <- fmt.Errorf("client %d stopped: %w", client.ClientID, err)
			}
		}(client)
	}

	if err := <-errs; err != nil {
		panic(err)
	}
}
