package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"engine-go/binlog"
	"engine-go/fusion"
)

func main() {
	pcapPath := flag.String("pcap", "", "Input PCAP/binlog file")
	tagHex := flag.String("tag", "B50AC", "Tag ID in hex (e.g. B50AC)")
	outPath := flag.String("out", "fused.csv", "Output CSV path")
	allTags := flag.Bool("all", false, "Process all active tags in the pcap/binlog")
	signalLoss := flag.Float64("signal-loss-frac", 3.0, "BLE path-loss exponent")
	signalAdjust := flag.Float64("signal-adjust", 8.0, "BLE adjust A at 1m")
	deployDist := flag.Int("deploy-dist", 800, "Deployment interval cm")
	tsOffset := flag.Int64("ts-offset-ms", 0, "Timestamp offset ms to align with engine output")
	refPath := flag.String("ref", "", "Optional reference CSV for RMSE")
	maxShift := flag.Int("max-shift", 400, "Max frame shift for RMSE")
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

	tagIDs := []int{}
	if *allTags {
		tagIDs = collectActiveTags(parser)
		if len(tagIDs) == 0 {
			fmt.Println("no active tags found")
			os.Exit(1)
		}
	} else {
		tagID, err := parseTagHex(*tagHex)
		if err != nil {
			fmt.Printf("invalid tag: %v\n", err)
			os.Exit(1)
		}
		tagIDs = []int{tagID}
	}

	// load config
	baseDir := filepath.Dir(*pcapPath)
	projectXML := filepath.Join(baseDir, "project.xml")
	wogiXML := filepath.Join(baseDir, "wogi.xml")
	anchors := fusion.ParseProjectAnchors(projectXML)
	beacons := fusion.ParseProjectBeacons(projectXML)
	for id, b := range beacons {
		anchors[id] = b
	}
	// merge anchors from PCAP header blocks (positions in metres)
	for _, a := range parser.Anchors {
		if _, exists := anchors[int(a.AnchorID)]; !exists {
			anchors[int(a.AnchorID)] = fusion.Anchor{ID: int(a.AnchorID), X: a.X, Y: a.Y, Z: a.Z, Layer: 0, Building: 0}
		}
	}
	dimMap, beaconLayer, beaconDims := fusion.ParseWogiDims(wogiXML)
	for bid, lay := range beaconLayer {
		if a, ok := anchors[bid]; ok {
			a.Layer = lay
			anchors[bid] = a
		}
	}
	layerManager := fusion.LayerManagerFromConfig(projectXML, wogiXML, anchors)

	// map low16 -> full anchor id for resolving short ids in frames
	low16Map := make(map[int]int)
	for id := range anchors {
		low := id & 0xFFFF
		// only store if unique to avoid collisions
		if _, exists := low16Map[low]; !exists {
			low16Map[low] = id
		}
	}

	rssiModel := fusion.NewBLERssi(*signalLoss, *signalAdjust, *deployDist)

	windowLen := int64(1000)

	runTag := func(tagID int, out string) error {
		tagHeight := parser.GetTagHeight(uint32(tagID))
		pipeline := fusion.NewFusionPipeline(anchors, rssiModel, dimMap, beaconLayer, beaconDims, layerManager)
		rows := [][]string{{"seq", "fused_x_m", "fused_y_m"}}
		seq := 1
		pendingBle := [][2]interface{}{} // tsMs, []fusion.BLEMeas
		pendingTwr := [][2]interface{}{}

		processWindow := func(cutoff int64) bool {
			if len(pendingBle) == 0 && len(pendingTwr) == 0 {
				return false
			}
			earliest := cutoff + 1
			if len(pendingBle) > 0 && pendingBle[0][0].(int64) < earliest {
				earliest = pendingBle[0][0].(int64)
			}
			if len(pendingTwr) > 0 && pendingTwr[0][0].(int64) < earliest {
				earliest = pendingTwr[0][0].(int64)
			}
			if earliest+windowLen > cutoff {
				return false
			}
			windowEnd := earliest + windowLen
			var selBle []fusion.BLEMeas
			var selTwr []fusion.TWRMeas
			selBleTS := int64(0)
			selTwrTS := int64(0)
			for i, v := range pendingBle {
				ts := v[0].(int64)
				if ts <= windowEnd {
					selBleTS = ts
					selBle = v[1].([]fusion.BLEMeas)
					pendingBle = append(pendingBle[:i], pendingBle[i+1:]...)
					break
				}
			}
			for i, v := range pendingTwr {
				ts := v[0].(int64)
				if ts <= windowEnd {
					selTwrTS = ts
					selTwr = v[1].([]fusion.TWRMeas)
					pendingTwr = append(pendingTwr[:i], pendingTwr[i+1:]...)
					break
				}
			}
			if selBle == nil && selTwr == nil {
				// drop stale frames
				nb := pendingBle[:0]
				for _, v := range pendingBle {
					if v[0].(int64) > windowEnd {
						nb = append(nb, v)
					}
				}
				pendingBle = nb
				nt := pendingTwr[:0]
				for _, v := range pendingTwr {
					if v[0].(int64) > windowEnd {
						nt = append(nt, v)
					}
				}
				pendingTwr = nt
				return true
			}
			tsOut := selBleTS
			if selTwr != nil && (tsOut == 0 || selTwrTS < tsOut) {
				tsOut = selTwrTS
			}
			res := pipeline.Process(tsOut, tagID, selBle, selTwr, tagHeight)
			if res.Flag == 2 {
				rows = append(rows, []string{strconv.Itoa(seq), fmt.Sprintf("%.4f", res.X), fmt.Sprintf("%.4f", res.Y)})
				seq++
			}
			return true
		}

		for _, evt := range parser.Events {
			bleS, twrS, imuS := parser.FilterSamples(evt, uint32(tagID))
			// feed IMU immediately to propagate dead-reckoning
			tsMs := int64(math.Round(evt.Timestamp*1000.0)) + *tsOffset

			for _, im := range imuS {
				if im.Distance > 0 {
					pipeline.ProcessIMU(tsMs, float64(im.Distance), float64(im.YawDeg))
				}
			}

			if len(bleS) == 0 && len(twrS) == 0 {
				continue
			}
			if len(bleS) > 0 {
				lst := make([]fusion.BLEMeas, 0, len(bleS))
				for _, s := range bleS {
					aid := s.AnchorID
					if _, ok := anchors[aid]; !ok {
						if full, ok := low16Map[aid&0xFFFF]; ok {
							aid = full
						}
					}
					lst = append(lst, fusion.BLEMeas{AnchorID: aid, RSSIDb: s.RSSIDb})
				}
				pendingBle = append(pendingBle, [2]interface{}{tsMs, lst})
			}
			if len(twrS) > 0 {
				lst := make([]fusion.TWRMeas, 0, len(twrS))
				for _, s := range twrS {
					aid := s.AnchorID
					if _, ok := anchors[aid]; !ok {
						if full, ok := low16Map[aid&0xFFFF]; ok {
							aid = full
						}
					}
					lst = append(lst, fusion.TWRMeas{AnchorID: aid, Range: s.RangeM})
				}
				pendingTwr = append(pendingTwr, [2]interface{}{tsMs, lst})
			}
			for processWindow(tsMs) {
			}
		}

		if len(parser.Events) > 0 {
			lastTs := int64(math.Round(parser.Events[len(parser.Events)-1].Timestamp*1000.0)) + *tsOffset
			for processWindow(lastTs + windowLen) {
			}
		}

		if err := writeCSV(out, rows); err != nil {
			return err
		}
		fmt.Printf("Tag %X written %d rows to %s\n", tagID, len(rows)-1, out)
		return nil
	}

	for _, tagID := range tagIDs {
		out := *outPath
		if *allTags {
			ext := filepath.Ext(*outPath)
			base := strings.TrimSuffix(*outPath, ext)
			out = fmt.Sprintf("%s_%X%s", base, tagID, ext)
		}
		if err := runTag(tagID, out); err != nil {
			fmt.Printf("tag %X failed: %v\n", tagID, err)
		}
	}

	if *refPath != "" {
		rmse, shift, err := compareWithRef(*outPath, *refPath, *maxShift)
		if err != nil {
			fmt.Printf("rmse compare failed: %v\n", err)
		} else {
			fmt.Printf("ref shift %d frames, RMSE %.3f m\n", shift, rmse)
		}
	}
}

