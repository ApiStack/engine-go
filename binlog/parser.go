package binlog

import (
    "encoding/binary"
    "errors"
    "fmt"
    "io"
    "math"
    "os"
)

const (
    pcapGlobalLen = 24 // PCAP_GLOBAL_STRUCT <IHHiiii
    pcapRecordLen = 16 // PCAP_RECORD_STRUCT <IIII
    phdr2Len      = 8  // PCAP_PHDR2_STRUCT <HHI

    flagAnchor = 0x04
    flagTag    = 0x08
    flagStats  = 0x10

    unibMagic    = 0x7857
    unibHdrLen   = 9
    unibWrapLen  = 11
    secondsFlag  = 0x2
)

type AnchorInfo struct {
    AnchorID uint64
    X        float64
    Y        float64
    Z        float64
    Region   uint16
}

type TagHeight struct {
    TagID uint64
    Height float64
}

type Sample struct {
    AnchorID int
    RSSIDb   int
    RangeM   float64
}

type IMUSample struct {
    Distance float64
    YawDeg   float64
}

type InnerFrame struct {
    Addr    uint32
    Type    uint8
    Samples []Sample
    IMU     *IMUSample
}

type Event struct {
    Timestamp float64
    Inner     []InnerFrame
}

type BinlogParser struct {
    Path string
    VerifyCRC bool

    Anchors []AnchorInfo
    Tags    []TagHeight
    Events  []Event
}

func NewBinlogParser(path string) *BinlogParser {
    return &BinlogParser{Path: path, VerifyCRC: true}
}

func (p *BinlogParser) Parse() error {
    f, err := os.Open(p.Path)
    if err != nil {
        return err
    }
    defer f.Close()

    hdr := make([]byte, pcapGlobalLen)
    if _, err := io.ReadFull(f, hdr); err != nil {
        return fmt.Errorf("pcap header: %w", err)
    }

    for {
        rec := make([]byte, pcapRecordLen)
        if _, err := io.ReadFull(f, rec); err != nil {
            if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
                break
            }
            return fmt.Errorf("pcap record: %w", err)
        }
        tsSec := binary.LittleEndian.Uint32(rec[0:4])
        tsUsec := binary.LittleEndian.Uint32(rec[4:8])
        inclLen := binary.LittleEndian.Uint32(rec[8:12])
        // origLen := binary.LittleEndian.Uint32(rec[12:16]) // unused
        if inclLen < phdr2Len {
            // malformed record, skip the stated length
            if _, err := f.Seek(int64(inclLen), io.SeekCurrent); err != nil {
                return fmt.Errorf("skip malformed record: %w", err)
            }
            continue
        }

        phdr := make([]byte, phdr2Len)
        if _, err := io.ReadFull(f, phdr); err != nil {
            if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
                break
            }
            return fmt.Errorf("pcap phdr2: %w", err)
        }
        flag := binary.LittleEndian.Uint16(phdr[0:2])
        wport := binary.LittleEndian.Uint16(phdr[2:4])
        uip := binary.LittleEndian.Uint32(phdr[4:8])

        payloadLen := int(inclLen) - phdr2Len
        if payloadLen <= 0 {
            continue
        }
        payload := make([]byte, payloadLen)
        if _, err := io.ReadFull(f, payload); err != nil {
            if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
                break
            }
            return fmt.Errorf("pcap payload: %w", err)
        }

        switch flag {
        case flagAnchor:
            p.parseAnchorBlock(payload, int(wport), int(uip))
            continue
        case flagTag:
            p.parseTagBlock(payload, int(wport), int(uip))
            continue
        case flagStats:
            // ignore
            continue
        }

        if len(payload) < unibWrapLen || binary.LittleEndian.Uint16(payload[0:2]) != unibMagic {
            continue
        }
        unib, err := parseUnib(payload, 0, p.VerifyCRC)
        if err != nil {
            continue
        }
        ts := float64(tsSec) + float64(tsUsec)/1e6
        evt, err := p.decodeOuter(unib)
        if err != nil {
            continue
        }
        p.Events = append(p.Events, Event{Timestamp: ts, Inner: evt})
    }
    return nil
}

func (p *BinlogParser) parseAnchorBlock(payload []byte, itemnum int, itemsize int) {
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
        p.Anchors = append(p.Anchors, AnchorInfo{AnchorID: anchorID, X: float64(x) / 100.0, Y: float64(y) / 100.0, Z: float64(z) / 100.0, Region: region})
    }
}

func (p *BinlogParser) parseTagBlock(payload []byte, itemnum int, itemsize int) {
    for i := 0; i < itemnum; i++ {
        start := i * itemsize
        end := start + itemsize
        if end > len(payload) {
            return
        }
        chunk := payload[start:end]
        tagID := binary.LittleEndian.Uint64(chunk[0:8])
        height := int32(binary.LittleEndian.Uint32(chunk[8:12]))
        p.Tags = append(p.Tags, TagHeight{TagID: tagID, Height: float64(height) / 100.0})
    }
}

// --------------------- UNIB parsing ----------------------------

