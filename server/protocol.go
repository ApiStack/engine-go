package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	UnibMagic = 0x7857 // Little Endian for 'W' 'x'
	UnibHdrLen = 9
	UnibWrapLen = 11

	TypeTwrFrame   = 0x50
	TypeTwrFrameS  = 0x52
	TypeRssiFrame  = 0x60
	TypeRssiFrameS = 0x61
	TypeLoraRawDataUp = 0x48
	TypeImuFrame   = 0x90
)

type UnibHeader struct {
	Magic   uint16
	Addr    uint32
	Flags   uint8
	Type    uint16
	BodyLen int
}

type TwrSample struct {
	AnchorID int
	RangeM   float64
}

type RssiSample struct {
	AnchorID int
	RSSIDb   int
}

type ImuData struct {
	DistanceM float64
	YawDeg    float64
}

// ParseHeader parses the UNIB header from the beginning of the packet.
func ParseHeader(data []byte) (*UnibHeader, error) {
	if len(data) < UnibHdrLen {
		return nil, fmt.Errorf("packet too short")
	}

	magic := binary.LittleEndian.Uint16(data[0:2])
	if magic != UnibMagic {
		return nil, fmt.Errorf("invalid magic: 0x%x", magic)
	}

	addr := binary.LittleEndian.Uint32(data[2:6])
	
	// Byte 6: type_flags (typ_low:5, flags:3) -- Wait, struct says flags:3, typ_l:5.
	// C++: uint8_t flags:3, typ_l:5; (Bitfield order is compiler dependent but usually LSB first).
	// Python: type_flags = data[offset + 6]; typ_low = type_flags >> 3; flags = type_flags & 0x7
	// Let's follow Python logic which has proven to work on the binlogs.
	b6 := data[6]
	flags := b6 & 0x7
	typLow := uint16(b6 >> 3)

	// Byte 7: type_len (typ_h:5, len_l:3) -> typ_high = type_len & 0x1F; len_low = type_len >> 5
	b7 := data[7]
	typHigh := uint16(b7 & 0x1F)
	lenLow := int(b7 >> 5)

	// Byte 8: len_h
	lenHigh := int(data[8])

	pktType := typLow + (typHigh << 5)
	bodyLen := lenLow + (lenHigh << 3)

	return &UnibHeader{
		Magic:   magic,
		Addr:    addr,
		Flags:   flags,
		Type:    pktType,
		BodyLen: bodyLen,
	}, nil
}

func ParseTwrFrame(body []byte) ([]TwrSample, error) {
	if len(body) < 2 {
		return nil, fmt.Errorf("twr frame too short")
	}
	// seq := body[0]
	meta := body[1]
	num := int(meta >> 4)
	
	base := 2
	samples := make([]TwrSample, 0, num)
	for i := 0; i < num; i++ {
		if base+5 > len(body) {
			return nil, fmt.Errorf("twr sample truncated")
		}
		addrLow := binary.LittleEndian.Uint16(body[base : base+2])
		addrHi := uint32(body[base+2])
		rngRaw := binary.LittleEndian.Uint16(body[base+3 : base+5])
		base += 5

		anchorID := int(uint32(addrLow) | (addrHi << 16))
		samples = append(samples, TwrSample{
			AnchorID: anchorID,
			RangeM:   float64(rngRaw) / 100.0,
		})
	}
	return samples, nil
}

func ParseTwrFrameS(body []byte) ([]TwrSample, error) {
	if len(body) < 2 {
		return nil, fmt.Errorf("twr_s frame too short")
	}
	// seq := body[0]
	meta := body[1]
	num := int(meta >> 4)
	
	base := 2
	samples := make([]TwrSample, 0, num)
	for i := 0; i < num; i++ {
		if base+4 > len(body) {
			return nil, fmt.Errorf("twr_s sample truncated")
		}
		addr := binary.LittleEndian.Uint16(body[base : base+2])
		rngRaw := binary.LittleEndian.Uint16(body[base+2 : base+4])
		base += 4

		samples = append(samples, TwrSample{
			AnchorID: int(addr),
			RangeM:   float64(rngRaw) / 100.0,
		})
	}
	return samples, nil
}

func ParseRssiFrame(body []byte) ([]RssiSample, error) {
	if len(body) < 2 {
		return nil, fmt.Errorf("rssi frame too short")
	}
	meta := body[1]
	num := int(meta >> 4)
	
	base := 2
	samples := make([]RssiSample, 0, num)
	
	// Fallback for short format without correct type? Python parser has this logic.
	// if num == 0 && len(body) >= 5 && (len(body)-2)%3 == 0 ...
	// We will assume standard compliance for now.

	for i := 0; i < num; i++ {
		if base+4 > len(body) {
			return nil, fmt.Errorf("rssi sample truncated")
		}
		addrLow := binary.LittleEndian.Uint16(body[base : base+2])
		addrHi := uint32(body[base+2])
		rssi := int8(body[base+3])
		base += 4

		anchorID := int(uint32(addrLow) | (addrHi << 16))
		samples = append(samples, RssiSample{
			AnchorID: anchorID,
			RSSIDb:   int(rssi),
		})
	}
	return samples, nil
}

func ParseRssiFrameS(body []byte) ([]RssiSample, error) {
	if len(body) < 2 {
		return nil, fmt.Errorf("rssi_s frame too short")
	}
	meta := body[1]
	num := int(meta >> 4)
	
	base := 2
	samples := make([]RssiSample, 0, num)
	for i := 0; i < num; i++ {
		if base+3 > len(body) {
			return nil, fmt.Errorf("rssi_s sample truncated")
		}
		addr := binary.LittleEndian.Uint16(body[base : base+2])
		rssi := int8(body[base+2])
		base += 3

		samples = append(samples, RssiSample{
			AnchorID: int(addr),
			RSSIDb:   int(rssi),
		})
	}
	return samples, nil
}

func ParseImuFrame(body []byte) (*ImuData, error) {
	if len(body) < 11 {
		return nil, fmt.Errorf("imu frame too short")
	}
	
	var distance float32
	buf := bytes.NewReader(body[1:5])
	if err := binary.Read(buf, binary.LittleEndian, &distance); err != nil {
		return nil, err
	}

	word1 := binary.LittleEndian.Uint32(body[5:9])
	yawCode := word1 & 0x1FFF
	yawDeg := float64(yawCode) * (360.0 / 8192.0)

	return &ImuData{
		DistanceM: float64(distance),
		YawDeg:    yawDeg,
	}, nil
}
