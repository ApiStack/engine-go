package fusion

import (
    "encoding/xml"
    "io"
    "strconv"
    "strings"
)

type RbcSenderConfig struct {
	Addr string
	Port int
	Type string
	Mask uint32
}

// ParseRbcSenders parses rbc senders from project.xml.
func ParseRbcSenders(path string) []RbcSenderConfig {
	configs := []RbcSenderConfig{}
	dec, f, err := readXML(path)
	if err != nil {
		return configs
	}
	defer f.Close()
	inTxList := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "txlist" {
				inTxList = true
				continue
			}
			if t.Name.Local == "transferItem" && inTxList {
				addr, _ := attrValue(t, "addr")
				portStr, _ := attrValue(t, "port")
				typ, _ := attrValue(t, "type")
				maskStr, _ := attrValue(t, "data")

				port, _ := strconv.Atoi(portStr)
				mask, _ := strconv.ParseInt(maskStr, 10, 64) // Parsing as decimal based on example "50331649"

				configs = append(configs, RbcSenderConfig{
					Addr: addr,
					Port: port,
					Type: typ,
					Mask: uint32(mask),
				})
			}
		case xml.EndElement:
			if t.Name.Local == "txlist" {
				inTxList = false
			}
		}
	}
	return configs
}

// ParseProjectAnchors loads anchorlist from project.xml into Anchor map keyed by id.
func ParseProjectAnchors(path string) map[int]Anchor {
    anchors := map[int]Anchor{}
    dec, f, err := readXML(path)
    if err != nil {
        return anchors
    }
    defer f.Close()
    inAnchorList := false
    for {
        tok, err := dec.Token()
        if err == io.EOF {
            break
        }
        if err != nil {
            break
        }
        switch t := tok.(type) {
        case xml.StartElement:
            if t.Name.Local == "anchorlist" {
                inAnchorList = true
                continue
            }
            if t.Name.Local == "deviceItem" && inAnchorList {
                cls, _ := attrValue(t, "class")
                layer, _ := display2groupID(cls)
                idStr, ok := attrValue(t, "id")
                if !ok {
                    continue
                }
                posStr, ok := attrValue(t, "pos")
                if !ok {
                    continue
                }
                aid, err := strconv.ParseInt(idStr, 16, 64)
                if err != nil {
                    continue
                }
                coords := strings.Split(posStr, ",")
                if len(coords) < 3 {
                    continue
                }
                x, err1 := strconv.ParseFloat(coords[0], 64)
                y, err2 := strconv.ParseFloat(coords[1], 64)
                z, err3 := strconv.ParseFloat(coords[2], 64)
                if err1 != nil || err2 != nil || err3 != nil {
                    continue
                }
                shortID := int(aid & 0xFFFF)
                anchors[shortID] = Anchor{ID: shortID, X: x / 100.0, Y: y / 100.0, Z: z / 100.0, Layer: layer, Building: 0}
            }
        case xml.EndElement:
            if t.Name.Local == "anchorlist" {
                inAnchorList = false
            }
        }
    }
    return anchors
}

// ParseProjectBeacons returns beacons (BLE) as anchors.
func ParseProjectBeacons(path string) map[int]Anchor {
    beacons := map[int]Anchor{}
    dec, f, err := readXML(path)
    if err != nil {
        return beacons
    }
    defer f.Close()
    inBeaconList := false
    for {
        tok, err := dec.Token()
        if err == io.EOF {
            break
        }
        if err != nil {
            break
        }
        switch t := tok.(type) {
        case xml.StartElement:
            if t.Name.Local == "beaconlist" {
                inBeaconList = true
                continue
            }
            if t.Name.Local == "deviceItem" && inBeaconList {
                cls, _ := attrValue(t, "class")
                layer := display2layer(cls)
                idStr, ok := attrValue(t, "id")
                if !ok {
                    continue
                }
                posStr, ok := attrValue(t, "pos")
                if !ok {
                    continue
                }
                bid, err := strconv.ParseInt(idStr, 16, 64)
                if err != nil {
                    continue
                }
                coords := strings.Split(posStr, ",")
                if len(coords) < 3 {
                    continue
                }
                x, err1 := strconv.ParseFloat(coords[0], 64)
                y, err2 := strconv.ParseFloat(coords[1], 64)
                z, err3 := strconv.ParseFloat(coords[2], 64)
                if err1 != nil || err2 != nil || err3 != nil {
                    continue
                }
                shortID := int(bid & 0xFFFF)
                beacons[shortID] = Anchor{ID: shortID, X: x / 100.0, Y: y / 100.0, Z: z / 100.0, Layer: layer, Building: 0}
            }
        case xml.EndElement:
            if t.Name.Local == "beaconlist" {
                inBeaconList = false
            }
        }
    }
    return beacons
}