func parseTagHex(s string) (int, error) {
	s = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(s)), "0X")
	v, err := strconv.ParseInt(s, 16, 64)
	return int(v), err
}

// collectActiveTags finds tags with UWB/BLE/IMU data in the parsed events.
func collectActiveTags(p *binlog.BinlogParser) []int {
	seen := map[int]bool{}
	for _, evt := range p.Events {
		for _, in := range evt.Inner {
			switch in.Type {
			case 0x50, 0x52, 0x60, 0x61, 0x90:
				tag := int(in.Addr)
				seen[tag] = true
			}
		}
	}
	out := []int{}
	for t := range seen {
		out = append(out, t)
	}
	sort.Ints(out)
	return out
}

func writeCSV(path string, rows [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func compareWithRef(predPath, refPath string, maxShift int) (float64, int, error) {
	pred, err := readXY(predPath)
	if err != nil {
		return 0, 0, err
	}
	ref, err := readXY(refPath)
	if err != nil {
		return 0, 0, err
	}
	bestShift := 0
	bestRmse := math.MaxFloat64
	for shift := -maxShift; shift <= maxShift; shift++ {
		var n int
		var sum float64
		if shift >= 0 {
			n = min(len(pred)-shift, len(ref))
			if n <= 0 {
				continue
			}
			for i := 0; i < n; i++ {
				dx := pred[i+shift][0] - ref[i][0]
				dy := pred[i+shift][1] - ref[i][1]
				sum += dx*dx + dy*dy
			}
		} else {
			s := -shift
			n = min(len(ref)-s, len(pred))
			if n <= 0 {
				continue
			}
			for i := 0; i < n; i++ {
				dx := pred[i][0] - ref[i+s][0]
				dy := pred[i][1] - ref[i+s][1]
				sum += dx*dx + dy*dy
			}
		}
		rmse := math.Sqrt(sum / float64(n))
		if rmse < bestRmse {
			bestRmse = rmse
			bestShift = shift
		}
	}
	return bestRmse, bestShift, nil
}

func readXY(path string) ([][2]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) <= 1 {
		return nil, fmt.Errorf("no rows")
	}
	out := make([][2]float64, 0, len(recs)-1)
	// detect columns: fused_x_m/fused_y_m, fallback x_m/y_m, uwb_x_m/uwb_y_m, uwb_ble_x_m/uwb_ble_y_m
	var idxX, idxY int
	header := recs[0]
	pairs := [][2]string{
		{"fused_x_m", "fused_y_m"},
		{"x_m", "y_m"},
		{"uwb_x_m", "uwb_y_m"},
		{"uwb_ble_x_m", "uwb_ble_y_m"},
	}
	idxX, idxY = -1, -1
	for _, p := range pairs {
		ix := indexOf(header, p[0])
		iy := indexOf(header, p[1])
		if ix >= 0 && iy >= 0 {
			idxX, idxY = ix, iy
			break
		}
	}
	if idxX < 0 || idxY < 0 {
		return nil, fmt.Errorf("columns not found")
	}
	for _, row := range recs[1:] {
		if len(row) <= idxX || len(row) <= idxY {
			continue
		}
		x, _ := strconv.ParseFloat(row[idxX], 64)
		y, _ := strconv.ParseFloat(row[idxY], 64)
		out = append(out, [2]float64{x, y})
	}
	return out, nil
}

func indexOf(arr []string, key string) int {
	for i, v := range arr {
		if strings.EqualFold(v, key) {
			return i
		}
	}
	return -1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