type unibPacket struct {
    Addr uint32
    PktType uint8
    Flags uint8
    Body []byte
    TotalLen int
}

func parseUnib(data []byte, offset int, verifyCRC bool) (*unibPacket, error) {
    if len(data)-offset < unibWrapLen {
        return nil, fmt.Errorf("unib too short")
    }
    if binary.LittleEndian.Uint16(data[offset:offset+2]) != unibMagic {
        return nil, fmt.Errorf("unib magic")
    }
    addr := binary.LittleEndian.Uint32(data[offset+2 : offset+6])
    typeFlags := data[offset+6]
    typeLen := data[offset+7]
    typHigh := typeLen & 0x1F
    lenLow := typeLen >> 5
    lenHigh := data[offset+8]
    bodyLen := int(lenLow) + (int(lenHigh) << 3)
    bodyStart := offset + unibHdrLen
    bodyEnd := bodyStart + bodyLen
    if bodyEnd+2 > len(data) {
        return nil, fmt.Errorf("unib body truncated")
    }
    body := data[bodyStart:bodyEnd]
    crcRead := binary.LittleEndian.Uint16(data[bodyEnd : bodyEnd+2])
    if verifyCRC {
        if crc16(data[offset:bodyEnd]) != crcRead {
            return nil, fmt.Errorf("crc mismatch")
        }
    }
    typLow := typeFlags >> 3
    pktType := typLow + (typHigh << 5)
    flags := typeFlags & 0x7
    total := bodyLen + unibWrapLen
    return &unibPacket{Addr: addr, PktType: pktType, Flags: flags, Body: body, TotalLen: total}, nil
}

func crc16(data []byte) uint16 {
    var crc uint16 = 0
    for _, b := range data {
        crc ^= uint16(b) << 8
        for i := 0; i < 8; i++ {
            if crc&0x8000 != 0 {
                crc = (crc << 1) ^ 0x1021
            } else {
                crc <<= 1
            }
        }
    }
    return crc
}

func (p *BinlogParser) decodeOuter(pkt *unibPacket) ([]InnerFrame, error) {
    if pkt.PktType != 0x48 { // LORA_RAWDATA_UP
        return nil, nil
    }
    if len(pkt.Body) < 4 {
        return nil, fmt.Errorf("rawup too short")
    }
    var deviceID uint32
    var rssi int16
    offset := 0
    if len(pkt.Body) >= 6 {
        deviceID = binary.LittleEndian.Uint32(pkt.Body[0:4])
        rssi = int16(binary.LittleEndian.Uint16(pkt.Body[4:6]))
        offset = 6
    } else {
        deviceID = uint32(binary.LittleEndian.Uint16(pkt.Body[0:2]))
        rssi = int16(binary.LittleEndian.Uint16(pkt.Body[2:4]))
        offset = 4
    }
    _ = deviceID
    _ = rssi

    innerPayload := pkt.Body[offset:]
    inner := []InnerFrame{}
    pos := 0
    for pos+unibWrapLen <= len(innerPayload) {
        if binary.LittleEndian.Uint16(innerPayload[pos:pos+2]) != unibMagic {
            pos++
            continue
        }
        inPkt, err := parseUnib(innerPayload, pos, p.VerifyCRC)
        if err != nil {
            pos++
            continue
        }
        pos += inPkt.TotalLen
        frame, err := p.decodeInner(inPkt, pkt.Flags)
        if err == nil && frame != nil {
            inner = append(inner, *frame)
        }
    }
    return inner, nil
}

func (p *BinlogParser) decodeInner(pkt *unibPacket, parentFlags uint8) (*InnerFrame, error) {
    secFlags := pkt.Flags | parentFlags
    body := pkt.Body
    var secPrefix *uint8
    if secFlags&secondsFlag != 0 && len(body) > 0 {
        v := body[0]
        secPrefix = &v
        body = body[1:]
    }
    _ = secPrefix

    frame := InnerFrame{Addr: pkt.Addr, Type: pkt.PktType}
    switch pkt.PktType {
    case 0x50: // TWR
        seq, samples, err := decodeTwrSamples(body, false)
        _ = seq
        if err != nil {
            return nil, err
        }
        frame.Samples = samples
    case 0x52: // TWR_S
        seq, samples, err := decodeTwrSamples(body, true)
        _ = seq
        if err != nil {
            return nil, err
        }
        frame.Samples = samples
    case 0x60: // RSSI
        seq, samples, err := decodeRssi(body, false)
        _ = seq
        if err != nil {
            return nil, err
        }
        frame.Samples = samples
    case 0x61: // RSSI_S
        seq, samples, err := decodeRssi(body, true)
        _ = seq
        if err != nil {
            return nil, err
        }
        frame.Samples = samples
    case 0x90: // IMU
        imu, err := decodeIMU(body)
        if err != nil {
            return nil, err
        }
        frame.IMU = imu
    default:
        return nil, nil
    }
    return &frame, nil
}

