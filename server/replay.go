package server

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"engine-go/fusion"
)

const (
	pcapGlobalLen = 24
	pcapRecordLen = 16
	phdr2Len      = 8

	flagAnchor = 0x04
	flagTag    = 0x08
	flagStats  = 0x10
)

func (s *UdpServer) parseAnchorBlock(payload []byte, itemnum int, itemsize int) {
	for i := 0; i < itemnum; i++ {
		start := i * itemsize
		end := start + itemsize
		if end > len(payload) {
			return
		}
		chunk := payload[start:end]
		anchorID := binary.LittleEndian.Uint64(chunk[0:8])
		x := int32(binary.LittleEndian.Uint32(chunk[8:12]))
		y := int32(binary.LittleEndian.Uint32(chunk[12:16]))
		z := int32(binary.LittleEndian.Uint32(chunk[16:20]))
		region := binary.LittleEndian.Uint16(chunk[20:22])

		// Truncate to Short ID to match map keys and server logic
		shortID := int(anchorID & 0xFFFF)

		a := fusion.Anchor{
			ID:       shortID,
			X:        float64(x) / 100.0,
			Y:        float64(y) / 100.0,
			Z:        float64(z) / 100.0,
			Layer:    int(region),
			Building: 0,
		}
		s.addAnchorGlobal(a)
	}
}

func (s *UdpServer) Replay(path string, speed float64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Read Global Header
	hdr := make([]byte, pcapGlobalLen)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return fmt.Errorf("read global header: %w", err)
	}

	s.running = true
	log.Printf("Replaying %s at %.1fx speed...", path, speed)

	bufRec := make([]byte, pcapRecordLen)
	bufPhdr2 := make([]byte, phdr2Len)

	var firstTs float64
	var startReal time.Time

	// Initialize real-time start
	startReal = time.Now()

	pktCount := 0

	for s.running {
		// Read Record Header
		if _, err := io.ReadFull(f, bufRec); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read record: %w", err)
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
			return fmt.Errorf("read phdr2: %w", err)
		}

		flag := binary.LittleEndian.Uint16(bufPhdr2[0:2])
		port := binary.LittleEndian.Uint16(bufPhdr2[2:4])
		ipBytes := bufPhdr2[4:8]

		payloadLen := int(inclLen) - phdr2Len
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(f, payload); err != nil {
			return fmt.Errorf("read payload: %w", err)
		}

		// Process metadata blocks
		if flag == flagAnchor {
			s.parseAnchorBlock(payload, int(port), int(binary.LittleEndian.Uint32(ipBytes)))
			continue
		}
		if flag == flagTag || flag == flagStats {
			continue
		}

		pktCount++
		if pktCount <= 10 {
			log.Printf("Replay Pkt #%d: TS=%.3f Len=%d Flag=%x IP=%d.%d.%d.%d:%d",
				pktCount, float64(tsSec)+float64(tsUsec)/1e6, payloadLen, flag,
				ipBytes[0], ipBytes[1], ipBytes[2], ipBytes[3], port)
		}

		// Timing logic
		ts := float64(tsSec) + float64(tsUsec)/1e6
		if firstTs == 0 {
			firstTs = ts
			startReal = time.Now() // Reset start time to now
		} else if speed > 0 {
			targetDelay := time.Duration((ts - firstTs) / speed * float64(time.Second))
			elapsed := time.Since(startReal)
			if targetDelay > elapsed {
				time.Sleep(targetDelay - elapsed)
			}
		}

		// Construct simulated address
		addr := &net.UDPAddr{
			IP:   net.IP(ipBytes),
			Port: int(port),
		}

		// Feed to pipeline
		s.handlePacket(payload, addr, int64(ts*1000))

		if pktCount%1000 == 0 {
			// log.Printf("Processed %d packets", pktCount)
		}
	}
	log.Printf("Replay loop ended. Total Packets: %d", pktCount)
	return nil
}
