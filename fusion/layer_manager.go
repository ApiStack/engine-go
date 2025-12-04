package fusion

import (
    "encoding/xml"
    "io"
    "math"
    "os"
    "strconv"
    "strings"
)

// OUTDOOR layer id is defined in constants.go

type Region struct {
    XTL float64
    YTL float64
    XBR float64
    YBR float64
}

type Layer struct {
    ID         int
    Building   int
    Width      float64
    Height     float64
    XTL        float64
    YTL        float64
    XBR        float64
    YBR        float64
    ProjectIdx int
    Regions    []Region
}

type Project struct {
    ID       int
    Building int
    XTL      float64
    YTL      float64
    XBR      float64
    YBR      float64
    Regions  []*Layer
}

type LayerManager struct {
    layers   map[int]*Layer
    projects []*Project
}

// NewLayerManager builds from parsed layers and projects.
func NewLayerManager(layers map[int]*Layer, projects []*Project) *LayerManager {
    return &LayerManager{layers: layers, projects: projects}
}

func readXML(path string) (*xml.Decoder, *os.File, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, nil, err
    }
    dec := xml.NewDecoder(f)
    return dec, f, nil
}

func attrValue(start xml.StartElement, name string) (string, bool) {
    for _, a := range start.Attr {
        if a.Name.Local == name {
            return a.Value, true
        }
    }
    return "", false
}

func parseFloatAttr(start xml.StartElement, name string) (float64, bool) {
    if v, ok := attrValue(start, name); ok {
        val, err := strconv.ParseFloat(v, 64)
        if err == nil {
            return val, true
        }
    }
    return 0, false
}

func parseIntAttr(start xml.StartElement, name string) (int, bool) {
    if v, ok := attrValue(start, name); ok {
        val, err := strconv.Atoi(v)
        if err == nil {
            return val, true
        }
    }
    return 0, false
}

// Parse project.xml maplist mapItem entries.
func parseProjectMaps(path string) map[int]*Layer {
    layers := map[int]*Layer{}
    dec, f, err := readXML(path)
    if err != nil {
        return layers
    }
    defer f.Close()
    for {
        tok, err := dec.Token()
        if err == io.EOF {
            break
        }
        if err != nil {
            break
        }
        start, ok := tok.(xml.StartElement)
        if !ok || start.Name.Local != "mapItem" {
            continue
        }
        grp, ok := parseIntAttr(start, "group")
        if !ok {
            continue
        }
        building, _ := parseIntAttr(start, "building")
        xTL, _ := parseFloatAttr(start, "x-topleft")
        yTL, _ := parseFloatAttr(start, "y-topleft")
        width, _ := parseFloatAttr(start, "width")
        height, _ := parseFloatAttr(start, "height")
        xBR := xTL + width
        yBR := yTL + height
        lyr, exists := layers[grp]
        if !exists {
            lyr = &Layer{ID: grp, Building: building, XTL: xTL, YTL: yTL, XBR: xBR, YBR: yBR, Width: width, Height: height}
        } else {
            if lyr.Width == 0 {
                lyr.XTL, lyr.YTL, lyr.XBR, lyr.YBR = xTL, yTL, xBR, yBR
            } else {
                if xTL < lyr.XTL {
                    lyr.XTL = xTL
                }
                if yTL < lyr.YTL {
                    lyr.YTL = yTL
                }
                if xBR > lyr.XBR {
                    lyr.XBR = xBR
                }
                if yBR > lyr.YBR {
                    lyr.YBR = yBR
                }
            }
            if width > lyr.Width {
                lyr.Width = width
            }
            if height > lyr.Height {
                lyr.Height = height
            }
            if lyr.Building == 0 {
                lyr.Building = building
            }
        }
        layers[grp] = lyr
    }
    return layers
}

