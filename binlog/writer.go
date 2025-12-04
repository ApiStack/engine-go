package binlog

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

const (
	PcapMagic = 0xA1B2C3D4
)

type PcapWriter struct {
	mu  sync.Mutex
	w   io.Writer
	buf []byte
}

func NewPcapWriter(path string) (*PcapWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	pw := &PcapWriter{
		w:   f,
		buf: make([]byte, 32), // reused buffer for headers
	}

	if err := pw.writeGlobalHeader(); err != nil {
		f.Close()
		return nil, err
	}

	return pw, nil
}

func (pw *PcapWriter) writeGlobalHeader() error {
	// Global Header: 24 bytes
	// Magic(4), Major(2), Minor(2), Zone(4), Sig(4), Snap(4), Link(4)
	b := make([]byte, 24)
	binary.LittleEndian.PutUint32(b[0:], PcapMagic)
	binary.LittleEndian.PutUint16(b[4:], 2) // Major 2
	binary.LittleEndian.PutUint16(b[6:], 4) // Minor 4
	// Zone, Sig = 0
	binary.LittleEndian.PutUint32(b[16:], 65535) // SnapLen
	binary.LittleEndian.PutUint32(b[20:], 1)     // LinkType (Ethernet, but ignored)

	_, err := pw.w.Write(b)
	return err
}

func (pw *PcapWriter) WritePacket(flag uint16, addr *net.UDPAddr, data []byte) error {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	now := time.Now()
	tsSec := uint32(now.Unix())
	tsUsec := uint32(now.Nanosecond() / 1000)

	payloadLen := len(data)
	phdr2Len := 8
	totalLen := uint32(payloadLen + phdr2Len)

	// 1. Standard Record Header (16 bytes)
	// ts_sec(4), ts_usec(4), incl_len(4), orig_len(4)
	binary.LittleEndian.PutUint32(pw.buf[0:], tsSec)
	binary.LittleEndian.PutUint32(pw.buf[4:], tsUsec)
	binary.LittleEndian.PutUint32(pw.buf[8:], totalLen)
	binary.LittleEndian.PutUint32(pw.buf[12:], totalLen)

	if _, err := pw.w.Write(pw.buf[:16]); err != nil {
		return err
	}

	// 2. Custom Record Header 2 (8 bytes)
	// flag(2), port(2), ip(4)
	binary.LittleEndian.PutUint16(pw.buf[0:], flag)
	
	port := uint16(0)
	var ip4 net.IP
	if addr != nil {
		port = uint16(addr.Port)
		ip4 = addr.IP.To4()
	}
	binary.LittleEndian.PutUint16(pw.buf[2:], port)

	if ip4 != nil && len(ip4) == 4 {
		// Copy bytes directly to preserve Network Byte Order which is expected 
		// by C++ and Python tools even if the struct field is uint32.
		copy(pw.buf[4:8], ip4)
	} else {
		binary.LittleEndian.PutUint32(pw.buf[4:], 0)
	}

	if _, err := pw.w.Write(pw.buf[:8]); err != nil {
		return err
	}

	// 3. Payload
	if _, err := pw.w.Write(data); err != nil {
		return err
	}

	return nil
}

func (pw *PcapWriter) Close() error {
	if c, ok := pw.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
