package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

const (
	pcapGlobalLen = 24
	pcapRecordLen = 16
	phdr2Len      = 8

	flagAnchor = 0x04
	flagTag    = 0x08
	flagStats  = 0x10
)

func main() {
	file1 := flag.String("1", "", "Original PCAP")
	file2 := flag.String("2", "", "Replayed PCAP")
	flag.Parse()

	if *file1 == "" || *file2 == "" {
		log.Fatal("Usage: verify_pcap -1 <original> -2 <replayed>")
	}

	pkts1, err := readPackets(*file1)
	if err != nil {
		log.Fatalf("Error reading %s: %v", *file1, err)
	}

	pkts2, err := readPackets(*file2)
	if err != nil {
		log.Fatalf("Error reading %s: %v", *file2, err)
	}

	fmt.Printf("Original packets (data only): %d\n", len(pkts1))
	fmt.Printf("Replayed packets (data only): %d\n", len(pkts2))

	minLen := len(pkts1)
	if len(pkts2) < minLen {
		minLen = len(pkts2)
	}

	mismatches := 0
	for i := 0; i < minLen; i++ {
		if !bytes.Equal(pkts1[i], pkts2[i]) {
			fmt.Printf("Mismatch at packet %d: len1=%d len2=%d\n", i, len(pkts1[i]), len(pkts2[i]))
			// fmt.Printf("Org: %x\n", pkts1[i])
			// fmt.Printf("Rep: %x\n", pkts2[i])
			mismatches++
			if mismatches > 10 {
				fmt.Println("Too many mismatches, stopping.")
				break
			}
		}
	}

	if len(pkts1) != len(pkts2) {
		fmt.Printf("Count mismatch: %d vs %d\n", len(pkts1), len(pkts2))
		mismatches++
	}

	if mismatches == 0 {
		fmt.Println("SUCCESS: All payloads match.")
	} else {
		fmt.Println("FAILURE: Mismatches found.")
		os.Exit(1)
	}
}

func readPackets(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read Global Header
	hdr := make([]byte, pcapGlobalLen)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return nil, err
	}

	var packets [][]byte
	bufRec := make([]byte, pcapRecordLen)
	bufPhdr2 := make([]byte, phdr2Len)

	for {
		if _, err := io.ReadFull(f, bufRec); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		inclLen := binary.LittleEndian.Uint32(bufRec[8:12])
		if inclLen < phdr2Len {
			f.Seek(int64(inclLen), io.SeekCurrent)
			continue
		}

		if _, err := io.ReadFull(f, bufPhdr2); err != nil {
			return nil, err
		}
		flag := binary.LittleEndian.Uint16(bufPhdr2[0:2])

		payloadLen := int(inclLen) - phdr2Len
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(f, payload); err != nil {
			return nil, err
		}

		if flag == flagAnchor || flag == flagTag || flag == flagStats {
			continue
		}

		packets = append(packets, payload)
	}
	return packets, nil
}
