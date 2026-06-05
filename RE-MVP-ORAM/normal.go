package main

import "fmt"

func Normal() {
	const (
		z           = 4
		l           = 8
		n           = 256
		seed        = 542
		clientCount = 10
	)

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
