package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

const (
	pcapGlobalLen = 24
	pcapRecordLen = 16
	phdr2Len      = 8

	flagAnchor = 0x04
	flagTag    = 0x08
	flagStats  = 0x10
	
	// We only care about replaying data packets
)

func main() {
	pcapPath := flag.String("pcap", "", "Input PCAP file")
	destAddr := flag.String("dest", "127.0.0.1:44333", "Destination UDP address")
	speed := flag.Float64("speed", 1.0, "Replay speed multiplier (0 for max speed)")
	flag.Parse()

	if *pcapPath == "" {
		log.Fatal("--pcap required")
	}

	// Resolve destination
	raddr, err := net.ResolveUDPAddr("udp", *destAddr)
	if err != nil {
		log.Fatalf("Invalid dest address: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	f, err := os.Open(*pcapPath)
	if err != nil {
		log.Fatalf("Open pcap failed: %v", err)
	}
	defer f.Close()

	// Read Global Header
	hdr := make([]byte, pcapGlobalLen)
	if _, err := io.ReadFull(f, hdr); err != nil {
		log.Fatalf("Read global header failed: %v", err)
	}

	var firstTs float64
	var startReal time.Time
	
	count := 0
	
	log.Printf("Replaying %s to %s...", *pcapPath, *destAddr)

	bufRec := make([]byte, pcapRecordLen)
	bufPhdr2 := make([]byte, phdr2Len)

	for {
		// Read Record Header
		if _, err := io.ReadFull(f, bufRec); err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("Read record failed: %v", err)
		}

		tsSec := binary.LittleEndian.Uint32(bufRec[0:4])
		tsUsec := binary.LittleEndian.Uint32(bufRec[4:8])
		inclLen := binary.LittleEndian.Uint32(bufRec[8:12])

		if inclLen < phdr2Len {
			// Skip malformed
			f.Seek(int64(inclLen), io.SeekCurrent)
			continue
		}

		// Read PHDR2
		if _, err := io.ReadFull(f, bufPhdr2); err != nil {
			log.Fatalf("Read phdr2 failed: %v", err)
		}
		
		flag := binary.LittleEndian.Uint16(bufPhdr2[0:2])
		
		payloadLen := int(inclLen) - phdr2Len
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(f, payload); err != nil {
			log.Fatalf("Read payload failed: %v", err)
		}

		// Skip metadata blocks
		if flag == flagAnchor || flag == flagTag || flag == flagStats {
			continue
		}

		// Timing logic
		ts := float64(tsSec) + float64(tsUsec)/1e6
		if firstTs == 0 {
			firstTs = ts
			startReal = time.Now()
		} else if *speed > 0 {
			targetDelay := time.Duration((ts - firstTs) / *speed * float64(time.Second))
			elapsed := time.Since(startReal)
			if targetDelay > elapsed {
				time.Sleep(targetDelay - elapsed)
			}
		}

		// Send
		_, err = conn.Write(payload)
		if err != nil {
			log.Printf("Write error: %v", err)
		}
		count++
		if count%1000 == 0 {
			fmt.Printf("\rSent %d packets...", count)
		}
	}
	fmt.Printf("\nDone. Sent %d packets.\n", count)
}
