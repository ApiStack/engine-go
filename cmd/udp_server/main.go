package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"engine-go/binlog"
	"engine-go/fusion"
	"engine-go/rbc"
	"engine-go/server"
	"engine-go/web"
)

func main() {
	port := flag.Int("port", 44333, "UDP port to listen on")
	httpPort := flag.Int("http", 0, "HTTP/WebSocket port (e.g. 8080). 0 to disable.")
	projectXML := flag.String("project", "project.xml", "Path to project.xml")
	wogiXML := flag.String("wogi", "wogi.xml", "Path to wogi.xml")
	signalLoss := flag.Float64("signal-loss-frac", 3.0, "BLE path-loss exponent")
	signalAdjust := flag.Float64("signal-adjust", 8.0, "BLE adjust A at 1m")
	deployDist := flag.Int("deploy-dist", 800, "Deployment interval cm")
	pcapPath := flag.String("pcap", "", "Path to output PCAP file (optional)")
	flag.Parse()

	if _, err := os.Stat(*projectXML); os.IsNotExist(err) {
		log.Fatalf("project.xml not found at %s", *projectXML)
	}
	if _, err := os.Stat(*wogiXML); os.IsNotExist(err) {
		log.Fatalf("wogi.xml not found at %s", *wogiXML)
	}

	// Load configuration
	log.Println("Loading configuration...")
	anchors := fusion.ParseProjectAnchors(*projectXML)
	beacons := fusion.ParseProjectBeacons(*projectXML)
	for id, b := range beacons {
		anchors[id] = b
	}

	dimMap, beaconLayer, beaconDims := fusion.ParseWogiDims(*wogiXML)
	for bid, lay := range beaconLayer {
		if a, ok := anchors[bid]; ok {
			a.Layer = lay
			anchors[bid] = a
		}
	}
	layerManager := fusion.LayerManagerFromConfig(*projectXML, *wogiXML, anchors)

	rssiModel := fusion.NewBLERssi(*signalLoss, *signalAdjust, *deployDist)
	pipeline := fusion.NewFusionPipeline(anchors, rssiModel, dimMap, beaconLayer, beaconDims, layerManager)

	// Initialize Server
	udpSvr, err := server.NewUdpServer(*port, pipeline)
	if err != nil {
		log.Fatalf("Failed to create UDP server: %v", err)
	}

	// Configure Web Server
	if *httpPort > 0 {
		webSvr := web.NewServer()
		configDir := filepath.Dir(*projectXML)
		// Serve static files from config directory (assuming map images are there)
		go webSvr.Start(*httpPort, configDir)
		udpSvr.SetWebHub(webSvr.Hub)
	}

	// Configure RBC
	rbcConfigs := fusion.ParseRbcSenders(*projectXML)
	if len(rbcConfigs) > 0 {
		sender := rbc.NewSender()
		for _, cfg := range rbcConfigs {
			if cfg.Type == "RBCC" || cfg.Type == "UDP" {
				fullAddr := fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port)
				if cfg.Type == "TCP" {
					sender.AddTCPSender(fullAddr, cfg.Mask)
					log.Printf("Added RBC TCP Sender: %s (mask %x)", fullAddr, cfg.Mask)
				} else {
					sender.AddUDPSender(fullAddr, cfg.Mask)
					log.Printf("Added RBC UDP Sender: %s (mask %x)", fullAddr, cfg.Mask)
				}
			}
		}
		sender.Start()
		udpSvr.SetRbcSender(sender)
		defer sender.Stop()
	}

	if *pcapPath != "" {
		// Auto-generate name if directory
		path := *pcapPath
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			path = fmt.Sprintf("%s/PKTSBIN_%s.pcap", path, time.Now().Format("20060102150405"))
		}
		
		pw, err := binlog.NewPcapWriter(path)
		if err != nil {
			log.Fatalf("Failed to create pcap writer: %v", err)
		}
		defer pw.Close()
		udpSvr.SetPcapWriter(pw)
		log.Printf("Logging packets to %s", path)
	}

	// Start Server in a goroutine
	go udpSvr.Start()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	udpSvr.Stop()
}