func display2groupID(cls string) (int, int) {
    if !strings.Contains(cls, ":") {
        return 0, 0
    }
    parts := strings.SplitN(cls, ":", 2)
    rid, err := strconv.Atoi(parts[0])
    if err != nil {
        return 0, 0
    }
    rest := strings.Split(parts[1], ",")
    gid0 := 0
    if len(rest) > 0 && rest[0] != "" {
        if v, err := strconv.Atoi(rest[0]); err == nil {
            gid0 = v
        }
    }
    gidCombined := (rid << 16) | (gid0 & 0xFFFF)
    return rid, gidCombined
}

func display2layer(cls string) int {
    if !strings.Contains(cls, ":") {
        return 0
    }
    parts := strings.SplitN(cls, ":", 2)
    rid, err := strconv.Atoi(parts[0])
    if err != nil {
        return 0
    }
    return rid
}

// ParseWogiDims parses wogi.xml into dim map and beacon dim mappings.
func ParseWogiDims(path string) (map[int][]DimMat, map[int]int, map[int][]DimMat) {
    dimMap := map[int][]DimMat{}
    beaconLayer := map[int]int{}
    beaconDims := map[int][]DimMat{}
    dec, f, err := readXML(path)
    if err != nil {
        return dimMap, beaconLayer, beaconDims
    }
    defer f.Close()

    addMat := func(layer int, mat DimMat) {
        dimMap[layer] = append(dimMap[layer], mat)
    }

    for {
        tok, err := dec.Token()
        if err == io.EOF {
            break
        }
        if err != nil {
            break
        }
        start, ok := tok.(xml.StartElement)
        if !ok || (start.Name.Local != "zone" && start.Name.Local != "cell") {
            continue
        }
        layer, _ := parseIntAttr(start, "layer")
        dimAttr, _ := parseIntAttr(start, "dimens")
        posgroup, ok := attrValue(start, "posgroup")
        if !ok {
            continue
        }
        pts := parsePoints(posgroup)
        if len(pts) == 0 {
            continue
        }
        mats := []DimMat{}
        if dimAttr == 0 || len(pts) == 1 {
            c := meanPoint(pts)
            mats = append(mats, DimMat{{c[0] / 100.0, c[1] / 100.0, c[2] / 100.0}})
        } else if dimAttr == 1 {
            for i := 0; i < len(pts)-1; i++ {
                mats = append(mats, DimMat{{pts[i][0] / 100.0, pts[i][1] / 100.0, 0}, {pts[i+1][0] / 100.0, pts[i+1][1] / 100.0, 0}})
            }
        } else {
            c := meanPoint(pts)
            mats = append(mats, DimMat{{c[0] / 100.0, c[1] / 100.0, c[2] / 100.0}})
        }
        for _, m := range mats {
            addMat(layer, m)
        }
        if start.Name.Local == "zone" {
            if bStr, okb := attrValue(start, "beacons"); okb {
                bids := strings.Split(bStr, ",")
                for _, s := range bids {
                    s = strings.TrimSpace(s)
                    if s == "" {
                        continue
                    }
                    bid, err := strconv.ParseInt(s, 16, 64)
                    if err != nil {
                        continue
                    }
                    beaconLayer[int(bid)] = layer
                    for _, m := range mats {
                        beaconDims[int(bid)] = append(beaconDims[int(bid)], m)
                    }
                }
            }
        }
    }
    return dimMap, beaconLayer, beaconDims
}

func meanPoint(pts [][2]float64) [3]float64 {
    var sx, sy float64
    for _, p := range pts {
        sx += p[0]
        sy += p[1]
    }
    n := float64(len(pts))
    return [3]float64{sx / n, sy / n, 0}
}
