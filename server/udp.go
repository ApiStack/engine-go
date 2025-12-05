package server

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
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

type wsPos struct {
	ID int64 `json:"id"`
	TS int64 `json:"ts"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	Z  float64 `json:"z"`
	Layer int `json:"layer"`
	Flag  int `json:"flag"`
}

type UdpServer struct {
	conn     *net.UDPConn
	pipeline *fusion.FusionPipeline
	pcap     *binlog.PcapWriter
	sender   *rbc.Sender
	webHub   *web.Hub
	running  bool
	
	// Map TagID -> Last Seen Gateway Addr
	lastGw map[int]*net.UDPAddr
	// Map TagID -> Last Known Position
	tagsState map[int]*wsPos
	mu     sync.Mutex
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
		lastGw:   make(map[int]*net.UDPAddr),
		tagsState: make(map[int]*wsPos),
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

func (s *UdpServer) GetTags() interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	tags := make([]*wsPos, 0, len(s.tagsState))
	for _, t := range s.tagsState {
		tags = append(tags, t)
	}
	return tags
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
		
		s.handlePacket(data, addr, time.Now().UnixMilli())
	}
}

func (s *UdpServer) Stop() {
	s.running = false
	s.conn.Close()
}

func (s *UdpServer) SendConfig(tagID int, cmdID int, data []byte) error {
	s.mu.Lock()
	addr, ok := s.lastGw[tagID]
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("gateway for tag %d not found", tagID)
	}

	// Ideally, we should know the Gateway ID to put in the header.
	// But we don't track Gateway IDs mapped to IPs yet.
	// The C++ code uses gwid in PkgingSetTagReq.
	// If we don't have it, we might use 0 or a dummy.
	// The Gateway might ignore it or use it.
	// Let's try to extract GatewayID from incoming packets if possible, but UnibHeader Addr is usually TagID for uplink.
	// Except for Gateway Heartbeat.
	
	// For now, use 0 as gwID.
	gwID := uint32(0) 
	
	pkt := PackageSetTagReq(gwID, uint32(tagID), uint8(cmdID), data)
	
	_, err := s.conn.WriteToUDP(pkt, addr)
	return err
}

func (s *UdpServer) handlePacket(data []byte, addr *net.UDPAddr, ts int64) {
	offset := 0
	for offset < len(data) {
		if len(data)-offset < UnibHdrLen {
			break
		}
		
		hdr, err := ParseHeader(data[offset:])
		if err != nil {
			offset++
			continue
		}

		totalLen := UnibWrapLen + hdr.BodyLen
		if offset+totalLen > len(data) {
			break
		}

		pktData := data[offset : offset+totalLen]

		if s.pcap != nil {
			_ = s.pcap.WritePacket(PcapFlag, addr, pktData)
		}

		bodyStart := offset + UnibHdrLen
		bodyEnd := bodyStart + hdr.BodyLen
		body := data[bodyStart:bodyEnd]
		
		// Update Gateway Map
		tagID := int(hdr.Addr)
		s.mu.Lock()
		s.lastGw[tagID] = addr
		s.mu.Unlock()

		s.processInner(hdr, body, ts, 0)

		offset += totalLen
	}
}

func (s *UdpServer) processInner(hdr *UnibHeader, body []byte, ts int64, parentFlags uint8) {
	combinedFlags := hdr.Flags | parentFlags
	realBody := body
	if combinedFlags & 0x2 != 0 && len(body) > 0 {
		realBody = body[1:]
	}

	tagID := int(hdr.Addr)

	switch hdr.Type {
	case TypeLoraRawDataUp:
		offset := 4
		if len(realBody) >= 6 {
			offset = 6
		}
		if len(realBody) <= offset {
			return
		}
		innerPayload := realBody[offset:]
		pos := 0
		for pos+UnibWrapLen <= len(innerPayload) {
			inHdr, err := ParseHeader(innerPayload[pos:])
			if err != nil {
				pos++
				continue
			}
			
			totalLen := UnibWrapLen + inHdr.BodyLen
			if pos+totalLen > len(innerPayload) {
				break
			}
			
			inBody := innerPayload[pos+UnibHdrLen : pos+UnibHdrLen+inHdr.BodyLen]
			s.processInner(inHdr, inBody, ts, hdr.Flags)
			pos += totalLen
		}

	case TypeTwrFrame:
		samples, err := ParseTwrFrame(realBody)
		if err == nil {
			// log.Printf("TWR Frame: Tag=%x Num=%d", tagID, len(samples))
			s.feedTwr(tagID, ts, samples)
		} else {
			log.Printf("ParseTwrFrame error: %v", err)
		}
	case TypeTwrFrameS:
		samples, err := ParseTwrFrameS(realBody)
		if err == nil {
			// log.Printf("TWR_S Frame: Tag=%x Num=%d", tagID, len(samples))
			s.feedTwr(tagID, ts, samples)
		} else {
			log.Printf("ParseTwrFrameS error: %v", err)
		}
	case TypeRssiFrame:
		samples, err := ParseRssiFrame(realBody)
		if err == nil {
			// log.Printf("RSSI Frame: Tag=%x Num=%d", tagID, len(samples))
			s.feedRssi(tagID, ts, samples)
		} else {
			log.Printf("ParseRssiFrame error: %v", err)
		}
	case TypeRssiFrameS:
		samples, err := ParseRssiFrameS(realBody)
		if err == nil {
			// log.Printf("RSSI_S Frame: Tag=%x Num=%d", tagID, len(samples))
			s.feedRssi(tagID, ts, samples)
		} else {
			log.Printf("ParseRssiFrameS error: %v", err)
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
			AnchorID: smp.AnchorID & 0xFFFF,
			Range:    smp.RangeM,
		}
	}
	res := s.pipeline.Process(ts, tagID, []fusion.BLEMeas{}, twrMeas, 0.0)
	s.sendResult(tagID, ts, res)
}

func (s *UdpServer) feedRssi(tagID int, ts int64, samples []RssiSample) {
	bleMeas := make([]fusion.BLEMeas, len(samples))
	for i, smp := range samples {
		bleMeas[i] = fusion.BLEMeas{
			AnchorID: smp.AnchorID & 0xFFFF,
			RSSIDb:   smp.RSSIDb,
		}
	}
	res := s.pipeline.Process(ts, tagID, bleMeas, []fusion.TWRMeas{}, 0.0)
	s.sendResult(tagID, ts, res)
}

func (s *UdpServer) sendResult(tagID int, ts int64, res fusion.FusionResult) {
	// Debug logging for large coordinates
	if math.Abs(res.X) > 1000.0 || math.Abs(res.Y) > 1000.0 {
		log.Printf("WARNING: Large Coordinate detected! Tag=%x X=%.2f Y=%.2f", tagID, res.X, res.Y)
	}

	// Debug logging for Replay tracking
	if res.Flag > 0 && tagID % 10 == 0 {
		// log.Printf("Pos: ID=%x Flag=%d X=%.2f Y=%.2f", tagID, res.Flag, res.X, res.Y)
	}
	
	region := 0
	if res.Layer != nil {
		region = *res.Layer
	}

	// Only send valid positions to RBC
	if res.Flag >= 1 && s.sender != nil {
		msg := rbc.FormatTagPos(tagID, ts, 0, region, res.X, res.Y, 0.0)
		s.sender.Send(msg, rbc.FlagPosition)
	}
	
	pos := &wsPos{
		ID:    int64(tagID),
		TS:    ts,
		X:     res.X,
		Y:     res.Y,
		Z:     0.0,
		Layer: region,
		Flag:  res.Flag,
	}
	
	// Update State (Always update, even if invalid/predictive)
	s.mu.Lock()
	s.tagsState[tagID] = pos
	s.mu.Unlock()

	if s.webHub != nil {
		b, _ := json.Marshal(pos)
		s.webHub.Broadcast(b)
	}
}