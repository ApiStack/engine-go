package main

import (
    "flag"
    "fmt"
    "os"
    "sort"

    "engine-go/binlog"
)

func main() {
    pcapPath := flag.String("pcap", "", "Input PCAP file")
    flag.Parse()

    if *pcapPath == "" {
        fmt.Println("--pcap required")
        os.Exit(1)
    }

    parser := binlog.NewBinlogParser(*pcapPath)
    if err := parser.Parse(); err != nil {
        fmt.Printf("parse failed: %v\n", err)
        os.Exit(1)
    }

    // Collect tags from header definitions
    definedTags := make(map[uint64]bool)
    for _, t := range parser.Tags {
        definedTags[t.TagID] = true
    }

    // Collect tags from actual events
    activeTags := make(map[uint32]int)
    for _, evt := range parser.Events {
        for _, inner := range evt.Inner {
            activeTags[inner.Addr]++
        }
    }

    fmt.Println("--- Tags defined in Header ---")
    var defs []uint64
    for t := range definedTags {
        defs = append(defs, t)
    }
    sort.Slice(defs, func(i, j int) bool { return defs[i] < defs[j] })
    for _, t := range defs {
        fmt.Printf("Tag: %X\n", t)
    }

    fmt.Println("\n--- Tags with Data Events ---")
    var acts []uint32
    for t := range activeTags {
        acts = append(acts, t)
    }
    sort.Slice(acts, func(i, j int) bool { return acts[i] < acts[j] })
    for _, t := range acts {
        fmt.Printf("Tag: %X (Count: %d)\n", t, activeTags[t])
    }
}
