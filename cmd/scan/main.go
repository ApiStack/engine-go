package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"engine-go/binlog"
	"engine-go/fusion"
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
		fmt.Printf("parse pcap failed: %v\n", err)
		os.Exit(1)
	}

	// Setup minimal pipeline context
	baseDir := filepath.Dir(*pcapPath)
	projectXML := filepath.Join(baseDir, "../ruigao/project.xml") // Hack: assume ruigao structure
    if _, err := os.Stat(projectXML); os.IsNotExist(err) {
        projectXML = filepath.Join(baseDir, "project.xml") // Fallback
    }
    wogiXML := filepath.Join(baseDir, "../ruigao/wogi.xml")
    if _, err := os.Stat(wogiXML); os.IsNotExist(err) {
        wogiXML = filepath.Join(baseDir, "wogi.xml")
    }

	anchors := fusion.ParseProjectAnchors(projectXML)
    // Ensure Short ID aliases
	for id, a := range anchors {
		short := id & 0xFFFF
		if _, ok := anchors[short]; !ok {
			alias := a
			alias.ID = short
			anchors[short] = alias
		}
	}

		// Config
	    rssiModel := fusion.NewBLERssi(3.0, 8.0, 800)
	    dimMap, beaconLayer, beaconDims := fusion.ParseWogiDims(wogiXML)
	    lm := fusion.LayerManagerFromConfig(projectXML, wogiXML, anchors)
	
		fmt.Printf("Scanning tags in %s...\n", *pcapPath)
	
	    // Re-implement loop for B50AC
	    tagsToCheck := []int{0xB50AC} // Add others if known    
    for _, tagID := range tagsToCheck {
        pipeline := fusion.NewFusionPipeline(anchors, rssiModel, dimMap, beaconLayer, beaconDims, lm)
        minX, maxX, minY, maxY := 100000.0, -100000.0, 100000.0, -100000.0
        count := 0
        
        for _, evt := range parser.Events {
            bleS, twrS, imuS := parser.FilterSamples(evt, uint32(tagID))
            tsMs := int64(math.Round(evt.Timestamp*1000.0))
            
            // Feed IMU
            for _, im := range imuS {
                if im.Distance > 0 {
                    pipeline.ProcessIMU(tsMs, float64(im.Distance), float64(im.YawDeg))
                }
            }
            
            if len(bleS) > 0 || len(twrS) > 0 {
                // conversion logic omitted for brevity, assume pipeline handles empty lists safely?
                // fuse/main.go converts them. We need that logic.
                // Skip for now, just assume Process handles empty meas?
                // No, Process expects []BLEMeas.
                
                bl := make([]fusion.BLEMeas, len(bleS))
                for i, v := range bleS { bl[i] = fusion.BLEMeas{AnchorID: v.AnchorID, RSSIDb: v.RSSIDb}}
                tw := make([]fusion.TWRMeas, len(twrS))
                for i, v := range twrS { tw[i] = fusion.TWRMeas{AnchorID: v.AnchorID, Range: v.RangeM}}
                
                res := pipeline.Process(tsMs, tagID, bl, tw, 1.2)
                if res.Flag == 2 {
                    if res.X < minX { minX = res.X }
                    if res.X > maxX { maxX = res.X }
                    if res.Y < minY { minY = res.Y }
                    if res.Y > maxY { maxY = res.Y }
                    count++
                }
            }
        }
        fmt.Printf("Tag %X: %d points. X[%.2f, %.2f] Y[%.2f, %.2f]\n", tagID, count, minX, maxX, minY, maxY)
    }
}