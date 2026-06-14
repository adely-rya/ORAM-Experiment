package main

import (
	"fmt"
	"os"
	"strconv"
)

func Normal() {
	const seed = 542
	z := 4
	l := 15
	n := 1 << 14
	clientCount := 50

	if value := os.Getenv("MVP_L"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			l = parsed
		}
	}
	if value := os.Getenv("MVP_N"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			n = parsed
		}
	}
	if value := os.Getenv("MVP_CLIENT_COUNT"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			clientCount = parsed
		}
	}

	server := NewMvpServer(z, l)
	positionmap := server.InitializeRandomData(n, seed)

	go server.Run()

	errs := make(chan error, clientCount)
	for clientID := 0; clientID < clientCount; clientID++ {
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
