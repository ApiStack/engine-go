package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"engine-go/rbc"
)

func main() {
	udpAddr := flag.String("udp", "127.0.0.1:5555", "UDP destination")
	tcpAddr := flag.String("tcp", "127.0.0.1:6666", "TCP destination")
	header := flag.String("hdr", "AOX", "Header string")
	flag.Parse()

	sender := rbc.NewSender()
	sender.SetHeader(*header)

	if err := sender.AddUDPSender(*udpAddr, rbc.FlagPosition); err != nil {
		log.Fatalf("Failed to add UDP sender: %v", err)
	}
	sender.AddTCPSender(*tcpAddr, rbc.FlagWarning)

	if err := sender.Start(); err != nil {
		log.Fatalf("Failed to start sender: %v", err)
	}
	defer sender.Stop()

	log.Println("Sender started. Press Ctrl+C to exit.")

	i := 0
	for {
		msg := fmt.Sprintf("Message %d", i)
		// Send with Position flag (should go to UDP)
		sender.Send([]byte(msg+" POS"), rbc.FlagPosition)
		
		// Send with Warning flag (should go to TCP)
		sender.Send([]byte(msg+" WARN"), rbc.FlagWarning)

		// Send with both (should go to both)
		sender.Send([]byte(msg+" BOTH"), rbc.FlagPosition|rbc.FlagWarning)

		time.Sleep(1 * time.Second)
		i++
	}
}
