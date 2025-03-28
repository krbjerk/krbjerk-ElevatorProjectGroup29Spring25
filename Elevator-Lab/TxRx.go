package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"root/elevio"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtaci/kcp-go/v5"
)

//const NUM_FLOORS = 4

// Simulator
var _initialized bool = false
var _numFloors int = 4
var _mtx sync.Mutex
var _conn net.Conn

var slaveOrderChans = make(map[int32]chan [][4][3]bool)
var Read = make(chan string, 10)
var Send = make(chan string, 10)

var ActiveConnection bool = false
var Master bool

// For sending between elevators
var ipToID = make(map[string]int)
var RequestToID = make(map[string]int)
var idCounter = 1
var mutex sync.Mutex // Protects the map from race conditions

var (
	slaveMap      = make(map[string]int32)
	slaveMapMutex sync.Mutex
	IDCounter     int32 = 0
)

func MasterCheck(masterTimer int, _ELS *ElevatorList, _EL *Elevator) {
	timeoutDuration := time.Duration(masterTimer) * time.Millisecond
	deadline := time.Now().Add(timeoutDuration)

	fmt.Println("Searching for a master...")

	for {
		// Try to connect to a potential master using KCP
		conn, err := kcp.DialWithOptions("192.168.0.190:4001", nil, 10, 3) // Broadcast
		if err == nil {
			conn.SetDeadline(time.Now().Add(100 * time.Millisecond))
			_, err = conn.Write([]byte("ping"))
			if err == nil {
				// Wait for "ack" response
				buffer := make([]byte, 1024)
				n, err := conn.Read(buffer)
				if err == nil && string(buffer[:n]) == "ack" {
					fmt.Println("Master found! Running SendToMaster.")
					Master = false
					go _EL.SendToMaster(Send, _ELS)
					conn.Close()
					return
				}
			}
			conn.Close()
		}

		// Check if timeout has expired
		if time.Now().After(deadline) {
			fmt.Println("No master found. Becoming master.")
			Master = true
			go ackResponder()
			go _ELS.ReadFromSlave(Read)
			return
		}

		// Retry after 1 second
		time.Sleep(100 * time.Millisecond)
	}
}

// ackResponder listens on port 4001 and responds to handshake messages.
// It only sends "ack" if it receives a "ping".
func ackResponder() {
	listener, err := kcp.ListenWithOptions(":4001", nil, 10, 3) // Listen on all interfaces
	if err != nil {
		log.Fatalf("Failed to start KCP listener: %v", err)
	}
	defer listener.Close()
	fmt.Println("Master is running and responding to KCP discovery requests.")

	for {
		conn, err := listener.AcceptKCP()
		if err != nil {
			log.Printf("Error accepting KCP connection: %v", err)
			continue
		}

		// Handle each connection in a new goroutine
		go func(c *kcp.UDPSession) {
			defer c.Close()

			// Read data
			buffer := make([]byte, 1024)
			n, err := c.Read(buffer)
			if err != nil {
				log.Printf("Error reading data: %v", err)
				return
			}

			// If message is "ping", respond with "ack"
			if string(buffer[:n]) == "ping" {
				_, err = c.Write([]byte("ack"))
				if err != nil {
					log.Printf("Error sending ack: %v", err)
				} else {
					fmt.Println("Sent ack to a client")
				}
			}
		}(conn)
	}
}

// UNDER PROGRESS:
// ReadFromSlave accepts connections from slaves.
/*
func (_ELS ElevatorList) ReadFromSlave(receiver chan<- string) {
	listener, err := kcp.ListenWithOptions(":4001", nil, 10, 3)

	if err != nil {
		log.Fatalf("Failed to start KCP server: %v", err)
	}
	defer listener.Close()
	fmt.Println("KCP Master (Server) listening on port 4001...")

	for {
		conn, err := listener.AcceptKCP()
		if err != nil {
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}
		remoteAddr := conn.RemoteAddr().(*net.UDPAddr).IP.String()

		// Assign or retrieve the ID
		mutex.Lock()
		id, exists := ipToID[remoteAddr]

		if !exists {
			id = idCounter
			ipToID[remoteAddr] = id
			idCounter++
		} else {
			var update uint8 = uint8(BoolToInt(_ELS[id].m_requests[3][2])&0b1 | BoolToInt(_ELS[id].m_requests[2][2])&0b1<<1 | BoolToInt(_ELS[id].m_requests[1][2])&0b1<<2 | BoolToInt(_ELS[id].m_requests[0][2])&0b1<<3)
			_, err = conn.Write([]byte{update})
			if err != nil {
				fmt.Println(err)
			}
		}
		mutex.Unlock()

		fmt.Printf("Slave connected from %s assigned ID: %d\n", remoteAddr, id)

		receive := make(chan string, 10)

		go HandleConnections(conn, receive)
		go func() {
			for data := range receive {
				receiver <- data + strconv.Itoa(id)
			}
		}()
	}
}
*/
// HandleConnections reads data from the connection, processes it,
// waits for an order, and sends a response back.
/*
func HandleConnections(conn *kcp.UDPSession, receive chan<- string) {
	defer conn.Close()
	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				log.Printf("Slave Disconnected")
			} else {
				log.Printf("Read error from Slave", err)
			}
		}

		var data string
		for _, b := range buffer[:n] {
			data += fmt.Sprintf("%08b", b)
		}
		receive <- data
		fmt.Println("Data Received", data)

		response := "n"
		response2 := []byte(response)

		if len(masterOrder) > int(id) && len(masterOrder[id]) > 0 {
			fmt.Println("if")
			otherRequests := masterOrder[:slaveID]
			otherRequests = append(otherRequests, masterOrder[slaveID+1:]...)
			otherRequest := MergeRequestsSlice(otherRequests)
			// TODO: Add other requests to response. Easiest if it can be two end bytes so it is easy to pick out.
			response2 = []byte(EncodeMatrixToString(masterOrder[id]))
			fmt.Println("If-sentence")
		}

		_, err = conn.Write([]byte(response2))
		if err != nil {
			log.Printf("Failed to send response to Slave", err)
		}

	}
}*/

