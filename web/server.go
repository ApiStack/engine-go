package web

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type Server struct {
	Hub *Hub
}

func NewServer() *Server {
	return &Server{
		Hub: NewHub(),
	}
}

func (s *Server) Start(port int, distDir string, configDir string) {
	go s.Hub.Run()

	mux := http.NewServeMux()

	// WebSocket
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.Hub, w, r)
	})

	// Config Files
	if configDir != "" {
		// Serve specific files
		mux.HandleFunc("/project.xml", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(configDir, "project.xml"))
		})
		mux.HandleFunc("/wogi.xml", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(configDir, "wogi.xml"))
		})
		// Serve Map directory
		mapDir := filepath.Join(configDir, "Map")
		if _, err := os.Stat(mapDir); err == nil {
			mux.Handle("/Map/", http.StripPrefix("/Map/", http.FileServer(http.Dir(mapDir))))
		}
	}

	// Static Frontend
	if distDir != "" {
		fs := http.FileServer(http.Dir(distDir))
		mux.Handle("/", fs)
	}

	addr := fmt.Sprintf(":%d", port)
	log.Printf("HTTP Server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}