// Parse <regions><region> polygons to bounding boxes per layer.
func parseProjectRegions(path string, layers map[int]*Layer) {
    dec, f, err := readXML(path)
    if err != nil {
        return
    }
    defer f.Close()
    for {
        tok, err := dec.Token()
        if err == io.EOF {
            break
        }
        if err != nil {
            break
        }
        start, ok := tok.(xml.StartElement)
        if !ok || start.Name.Local != "region" {
            continue
        }
        layerID, ok := parseIntAttr(start, "layer")
        if !ok {
            continue
        }
        val, ok := attrValue(start, "value")
        if !ok {
            continue
        }
        pts := parsePoints(val)
        if len(pts) == 0 {
            continue
        }
        xs := []float64{}
        ys := []float64{}
        for _, p := range pts {
            xs = append(xs, p[0])
            ys = append(ys, p[1])
        }
        reg := Region{XTL: minSlice(xs), YTL: minSlice(ys), XBR: maxSlice(xs), YBR: maxSlice(ys)}
        lyr, ok := layers[layerID]
        if !ok {
            lyr = &Layer{ID: layerID}
        }
        lyr.Regions = append(lyr.Regions, reg)
        layers[layerID] = lyr
    }
}

func parseWogiZones(path string, layers map[int]*Layer) {
    dec, f, err := readXML(path)
    if err != nil {
        return
    }
    defer f.Close()
    for {
        tok, err := dec.Token()
        if err == io.EOF {
            break
        }
        if err != nil {
            break
        }
        start, ok := tok.(xml.StartElement)
        if !ok || start.Name.Local != "zone" {
            continue
        }
        layerID, ok := parseIntAttr(start, "layer")
        if !ok {
            continue
        }
        posgroup, ok := attrValue(start, "posgroup")
        if !ok || posgroup == "" {
            continue
        }
        pts := parsePoints(posgroup)
        if len(pts) == 0 {
            continue
        }
        xs := []float64{}
        ys := []float64{}
        for _, p := range pts {
            xs = append(xs, p[0])
            ys = append(ys, p[1])
        }
        reg := Region{XTL: minSlice(xs), YTL: minSlice(ys), XBR: maxSlice(xs), YBR: maxSlice(ys)}
        lyr, ok := layers[layerID]
        if !ok {
            lyr = &Layer{ID: layerID}
        }
        lyr.Regions = append(lyr.Regions, reg)
        layers[layerID] = lyr
    }
}

func fillFromAnchors(layers map[int]*Layer, anchors map[int]Anchor) {
    byLayer := map[int][]Anchor{}
    for _, a := range anchors {
        byLayer[a.Layer] = append(byLayer[a.Layer], a)
    }
    for lid, lst := range byLayer {
        lyr, ok := layers[lid]
        if !ok {
            lyr = &Layer{ID: lid}
        }
        if len(lst) > 0 {
            xs := []float64{}
            ys := []float64{}
            for _, a := range lst {
                xs = append(xs, a.X*100.0)
                ys = append(ys, a.Y*100.0)
            }
            if lyr.Width == 0 || lyr.Height == 0 {
                lyr.XTL = minSlice(xs)
                lyr.YTL = minSlice(ys)
                lyr.XBR = maxSlice(xs)
                lyr.YBR = maxSlice(ys)
            } else {
                lyr.XTL = math.Min(lyr.XTL, minSlice(xs))
                lyr.YTL = math.Min(lyr.YTL, minSlice(ys))
                lyr.XBR = math.Max(lyr.XBR, maxSlice(xs))
                lyr.YBR = math.Max(lyr.YBR, maxSlice(ys))
            }
            lyr.Width = math.Max(lyr.XBR-lyr.XTL, lyr.Width)
            lyr.Height = math.Max(lyr.YBR-lyr.YTL, lyr.Height)
        }
        layers[lid] = lyr
    }
}

