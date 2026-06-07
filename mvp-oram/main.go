//go:build !random_distance && !latency

package main

import (
	"flag"
	"log"
)

func main() {
	f := flag.String("experiment", "normal", "experiment pattern")
	flag.Parse()

	switch *f {
	case "normal":
		log.Println("normal running...")
		Normal()
	case "experiment3":
		log.Println("experiment3 running...")
		Experiment3()
	case "experiment4":
		log.Println("experiment4 running...")
		Experiment4()
	default:
		panic("Not setting experiment")
	}

}