// -------

func (_ELS ElevatorList) ReadFromSlave(receiver chan<- string) {
	listener, err := kcp.ListenWithOptions(":4000", nil, 10, 3)

	if err != nil {
		log.Fatalf("Failed to start KCP server: %v", err)
	}
	defer listener.Close()
	fmt.Println("KCP Master (Server) listening on port 4001...")

	for {
		conn, err := listener.AcceptKCP()
		if err != nil {
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}
		addr := conn.RemoteAddr().String()
		host, _, err := net.SplitHostPort(addr)

		if err != nil {
			host = addr
		}

		slaveMapMutex.Lock()
		id, exists := slaveMap[host]

		if !exists {
			id = atomic.AddInt32(&IDCounter, 1)
			slaveMap[host] = id
		} else {
			var update uint8 = uint8(BoolToInt(_ELS[id].m_requests[3][2])&0b1 | BoolToInt(_ELS[id].m_requests[2][2])&0b1<<1 | BoolToInt(_ELS[id].m_requests[1][2])&0b1<<2 | BoolToInt(_ELS[id].m_requests[0][2])&0b1<<3)
			_, err = conn.Write([]byte{update})
			if err != nil {
				fmt.Println(err)
			}
		}
		slaveMapMutex.Unlock()

		fmt.Printf("Slave connected from %s assigned ID: %d\n", id, host)

		receive := make(chan string, 10)
		orderChan := make(chan [][4][3]bool, 10)

		slaveMapMutex.Lock()
		slaveOrderChans[id] = orderChan
		slaveMapMutex.Unlock()

		go HandleConnections(conn, receive, id, orderChan, host)
		go func(slaveID int32) {
			for data := range receive {
				receiver <- data + strconv.Itoa(int(slaveID))
			}
		}(id)
	}
}

// HandleConnections reads data from the connection, processes it,
// waits for an order, and sends a response back.

func HandleConnections(conn *kcp.UDPSession, receive chan<- string, id int32, orderChan <-chan [][4][3]bool, host string) {
	defer conn.Close()
	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				log.Printf("Slave Disconnected")
			} else {
				log.Printf("Read error from Slave", err)
			}
		}

		var data string
		for _, b := range buffer[:n] {
			data += fmt.Sprintf("%08b", b)
		}
		receive <- data
		fmt.Println("Data Received", data)

		response := "n"
		response2 := []byte(response)

		select {
		case masterOrder := <-orderChan:
			if len(masterOrder) > int(id) && len(masterOrder[id]) > 0 {
				otherRequests := masterOrder[:id]
				otherRequests = append(otherRequests, masterOrder[id+1:]...)
				otherRequest := MergeRequestsSlice(otherRequests)
				response2 = []byte(EncodeMatrixToString(masterOrder[id]) + EncodeMatrixToString(otherRequest))
			}
		case <-time.After(100 * time.Millisecond):
			log.Printf("No master order available for slave %d, sending default response.\n", id)
		}

		_, err = conn.Write([]byte(response2))
		if err != nil {
			log.Printf("Failed to send response to Slave", err)
		}

	}
}

// -------

func (EL *Elevator) SendToMaster(receiver chan<- string, _ELS *ElevatorList) {
	conn, err := kcp.DialWithOptions("192.168.0.176:4000", nil, 10, 3)
	if err != nil {
		log.Fatalf("Failed to connect to master: %v", err)
	}
	defer conn.Close()

	fmt.Println("Connected to Master!")

	for {
		EL.printElevatorState()
		packet := EncodeElevator(EL)
		_, err := conn.Write(packet[:])
		if err != nil {
			log.Println("Failed to send data:", err)
			return
		}
		fmt.Println("Sent to Master:", packet)
		//TROUBLESHOOTING:
		testbuf := packet
		testbitstring := ""
		for _, testb := range testbuf {
			testbitstring += fmt.Sprintf("%08b", testb)
		}
		fmt.Println(testbitstring)

		// Read response from Master
		buffer := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second)) // Prevent infinite blocking
		n, err := conn.Read(buffer)

		if err != nil {
			fmt.Println("Failed to read response:", err)
			MasterCheck(rand.Intn(1500)+300, _ELS, EL)
			ActiveConnection = false
			continue
		}
		receiver <- string(buffer[:n])
		ActiveConnection = true

		time.Sleep(1 * time.Second)
	}
}

