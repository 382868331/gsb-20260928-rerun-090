// Command server runs the story orchestration platform API and serves the
// built frontend when available.
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"storyplatform/internal/api"
	"storyplatform/internal/seed"
	"storyplatform/internal/story"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = filepath.Join("..", "frontend", "dist")
	}

	store, err := story.NewFileStore(dataDir)
	if err != nil {
		log.Fatalf("open data dir: %v", err)
	}
	revDir := os.Getenv("REVISION_DIR")
	if revDir == "" {
		revDir = filepath.Join(dataDir, "revisions")
	}
	revisions, err := story.NewFileRevisionStore(revDir)
	if err != nil {
		log.Fatalf("open revision dir: %v", err)
	}
	// Restored snapshots (including revisions adopted by existing saves)
	// rebuild the catalog history before the built-in seed is registered; the
	// seed never downgrades a newer restored revision.
	catalog := story.NewCatalogWithStore(revisions, seed.Story())
	h := api.NewHandler(catalog, store, seed.InitialFacts())
	mux := http.NewServeMux()
	h.Register(mux)

	if _, err := os.Stat(filepath.Join(staticDir, "index.html")); err == nil {
		fs := http.FileServer(http.Dir(staticDir))
		mux.Handle("GET /", spaHandler{staticDir: staticDir, h: fs})
		log.Printf("serving frontend from %s", staticDir)
	}

	log.Printf("story platform listening on %s", addr)
	if err := http.ListenAndServe(addr, logRequests(mux)); err != nil {
		log.Fatal(err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

// spaHandler falls back to index.html for client-side routes.
type spaHandler struct {
	staticDir string
	h         http.Handler
}

func (s spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := filepath.Join(s.staticDir, filepath.Clean(r.URL.Path))
	if info, err := os.Stat(p); err != nil || info.IsDir() {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		s.h.ServeHTTP(w, r2)
		return
	}
	s.h.ServeHTTP(w, r)
}