func ensureRegions(layers map[int]*Layer) {
    for _, lyr := range layers {
        if lyr.Width == 0 || lyr.Height == 0 {
            continue
        }
        if len(lyr.Regions) == 0 {
            lyr.Regions = append(lyr.Regions, Region{XTL: lyr.XTL, YTL: lyr.YTL, XBR: lyr.XBR, YBR: lyr.YBR})
        }
    }
}

func buildProjects(layers map[int]*Layer) []*Project {
    byBuilding := map[int][]*Layer{}
    for _, lyr := range layers {
        byBuilding[lyr.Building] = append(byBuilding[lyr.Building], lyr)
    }
    projects := []*Project{}
    for bld, lst := range byBuilding {
        if len(lst) == 0 {
            continue
        }
        xsTL := []float64{}
        ysTL := []float64{}
        xsBR := []float64{}
        ysBR := []float64{}
        for _, l := range lst {
            xsTL = append(xsTL, l.XTL)
            ysTL = append(ysTL, l.YTL)
            xsBR = append(xsBR, l.XBR)
            ysBR = append(ysBR, l.YBR)
        }
        proj := &Project{
            ID:       len(projects) + 1,
            Building: bld,
            XTL:      minSlice(xsTL),
            YTL:      minSlice(ysTL),
            XBR:      maxSlice(xsBR),
            YBR:      maxSlice(ysBR),
            Regions:  lst,
        }
        idx := len(projects)
        for _, l := range lst {
            l.ProjectIdx = idx
        }
        projects = append(projects, proj)
    }
    return projects
}

// FromConfig builds LayerManager using project.xml, wogi.xml and anchors.
func LayerManagerFromConfig(projectPath, wogiPath string, anchors map[int]Anchor) *LayerManager {
    layers := parseProjectMaps(projectPath)
    parseProjectRegions(projectPath, layers)
    parseWogiZones(wogiPath, layers)
    fillFromAnchors(layers, anchors)
    ensureRegions(layers)
    projects := buildProjects(layers)
    return NewLayerManager(layers, projects)
}

// Helper parsing utils ----------------------------------------------------

func parsePoints(val string) [][2]float64 {
    pts := [][2]float64{}
    parts := strings.Split(val, ";")
    for _, p := range parts {
        if p == "" {
            continue
        }
        toks := strings.Split(p, ",")
        if len(toks) < 2 {
            continue
        }
        x, err1 := strconv.ParseFloat(strings.TrimSpace(toks[0]), 64)
        y, err2 := strconv.ParseFloat(strings.TrimSpace(toks[1]), 64)
        if err1 == nil && err2 == nil {
            pts = append(pts, [2]float64{x, y})
        }
    }
    return pts
}

func minSlice(a []float64) float64 {
    if len(a) == 0 {
        return 0
    }
    m := a[0]
    for _, v := range a[1:] {
        if v < m {
            m = v
        }
    }
    return m
}

func maxSlice(a []float64) float64 {
    if len(a) == 0 {
        return 0
    }
    m := a[0]
    for _, v := range a[1:] {
        if v > m {
            m = v
        }
    }
    return m
}

func isInProject(pos [3]float64, proj *Project) bool {
    x := pos[0] * 100.0
    y := pos[1] * 100.0
    return x >= proj.XTL && x <= proj.XBR && y >= proj.YTL && y <= proj.YBR
}

func isInLayer(pos [3]float64, layer *Layer) bool {
    if layer == nil {
        return false
    }
    x := pos[0] * 100.0
    y := pos[1] * 100.0
    if !(x >= layer.XTL && x <= layer.XBR && y >= layer.YTL && y <= layer.YBR) {
        return false
    }
    for _, r := range layer.Regions {
        if x >= r.XTL && x <= r.XBR && y >= r.YTL && y <= r.YBR {
            return true
        }
    }
    return false
}

