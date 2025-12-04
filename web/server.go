package web

import (
	"fmt"
	"log"
	"net/http"
)

type Server struct {
	Hub *Hub
}

func NewServer() *Server {
	return &Server{
		Hub: NewHub(),
	}
}

func (s *Server) Start(port int, staticDir string) {
	go s.Hub.Run()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.Hub, w, r)
	})

	if staticDir != "" {
		fs := http.FileServer(http.Dir(staticDir))
		http.Handle("/", fs)
	}

	addr := fmt.Sprintf(":%d", port)
	log.Printf("HTTP Server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}
