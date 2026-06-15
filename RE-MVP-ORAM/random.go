package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
)

func RandomAccess() {
	const (
		defaultZ    = 4
		defaultL    = 12
		seed        = 542
		clientCount = 50
	)

	z := defaultZ
	if value := os.Getenv("RE_MVP_Z"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			z = parsed
		}
	}
	l := defaultL
	if value := os.Getenv("RE_MVP_L"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			l = parsed
		}
	}
	nExponentOffset := 0
	if value := os.Getenv("RE_MVP_N_EXP_OFFSET"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			nExponentOffset = parsed
		}
	}
	nExponent := l + nExponentOffset
	if nExponent < 0 {
		nExponent = 0
	}
	n := 1 << nExponent
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
			if err := client.RunRandom(n); err != nil {
				errs <- fmt.Errorf("client %d stopped: %w", client.ClientID, err)
			}
		}(client)
	}

	if err := <-errs; err != nil {
		panic(err)
	}
}

func (c *MvpClient) RunRandom(addrCount int) error {
	for {
		if err := c.Access(randomAccessOperation(addrCount)); err != nil {
			return err
		}
	}
}

func randomAccessOperation(addrCount int) OramOP {
	if addrCount <= 0 {
		panic("addrCount must be greater than 0")
	}

	target := rand.Intn(addrCount)
	operation := Read
	if rand.Intn(2) == 0 {
		operation = Write
	}

	return OramOP{
		OP:     operation,
		target: target,
		param:  fmt.Sprintf("addr-%d", target),
	}
}
