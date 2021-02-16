package rtltcp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/google/uuid"
	"github.com/quan-to/slog"
	"github.com/racerxdl/go.fifo"
	"github.com/racerxdl/qo100-dedrift/metrics"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const defaultReadTimeout = time.Second
const chunkLength = 4096
const maxFifoLength = 64

var log = slog.Scope("RTLTCP Server")

type OnCommand func(sessionId string, cmd Command) bool
type OnConnect func(sessionId string, address string)

type Server struct {
	address     string
	connections []*Session
	dongleInfo  *DongleInfo

	connectionLock sync.Mutex
	running        bool
	waitClose      chan bool
	serverListener net.Listener
	onCommandCb    OnCommand
	onConnectCb    OnConnect
	bufferFifo     *fifo.Queue
}

func MakeRTLTCPServer(address string) *Server {
	return &Server{
		address:        address,
		connections:    make([]*Session, 0),
		connectionLock: sync.Mutex{},
		running:        false,
		dongleInfo: &DongleInfo{
			Magic:          [4]uint8{'R', 'T', 'L', '0'},
			TunerType:      RtlsdrTunerR820t,
			TunerGainCount: 0,
		},
		bufferFifo: fifo.NewQueue(),
	}
}

func (server *Server) SetDongleInfo(info DongleInfo) {
	server.dongleInfo = &info
	server.dongleInfo.Magic = [4]uint8{'R', 'T', 'L', '0'}
}

func (server *Server) Start() error {
	if !server.running {
		l, err := net.Listen("tcp", server.address)
		if err != nil {
			slog.Fatal("Error listening:", err.Error())
		}
		server.serverListener = l
		log.Info("Listening on %s", server.address)
		server.waitClose = make(chan bool)
		server.running = true
		go server.loop()
		go server.txLoop()
		return nil
	}

	return fmt.Errorf("already running")
}

func (server *Server) Stop() {
	if server.running {
		server.running = false
		log.Info("Sent close signal to server. Waiting it to finish")
		if server.serverListener != nil {
			_ = server.serverListener.Close()
		}
		<-server.waitClose
	}
}

func (server *Server) ComplexBroadcast(data []complex64) {
	if server.bufferFifo.Len() > maxFifoLength {
		log.Error("TX Fifo full!")
		return
	}

	iqBytes := make([]byte, len(data)*2)

	for i, v := range data {
		rv := 128 + real(v)*127
		iv := 128 + imag(v)*127

		if rv < 0 {
			rv = 0
		}
		if rv > 255 {
			rv = 255
		}

		if iv < 0 {
			iv = 0
		}

		if iv > 255 {
			iv = 255
		}

		iqBytes[i*2] = uint8(rv)
		iqBytes[i*2+1] = uint8(iv)
	}

	server.bufferFifo.Add(iqBytes)
}

func (server *Server) broadcast(data []byte) {
	server.connectionLock.Lock()
	chunks := len(data) / chunkLength
	for i := 0; i < chunks; i++ {
		s := i * chunkLength
		e := (i + 1) * chunkLength
		if e > len(data) {
			e = len(data)
		}
		payload := data[s:e]

		for _, v := range server.connections {
			n, _ := v.conn.Write(payload)
			metrics.BytesOut.Add(float64(n))
		}
	}
	server.connectionLock.Unlock()
}

func (server *Server) SetOnConnect(cb OnConnect) {
	server.onConnectCb = cb
}

func (server *Server) SetOnCommand(cb OnCommand) {
	server.onCommandCb = cb
}

func (server *Server) txLoop() {
	// Wait fifo to heat up
	for server.running && server.bufferFifo.Len() < 4 {
	}

	for server.running {
		// Now we can TX
		if server.bufferFifo.Len() > 0 {
			b := server.bufferFifo.Next().([]byte)
			server.broadcast(b)
			runtime.Gosched()
		} else {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func (server *Server) loop() {
	for server.running {
		// Listen for an incoming connection.
		conn, err := server.serverListener.Accept()
		if err != nil {
			if !strings.Contains(err.Error(), "use of closed") {
				log.Fatal("Error accepting: %s", err.Error())
			}
		} else {
			// Handle connections in a new goroutine.
			go server.handleRequest(conn)
		}
	}
	_ = server.serverListener.Close()
	log.Info("Server finished listening")
	// Send close signal
	server.waitClose <- true
}

func (server *Server) handlePacket(session *Session, cmd Command) {
	uParam := binary.BigEndian.Uint32(cmd.Param[:]) // Convert to local endianess
	session.log.Debug("Received Type %s (%d) with arg (%d) %v", CommandTypeToName[cmd.Type], cmd.Type, uParam, cmd.Param)

	if server.onCommandCb != nil {
		ok := server.onCommandCb(session.id, cmd)
		if !ok {
			_ = session.conn.Close()
		}
	}
}

func (server *Server) handleRequest(conn net.Conn) {
	uid, _ := uuid.NewRandom()
	// Create Session
	session := &Session{
		id:   uid.String(),
		conn: conn,
		log:  slog.Scope(conn.RemoteAddr().String()),
	}
	clog := session.log

	clog.Info("Received connection")

	clog.Debug("Sending greeting with DongleInfo")
	err := binary.Write(conn, binary.BigEndian, server.dongleInfo)
	if err != nil {
		clog.Error("Error sending greeting: %s", err)
		return
	}

	// Adding to connection pool
	server.connectionLock.Lock()
	server.connections = append(server.connections, session)
	server.connectionLock.Unlock()

	cmd := Command{}
	buffer := make([]byte, unsafe.Sizeof(cmd))
	running := true

	if server.onConnectCb != nil {
		server.onConnectCb(session.id, session.conn.RemoteAddr().String())
	}

	metrics.TotalConnections.Inc()
	metrics.Connections.Inc()

	for running {
		_ = conn.SetReadDeadline(time.Now().Add(defaultReadTimeout))
		n, err := conn.Read(buffer)

		if err != nil {
			if err.Error() != "EOF" {
				switch e := err.(type) {
				case net.Error:
					if !e.Timeout() {
						if running {
							clog.Error("Error receiving data: %s", e)
						}
						running = false
					}
				default:
					clog.Error("Error receiving data: %s", e)
					running = false
				}
			} else {
				running = false
			}
		}

		if !running {
			break
		}

		if n > 0 {
			b := bytes.NewReader(buffer)
			err := binary.Read(b, binary.LittleEndian, &cmd)
			if err != nil {
				clog.Error("Error parsing packet: %s", err)
				continue
			}
			server.handlePacket(session, cmd)
			metrics.BytesIn.Add(float64(n))
		}
	}
	server.connectionLock.Lock()
	for i, v := range server.connections {
		if v.id == session.id {
			server.connections = append(server.connections[:i], server.connections[i+1:]...)
			break
		}
	}
	server.connectionLock.Unlock()
	_ = conn.Close()

	metrics.Connections.Dec()
	clog.Info("Connection closed.")
}
