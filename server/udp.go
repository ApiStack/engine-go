package server

import (
	"encoding/json"
	"log"
	"net"
	"time"

	"engine-go/binlog"
	"engine-go/fusion"
	"engine-go/rbc"
	"engine-go/web"
)

const (
	DefaultPort = 44333
	MaxPacketSize = 65535
	
	// Flags: RX_PKT(1) | RBB_PKT(8) | PROT_UDP(0x100)
	PcapFlag = 0x109
)

type UdpServer struct {
	conn     *net.UDPConn
	pipeline *fusion.FusionPipeline
	pcap     *binlog.PcapWriter
	sender   *rbc.Sender
	webHub   *web.Hub
	running  bool
}

func NewUdpServer(port int, pipeline *fusion.FusionPipeline) (*UdpServer, error) {
	if port == 0 {
		port = DefaultPort
	}
	addr := net.UDPAddr{
		Port: port,
		IP:   net.ParseIP("0.0.0.0"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return nil, err
	}

	// Set buffer size similar to C++
	conn.SetReadBuffer(256 * 1024)

	return &UdpServer{
		conn:     conn,
		pipeline: pipeline,
	}, nil
}

func (s *UdpServer) SetPcapWriter(pw *binlog.PcapWriter) {
	s.pcap = pw
}

func (s *UdpServer) SetRbcSender(snd *rbc.Sender) {
	s.sender = snd
}

func (s *UdpServer) SetWebHub(h *web.Hub) {
	s.webHub = h
}

func (s *UdpServer) Start() {
	s.running = true
	buf := make([]byte, MaxPacketSize)
	log.Printf("UDP Server listening on %s", s.conn.LocalAddr().String())

	for s.running {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if s.running {
				log.Printf("Read error: %v", err)
			}
			continue
		}

		// Process packet
		// Make a copy of the data because parsing might slice it
		data := make([]byte, n)
		copy(data, buf[:n])
		
		s.handlePacket(data, addr)
	}
}

func (s *UdpServer) Stop() {
	s.running = false
	s.conn.Close()
}

func (s *UdpServer) handlePacket(data []byte, addr *net.UDPAddr) {
	// Basic loop to find magic header if multiple packets are concatenated
	// The C++ code handles concatenation and fragmentation. 
	// Here we assume UDP packets respect boundaries for simplicity, 
	// but we will loop through the buffer looking for headers.
	
	offset := 0
	for offset < len(data) {
		// Search for magic
		if len(data)-offset < UnibHdrLen {
			break
		}
		
		// Simple scan for magic if not at start (optional, but C++ does it)
		// For now, assume aligned.
		
		hdr, err := ParseHeader(data[offset:])
		if err != nil {
			// If invalid magic, maybe skip 1 byte and try again?
			// C++ FindNextPacketHeader does this.
			offset++
			continue
		}

		totalLen := UnibWrapLen + hdr.BodyLen
		if offset+totalLen > len(data) {
			// Truncated packet
			break
		}

		// Extract Packet Data
		pktData := data[offset : offset+totalLen]

		// Write to PCAP if enabled
		if s.pcap != nil {
			// We ignore write errors to avoid stalling processing
			_ = s.pcap.WritePacket(PcapFlag, addr, pktData)
		}

		// Extract Body
		// UNIB_WRAP_LEN is 11: Header(9) + CRC(2 at end).
		// Body starts at offset + 9
		bodyStart := offset + UnibHdrLen
		bodyEnd := bodyStart + hdr.BodyLen
		body := data[bodyStart:bodyEnd]

		// Handle Inner Packet
		s.processInner(hdr, body)

		offset += totalLen
	}
}

func (s *UdpServer) processInner(hdr *UnibHeader, body []byte) {
	// Handle seconds prefix if flag bit 1 is set
	// C++: bool bSec = pkg->flags & 0x2;
	// However, Python parser says: sec_flags = pkt.flags | parent_flags
	// In handlePacket(UnibHeader), we have the flags.
	
	realBody := body
	if hdr.Flags & 0x2 != 0 && len(body) > 0 {
		// sec := body[0]
		realBody = body[1:]
	}

	tagID := int(hdr.Addr)
	ts := time.Now().UnixMilli()

	switch hdr.Type {
	case TypeTwrFrame:
		samples, err := ParseTwrFrame(realBody)
		if err == nil {
			s.feedTwr(tagID, ts, samples)
		}
	case TypeTwrFrameS:
		samples, err := ParseTwrFrameS(realBody)
		if err == nil {
			s.feedTwr(tagID, ts, samples)
		}
	case TypeRssiFrame:
		samples, err := ParseRssiFrame(realBody)
		if err == nil {
			s.feedRssi(tagID, ts, samples)
		}
	case TypeRssiFrameS:
		samples, err := ParseRssiFrameS(realBody)
		if err == nil {
			s.feedRssi(tagID, ts, samples)
		}
	case TypeImuFrame:
		imu, err := ParseImuFrame(realBody)
		if err == nil {
			s.pipeline.ProcessIMU(ts, imu.DistanceM, imu.YawDeg)
		}
	}
}

func (s *UdpServer) feedTwr(tagID int, ts int64, samples []TwrSample) {
	twrMeas := make([]fusion.TWRMeas, len(samples))
	for i, smp := range samples {
		twrMeas[i] = fusion.TWRMeas{
			AnchorID: smp.AnchorID,
			Range:    smp.RangeM,
		}
	}
	// Empty BLE for TWR frame
	res := s.pipeline.Process(ts, tagID, []fusion.BLEMeas{}, twrMeas, 0.0)
	s.sendResult(tagID, ts, res)
}

func (s *UdpServer) feedRssi(tagID int, ts int64, samples []RssiSample) {
	bleMeas := make([]fusion.BLEMeas, len(samples))
	for i, smp := range samples {
		bleMeas[i] = fusion.BLEMeas{
			AnchorID: smp.AnchorID,
			RSSIDb:   smp.RSSIDb,
		}
	}
	// Empty TWR for RSSI frame
	res := s.pipeline.Process(ts, tagID, bleMeas, []fusion.TWRMeas{}, 0.0)
	s.sendResult(tagID, ts, res)
}

type wsPos struct {
	ID int64 `json:"id"`
	TS int64 `json:"ts"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	Z  float64 `json:"z"`
	Layer int `json:"layer"`
}

func (s *UdpServer) sendResult(tagID int, ts int64, res fusion.FusionResult) {
	if res.Flag != 1 {
		return
	}
	
	region := 0
	if res.Layer != nil {
		region = *res.Layer
	}

	// RBC Format
	if s.sender != nil {
		// Z is not returned by FusionResult, assuming 0.0 for now
		msg := rbc.FormatTagPos(tagID, ts, 0, region, res.X, res.Y, 0.0)
		s.sender.Send(msg, rbc.FlagPosition)
	}

	// Web Broadcast
	if s.webHub != nil {
		pos := wsPos{
			ID:    int64(tagID),
			TS:    ts,
			X:     res.X,
			Y:     res.Y,
			Z:     0.0,
			Layer: region,
		}
		b, _ := json.Marshal(pos)
		s.webHub.Broadcast(b)
	}
}