func Init(addr string, NUM_FLOORS int) {
	if _initialized {
		fmt.Println("Driver already initialized!")
		return
	}
	_numFloors = NUM_FLOORS
	_mtx = sync.Mutex{}
	var err error
	_conn, err = net.Dial("tcp", addr)
	if err != nil {
		panic(err.Error())
	}
	_initialized = true
}

func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func EncodeElevator(e *Elevator) [3]byte {
	var b0, b1, b2 byte
	req := e.m_requests

	// Byte 0: floor 0–1 buttons
	if req[0][0] {
		b0 |= 1 << 0
	}
	if req[0][1] {
		b0 |= 1 << 1
	}
	if req[0][2] {
		b0 |= 1 << 2
	}
	if req[1][0] {
		b0 |= 1 << 3
	}
	if req[1][1] {
		b0 |= 1 << 4
	}
	if req[1][2] {
		b0 |= 1 << 5
	}

	// Byte 1: floor 2–3 buttons
	if req[2][0] {
		b1 |= 1 << 0
	}
	if req[2][1] {
		b1 |= 1 << 1
	}
	if req[2][2] {
		b1 |= 1 << 2
	}
	if req[3][0] {
		b1 |= 1 << 3
	}
	if req[3][1] {
		b1 |= 1 << 4
	}
	if req[3][2] {
		b1 |= 1 << 5
	}

	// Byte 2: state info
	b2 |= byte(e.m_behavior & 0b11)
	b2 |= byte((e.m_dirn+1)&0b11) << 2
	b2 |= byte(e.m_floor&0b11) << 4

	return [3]byte{b0, b1, b2}
}

func DecodeElevator(data [3]byte) Elevator {
	var e Elevator
	b0, b1, b2 := data[0], data[1], data[2]

	// --- Byte 0: floors 0..1 ---
	// floor 0
	e.m_requests[0][0] = (b0 & (1 << 0)) != 0 // up[0]
	e.m_requests[0][1] = (b0 & (1 << 1)) != 0 // down[0]
	e.m_requests[0][2] = (b0 & (1 << 2)) != 0 // cab[0]
	// floor 1
	e.m_requests[1][0] = (b0 & (1 << 3)) != 0 // up[1]
	e.m_requests[1][1] = (b0 & (1 << 4)) != 0 // down[1]
	e.m_requests[1][2] = (b0 & (1 << 5)) != 0 // cab[1]

	// --- Byte 1: floors 2..3 ---
	// floor 2
	e.m_requests[2][0] = (b1 & (1 << 0)) != 0 // up[2]
	e.m_requests[2][1] = (b1 & (1 << 1)) != 0 // down[2]
	e.m_requests[2][2] = (b1 & (1 << 2)) != 0 // cab[2]
	// floor 3
	e.m_requests[3][0] = (b1 & (1 << 3)) != 0 // up[3]
	e.m_requests[3][1] = (b1 & (1 << 4)) != 0 // down[3]
	e.m_requests[3][2] = (b1 & (1 << 5)) != 0 // cab[3]

	// --- Byte 2: elevator state ---
	e.m_behavior = ElevatorBehavior(b2 & 0b11)               // bits 0..1
	e.m_dirn = elevio.MotorDirection(((b2 >> 2) & 0b11) - 1) // bits 2..3 - 1
	e.m_floor = int((b2 >> 4) & 0b11)                        // bits 4..5

	return e
}

func DecodeElevatorFromString(s string) Elevator {
	if len(s) != 24 {
		fmt.Println("DecodeElevatorFromString: invalid length:", len(s))
		return Elevator{}
	}
	b0, _ := strconv.ParseUint(s[0:8], 2, 8)
	b1, _ := strconv.ParseUint(s[8:16], 2, 8)
	b2, _ := strconv.ParseUint(s[16:24], 2, 8)

	return DecodeElevator([3]byte{byte(b0), byte(b1), byte(b2)})
}

func StringToByteList(s string) []byte {
	var result []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '1' {
			result = append(result, 1)
		} else {
			result = append(result, 0)
		}
	}
	return result
}

func stringCabToRequest(bin string) [4][3]bool {
	var result [4][3]bool

	// Bits 0–3 → [3][2] to [0][2]
	for i := 0; i < 4; i++ {
		bit := bin[7-i] // rightmost bit is index 7
		result[i][2] = bit == '1'
	}

	return result
}
