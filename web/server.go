package web

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type DownlinkHandler interface {
	SendConfig(tagID int, cmdID int, data []byte) error
}

type Server struct {
	Hub             *Hub
	DownlinkHandler DownlinkHandler
}

func NewServer() *Server {
	return &Server{
		Hub: NewHub(),
	}
}

func (s *Server) SetDownlinkHandler(h DownlinkHandler) {
	s.DownlinkHandler = h
}

func (s *Server) Start(port int, distDir string, configDir string) {
	go s.Hub.Run()

	mux := http.NewServeMux()

	// WebSocket
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.Hub, w, r)
	})

	// API
	mux.HandleFunc("/api/lora/config", s.handleLoraConfig)

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

type ConfigRequest struct {
	TagID   int    `json:"tag_id"`
	CmdID   int    `json:"cmd_id"`
	DataHex string `json:"data_hex"` // Hex encoded data
}

func (s *Server) handleLoraConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.DownlinkHandler == nil {
		http.Error(w, "Downlink handler not configured", http.StatusServiceUnavailable)
		return
	}

	var req ConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	data, err := hex.DecodeString(req.DataHex)
	if err != nil {
		http.Error(w, "Invalid DataHex", http.StatusBadRequest)
		return
	}

	if err := s.DownlinkHandler.SendConfig(req.TagID, req.CmdID, data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to send config: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