func decodeTwrSamples(body []byte, short bool) (uint8, []Sample, error) {
    if len(body) < 2 {
        return 0, nil, fmt.Errorf("twr too short")
    }
    seq := body[0]
    meta := body[1]
    num := int(meta >> 4)
    pos := 2
    samples := []Sample{}
    if !short {
        for i := 0; i < num; i++ {
            if pos+5 > len(body) {
                return seq, nil, fmt.Errorf("twr sample trunc")
            }
            addrLow := binary.LittleEndian.Uint16(body[pos : pos+2])
            addrHi := body[pos+2]
            rng := binary.LittleEndian.Uint16(body[pos+3 : pos+5])
            pos += 5
            anchorID := int(uint32(addrHi)<<16 | uint32(addrLow))
            samples = append(samples, Sample{AnchorID: anchorID, RangeM: float64(rng) / 100.0})
        }
    } else {
        for i := 0; i < num; i++ {
            if pos+4 > len(body) {
                return seq, nil, fmt.Errorf("twr_s sample trunc")
            }
            addr := binary.LittleEndian.Uint16(body[pos : pos+2])
            rng := binary.LittleEndian.Uint16(body[pos+2 : pos+4])
            pos += 4
            samples = append(samples, Sample{AnchorID: int(addr), RangeM: float64(rng) / 100.0})
        }
    }
    return seq, samples, nil
}

func decodeRssi(body []byte, short bool) (uint8, []Sample, error) {
    if len(body) < 2 {
        return 0, nil, fmt.Errorf("rssi too short")
    }
    seq := body[0]
    meta := body[1]
    num := int(meta >> 4)
    pos := 2
    samples := []Sample{}
    if !short {
        // fallback for short encoded with type 0x60
        if num == 0 && len(body) >= 5 && (len(body)-2)%3 == 0 {
            num = (len(body) - 2) / 3
            for i := 0; i < num; i++ {
                if pos+3 > len(body) {
                    return seq, nil, fmt.Errorf("rssi short trunc")
                }
                addr := binary.LittleEndian.Uint16(body[pos : pos+2])
                rssi := int(int8(body[pos+2]))
                pos += 3
                samples = append(samples, Sample{AnchorID: int(addr), RSSIDb: rssi})
            }
        } else {
            for i := 0; i < num; i++ {
                if pos+4 > len(body) {
                    return seq, nil, fmt.Errorf("rssi trunc")
                }
                addrLow := binary.LittleEndian.Uint16(body[pos : pos+2])
                addrHi := body[pos+2]
                rssi := int(int8(body[pos+3]))
                pos += 4
                anchorID := int(uint32(addrHi)<<16 | uint32(addrLow))
                samples = append(samples, Sample{AnchorID: anchorID, RSSIDb: rssi})
            }
        }
    } else {
        for i := 0; i < num; i++ {
            if pos+3 > len(body) {
                return seq, nil, fmt.Errorf("rssi_s trunc")
            }
            addr := binary.LittleEndian.Uint16(body[pos : pos+2])
            rssi := int(int8(body[pos+2]))
            pos += 3
            samples = append(samples, Sample{AnchorID: int(addr), RSSIDb: rssi})
        }
    }
    return seq, samples, nil
}

func decodeIMU(body []byte) (*IMUSample, error) {
    if len(body) < 11 {
        return nil, fmt.Errorf("imu too short")
    }
    // payload: seq(1) + distance float32 + word1 uint32 + word2 uint16
    distance := math.Float32frombits(binary.LittleEndian.Uint32(body[1:5]))
    word1 := binary.LittleEndian.Uint32(body[5:9])
    yaw := word1 & 0x1FFF
    yawDeg := float64(yaw) * 360.0 / 8192.0
    return &IMUSample{Distance: float64(distance), YawDeg: yawDeg}, nil
}

// ------------------------------------------------------------------------

// GetTagHeight returns the height for a tag id if present, else default 1.2m
func (p *BinlogParser) GetTagHeight(tagID uint32) float64 {
    for _, t := range p.Tags {
        if t.TagID == uint64(tagID) {
            return t.Height
        }
    }
    return 1.2
}

// FilterSamples returns BLE, TWR and IMU measurements for a tag address in this event.
func (p *BinlogParser) FilterSamples(evt Event, tagID uint32) ([]Sample, []Sample, []IMUSample) {
    ble := []Sample{}
    twr := []Sample{}
    imu := []IMUSample{}
    for _, in := range evt.Inner {
        if in.Addr != tagID {
            continue
        }
        switch in.Type {
        case 0x60, 0x61:
            for _, s := range in.Samples {
                if s.RSSIDb != 0 || s.RangeM == 0 {
                    ble = append(ble, s)
                }
            }
        case 0x50, 0x52:
            for _, s := range in.Samples {
                if s.RangeM > 0 {
                    twr = append(twr, s)
                }
            }
        case 0x90:
            if in.IMU != nil {
                imu = append(imu, *in.IMU)
            }
        }
    }
    return ble, twr, imu
}

// EarliestEventTs returns earliest timestamp.
func (p *BinlogParser) EarliestEventTs() float64 {
    if len(p.Events) == 0 {
        return 0
    }
    min := math.MaxFloat64
    for _, e := range p.Events {
        if e.Timestamp < min {
            min = e.Timestamp
        }
    }
    return min
}