func layerTrustRate(bleMeas []BLEMeas, twrMeas []TWRMeas, pos [3]float64, layerID int, rssi *BLERssi, anchors map[int]Anchor) float64 {
    if len(bleMeas) == 0 && len(twrMeas) == 0 {
        return 0xFF
    }
    n := 0
    rates := 0.0
    cmPos := [3]float64{pos[0] * 100.0, pos[1] * 100.0, pos[2] * 100.0}
    for _, m := range twrMeas {
        a, ok := anchors[m.AnchorID]
        if !ok || a.Layer != layerID {
            continue
        }
        distance := math.Hypot(cmPos[0]-a.X*100.0, cmPos[1]-a.Y*100.0)
        if distance < 1e-3 {
            continue
        }
        rngCm := m.Range * 100.0
        rates += 1.0 * rngCm / distance
        n++
    }
    for _, m := range bleMeas {
        a, ok := anchors[m.AnchorID]
        if !ok || a.Layer != layerID {
            continue
        }
        distance := math.Hypot(cmPos[0]-a.X*100.0, cmPos[1]-a.Y*100.0)
        if distance < 1e-3 {
            continue
        }
        strength := rssi.StrengthFromDbm(m.RSSIDb)
        dRangeCm := rssi.Rssi2Range(strength)
        dDataM := 0.01 * float64(dRangeCm)
        rates += 100.0 * dDataM / distance
        n++
    }
    if n > 0 {
        return math.Abs(1.0 - rates/float64(n))
    }
    return 0xFF
}

// GetLayer mirrors Python implementation.
func (lm *LayerManager) GetLayer(bleMeas []BLEMeas, twrMeas []TWRMeas, pos [3]float64, rssi *BLERssi, anchors map[int]Anchor) *int {
    layerList := []int{}
    outdoor := false
    for _, m := range bleMeas {
        a, ok := anchors[m.AnchorID]
        if !ok {
            continue
        }
        if a.Layer == OutdoorLayer {
            outdoor = true
        }
        if !containsInt(layerList, a.Layer) {
            layerList = append(layerList, a.Layer)
        }
    }
    for _, m := range twrMeas {
        a, ok := anchors[m.AnchorID]
        if !ok {
            continue
        }
        if a.Layer == OutdoorLayer {
            outdoor = true
        }
        if !containsInt(layerList, a.Layer) {
            layerList = append(layerList, a.Layer)
        }
    }
    if len(layerList) == 0 {
        return nil
    }

    proList := []*Project{}
    for _, lid := range layerList {
        if lid == OutdoorLayer {
            continue
        }
        lyr, ok := lm.layers[lid]
        if !ok || lyr.ProjectIdx < 0 || lyr.ProjectIdx >= len(lm.projects) {
            continue
        }
        proj := lm.projects[lyr.ProjectIdx]
        if isInProject(pos, proj) && !containsProject(proList, proj) {
            proList = append(proList, proj)
        }
    }

    if len(proList) == 0 {
        if outdoor {
            out := OutdoorLayer
            return &out
        }
        return nil
    }
    if len(proList) > 1 {
        return nil
    }

    layersInProj := []*Layer{}
    for _, lyr := range proList[0].Regions {
        if isInLayer(pos, lyr) {
            layersInProj = append(layersInProj, lyr)
        }
    }
    if len(layersInProj) == 0 {
        out := OutdoorLayer
        return &out
    }
    if len(layersInProj) == 1 {
        lid := layersInProj[0].ID
        return &lid
    }

    var bestLayer *int
    bestRate := 0xFF
    for _, lyr := range layersInProj {
        rate := layerTrustRate(bleMeas, twrMeas, pos, lyr.ID, rssi, anchors)
        if rate < float64(bestRate) {
            val := lyr.ID
            bestLayer = &val
            bestRate = int(rate)
        }
    }
    return bestLayer
}

func containsInt(arr []int, v int) bool {
    for _, x := range arr {
        if x == v {
            return true
        }
    }
    return false
}

func containsProject(arr []*Project, p *Project) bool {
    for _, x := range arr {
        if x == p {
            return true
        }
    }
    return false
}
