package rbc

import (
	"log"
	"net"
	"sync"
	"time"
)

type Message struct {
	Data []byte
	Flag uint32
}

type UdpTarget struct {
	addr *net.UDPAddr
	flag uint32
}

type TcpClient struct {
	addr    string
	flag    uint32
	queue   chan *Message
	running bool
	wg      sync.WaitGroup
}

type Sender struct {
	udpTargets []*UdpTarget
	tcpClients []*TcpClient
	connUDP    *net.UDPConn
	header     []byte
	running    bool
}

func NewSender() *Sender {
	return &Sender{
		udpTargets: make([]*UdpTarget, 0),
		tcpClients: make([]*TcpClient, 0),
	}
}

func (s *Sender) SetHeader(hdr string) {
	if hdr == "" {
		s.header = nil
	} else {
		s.header = []byte(hdr + ":")
	}
}

func (s *Sender) AddUDPSender(addr string, flag uint32) error {
	uaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	s.udpTargets = append(s.udpTargets, &UdpTarget{addr: uaddr, flag: flag})
	return nil
}

func (s *Sender) AddTCPSender(addr string, flag uint32) {
	client := &TcpClient{
		addr:  addr,
		flag:  flag,
		queue: make(chan *Message, 1000),
	}
	s.tcpClients = append(s.tcpClients, client)
}

func (s *Sender) Start() error {
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return err
	}
	s.connUDP = conn
	s.running = true

	for _, c := range s.tcpClients {
		c.Start()
	}
	return nil
}

func (s *Sender) Stop() {
	s.running = false
	if s.connUDP != nil {
		s.connUDP.Close()
	}
	for _, c := range s.tcpClients {
		c.Stop()
	}
}

func (s *Sender) Send(data []byte, flag uint32) {
	if !s.running {
		return
	}

	var msgData []byte
	if len(s.header) > 0 {
		msgData = make([]byte, len(s.header)+len(data))
		copy(msgData, s.header)
		copy(msgData[len(s.header):], data)
	} else {
		msgData = data
	}

	msg := &Message{Data: msgData, Flag: flag}

	// UDP
	for _, t := range s.udpTargets {
		if (t.flag & flag) == flag {
			_, err := s.connUDP.WriteToUDP(msgData, t.addr)
			if err != nil {
				// log.Printf("UDP send error: %v", err)
			}
		}
	}

	// TCP
	for _, c := range s.tcpClients {
		if (c.flag & flag) == flag {
			select {
			case c.queue <- msg:
			default:
				// Drop if full
			}
		}
	}
}

func (c *TcpClient) Start() {
	c.running = true
	c.wg.Add(1)
	go c.loop()
}

func (c *TcpClient) Stop() {
	c.running = false
	close(c.queue)
	c.wg.Wait()
}

func (c *TcpClient) loop() {
	defer c.wg.Done()
	var conn net.Conn
	var err error

	connect := func() bool {
		if conn != nil {
			return true
		}
		conn, err = net.DialTimeout("tcp", c.addr, 2*time.Second)
		if err != nil {
			return false
		}
		return true
	}

	for msg := range c.queue {
		if !c.running {
			break
		}

		if !connect() {
			// Wait a bit before retrying or dropping?
			// C++ logic sleeps 500ms. We can try to consume more messages or just sleep.
			// If we block here, we block the queue.
			time.Sleep(500 * time.Millisecond)
			if !connect() {
				continue // drop this message
			}
		}

		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err = conn.Write(msg.Data)
		if err != nil {
			log.Printf("TCP write to %s failed: %v", c.addr, err)
			conn.Close()
			conn = nil
			time.Sleep(100 * time.Millisecond)
		}
	}
	if conn != nil {
		conn.Close()
	}
}