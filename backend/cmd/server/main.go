// Command server runs the story orchestration platform backend.
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"storyplatform/internal/seed"
	"storyplatform/internal/server"
	"storyplatform/internal/store"
)

func main() {
	dataDir := os.Getenv("STORY_DATA")
	if dataDir == "" {
		dataDir = filepath.Join(".", "data")
	}
	st, err := store.NewFileStore(dataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	for _, def := range seed.All() {
		if existing, err := st.GetStory(def.ID); err == nil && existing.Version == def.Version {
			continue
		}
		if err := st.PutStory(def); err != nil {
			log.Fatalf("seed story %s/%s: %v", def.ID, def.Version, err)
		}
	}
	addr := os.Getenv("STORY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := server.New(st)
	log.Printf("story platform listening on %s (data=%s)", addr, dataDir)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
