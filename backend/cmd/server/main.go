package main

import (
	"log"
	"net/http"
	"os"

	"storyplatform/internal/server"
	"storyplatform/internal/story"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	store, err := story.NewStore(dataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	if _, err := store.GetDefinition("crossroads"); err == story.ErrNotFound {
		if err := store.PutDefinition(story.SeedStory()); err != nil {
			log.Fatalf("seed: %v", err)
		}
		log.Println("seeded story: crossroads v1")
	}
	h := server.New(store)
	log.Printf("story platform listening on %s (data: %s)", addr, dataDir)
	log.Fatal(http.ListenAndServe(addr, h))
}
