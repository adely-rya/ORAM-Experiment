//go:build !random_distance && !latency

package main

import (
	"flag"
	"log"
)

func main() {
	f := flag.String("experiment", "random", "experiment pattern")
	flag.Parse()

	switch *f {
	case "random":
		log.Println("random access running...")
		RandomAccess()
	default:
		panic("Not setting experiment")
	}

}
