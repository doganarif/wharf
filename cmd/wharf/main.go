// Command wharf runs a demo wharf server with three multiplayer apps.
//
//	go run ./cmd/wharf            # listens on :2222
//	ssh -p 2222 canvas@localhost  # draw together
//	ssh -p 2222 chat@localhost    # talk
//	ssh -p 2222 poll@localhost    # live tally (stateful room)
//	ssh -p 2222 localhost         # lobby: pick an app
package main

import (
	"log"
	"os"

	"wharf"
	"wharf/apps"
)

func main() {
	addr := os.Getenv("WHARF_ADDR")
	if addr == "" {
		addr = ":2222"
	}

	err := wharf.New(addr).
		App("canvas", apps.Canvas).
		App("chat", apps.Chat).
		App("poll", apps.Poll).
		StateRoom("poll", apps.PollInitialState(), apps.PollReducer).
		Run()
	if err != nil {
		log.Fatal(err)
	}
}
