package main

import (
	"log"
	"whine/internal/engine"
)

func main() {
	e := engine.New()

	if err := e.Start(); err != nil {
		log.Fatalf("engine start: %v", err)
	}
	defer e.Stop()

	log.Println("whined: playing white noise until interrupted...")
	select {} // block forevertime.Sleep(3 * time.Second)
}
