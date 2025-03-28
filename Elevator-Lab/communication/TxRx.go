package communication

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"root/elevator"
	"root/elevio"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtaci/kcp-go/v5"
)

// Exported so main.go can see them:
var (
	SlaveOrderChans  = make(map[int32]chan [][4][3]bool)
	Read             = make(chan string, 10)
	Send             = make(chan string, 10)
	ActiveConnection bool
	Master           bool

	SlaveMap      = make(map[string]int32)
	SlaveMapMutex sync.Mutex
	IDCounter     int32

	ipList      = []string{"10.22.113.144", "10.22.123.211", "10.22.123.33"}
	MasterIndex int

	packetLoss = 20
)

// MasterCheck tries to find a running master (via KCP). If none found, become master.
func MasterCheck(masterTimer int, _ELS *elevator.ElevatorList, _EL *elevator.Elevator) {
	timeoutDuration := time.Duration(masterTimer) * time.Millisecond
	deadline := time.Now().Add(timeoutDuration)

	fmt.Println("Searching for a master...")

	for {
		// Attempt to connect to a master on port 4001
		for i := range ipList {
			conn, err := kcp.DialWithOptions(ipList[i]+":4001", nil, 10, 3) // example IP
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
						go SendToMaster(_EL, Send, _ELS)
						return
					}
				}
				conn.Close()
			}
		}
		if time.Now().After(deadline) {
			fmt.Println("No master found. Becoming master.")
			Master = true
			go ackResponder()
			go ReadFromSlave(_ELS, Read)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ackResponder listens on port 4001 for handshake pings and responds with ack.
func ackResponder() {
	listener, err := kcp.ListenWithOptions(":4001", nil, 10, 3) // listen on all interfaces
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
		go func(c *kcp.UDPSession) {
			defer c.Close()

			buffer := make([]byte, 1024)
			n, err := c.Read(buffer)
			if err != nil {
				log.Printf("Error reading data: %v", err)
				return
			}
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

// ReadFromSlave listens on port 4000 for incoming slave connections.
func ReadFromSlave(_ELS *elevator.ElevatorList, receiver chan<- string) {
	listener, err := kcp.ListenWithOptions(":4000", nil, 10, 3)
	if err != nil {
		log.Fatalf("Failed to start KCP server: %v", err)
	}
	defer listener.Close()
	fmt.Println("KCP Master (Server) listening on port 4000...")

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

		SlaveMapMutex.Lock()
		id, exists := SlaveMap[host]
		if !exists {
			id = atomic.AddInt32(&IDCounter, 1)
			SlaveMap[host] = id
		} else {
			/*// If we already know this ID, send back a status update
			var update uint8 = uint8(
				BoolToInt(elevator.GetIndRequest((*_ELS)[id], 3, 2))&0b1 |
					BoolToInt(elevator.GetIndRequest((*_ELS)[id], 2, 2))&0b1<<1 |
					BoolToInt(elevator.GetIndRequest((*_ELS)[id], 1, 2))&0b1<<2 |
					BoolToInt(elevator.GetIndRequest((*_ELS)[id], 0, 2))&0b1<<3)
			_, err = conn.Write([]byte{update})*/
			if err != nil {
				fmt.Println(err)
			}
		}
		SlaveMapMutex.Unlock()

		fmt.Printf("Slave connected from %s assigned ID: %d\n", host, id)

		receive := make(chan string, 10)
		orderChan := make(chan [][4][3]bool, 10)

		SlaveMapMutex.Lock()
		SlaveOrderChans[id] = orderChan
		SlaveMapMutex.Unlock()

		// Reading from the connection
		go HandleConnections(conn, receive, id, orderChan, host)

		// Forward data read from the connection to 'receiver'
		go func(slaveID int32) {
			for data := range receive {
				receiver <- data + strconv.Itoa(int(slaveID))
			}
		}(id)
	}
}

// HandleConnections reads raw data from a slave, awaits a master order, and sends response
func HandleConnections(
	conn *kcp.UDPSession,
	receive chan<- string,
	id int32,
	orderChan <-chan [][4][3]bool,
	host string,
) {
	defer conn.Close()
	buffer := make([]byte, 1024)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				log.Printf("Slave Disconnected")
			} else {
				log.Printf("Read error from Slave: %v", err)
			}
		}
		var data string
		for _, b := range buffer[:n] {
			data += fmt.Sprintf("%08b", b)
		}
		receive <- data

		// Default response
		response := "n"
		response2 := []byte(response)

		// Wait up to 100ms for an order
		select {
		case masterOrder := <-orderChan:
			if len(masterOrder) > int(id) && len(masterOrder[id]) > 0 {
				otherRequests := masterOrder[:id]
				otherRequests = append(otherRequests, masterOrder[id+1:]...)
				otherRequest := elevator.MergeRequestsSlice(otherRequests)
				response2 = []byte(
					elevator.EncodeMatrixToString(masterOrder[id]) +
						elevator.EncodeMatrixToString(otherRequest))
			}
		case <-time.After(100 * time.Millisecond):
			log.Printf("No master order available for slave %d, sending default response.\n", id)
		}

		if rand.Intn(100) < 20 {
			_, err = conn.Write([]byte(response2))
			if err != nil {
				log.Printf("Failed to send response to Slave", err)
			}
		} else {
			fmt.Println("Packet loss oh no :o")
		}
		if err != nil {
			log.Printf("Failed to send response to Slave: %v", err)
		}
	}
}

// SendToMaster tries to connect to a master on port 4000 to send local elevator state
func SendToMaster(
	EL *elevator.Elevator,
	receiver chan<- string,
	_ELS *elevator.ElevatorList,
) {
	conn, err := kcp.DialWithOptions(ipList[MasterIndex]+":4000", nil, 10, 3)
	if err != nil {
		log.Fatalf("Failed to connect to master: %v", err)
	}
	defer conn.Close()

	fmt.Println("Connected to Master!")

	failureCount := 0
	maxFailures := 100                     // Number of consecutive failures before triggering MasterCheck
	failureTimeout := 1 * time.Millisecond // Time window to count failures
	lastFailureTime := time.Now()

	for {
		EL.PrintElevatorState()
		packet := EncodeElevator(EL)

		if rand.Intn(100) > packetLoss {
			_, err := conn.Write(packet[:])
			if err != nil {
				log.Println("Failed to send data:", err)
				return
			}
		} else {
			fmt.Println("Packet loss oh no :o")
		}
		fmt.Println("Sent to Master:", packet)

		// Read response from Master
		buffer := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buffer)

		if err != nil {
			fmt.Println("Failed to read response:", err)

			// Increment failure count and check timeout window
			if time.Since(lastFailureTime) > failureTimeout {
				failureCount = 0 // Reset failure count if timeout window passed
			}
			failureCount++
			lastFailureTime = time.Now()

			if failureCount >= maxFailures {
				fmt.Println("Connection issue persists, triggering MasterCheck.")
				MasterCheck(rand.Intn(3000)+1500, _ELS, EL)
				ActiveConnection = false
				return
			}
		} else {
			// Reset failure count on successful read
			failureCount = 0
			receiver <- string(buffer[:n])
			ActiveConnection = true
		}
		receiver <- string(buffer[:n])
		ActiveConnection = true

		time.Sleep(1 * time.Second)
	}
}

func EncodeElevator(e *elevator.Elevator) [3]byte {
	var b0, b1, b2 byte
	req := elevator.GetRequests(*e)

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
	b2 |= byte(elevator.GetBehavior(*e) & 0b11)
	b2 |= byte((elevator.GetDirection(*e)+1)&0b11) << 2
	b2 |= byte(elevator.GetFloor(*e)&0b11) << 4

	return [3]byte{b0, b1, b2}
}

func DecodeElevator(data [3]byte) elevator.Elevator {
	var e elevator.Elevator
	b0, b1, b2 := data[0], data[1], data[2]

	// --- Byte 0: floors 0..1 ---
	e.SetIndRequest((b0&(1<<0)) != 0, 0, 0)
	e.SetIndRequest((b0&(1<<1)) != 0, 0, 1)
	e.SetIndRequest((b0&(1<<2)) != 0, 0, 2)
	e.SetIndRequest((b0&(1<<3)) != 0, 1, 0)
	e.SetIndRequest((b0&(1<<4)) != 0, 1, 1)
	e.SetIndRequest((b0&(1<<5)) != 0, 1, 2)

	// --- Byte 1: floors 2..3 ---
	e.SetIndRequest((b1&(1<<0)) != 0, 2, 0)
	e.SetIndRequest((b1&(1<<1)) != 0, 2, 1)
	e.SetIndRequest((b1&(1<<2)) != 0, 2, 2)
	e.SetIndRequest((b1&(1<<3)) != 0, 3, 0)
	e.SetIndRequest((b1&(1<<4)) != 0, 3, 1)
	e.SetIndRequest((b1&(1<<5)) != 0, 3, 2)

	// --- Byte 2: elevator state ---
	e.SetBehavior(elevator.ElevatorBehavior(b2 & 0b11))
	e.SetDirection(elevio.MotorDirection(((b2 >> 2) & 0b11) - 1))
	e.SetFloor(int((b2 >> 4) & 0b11))

	return e
}

func DecodeElevatorFromString(s string) elevator.Elevator {
	if len(s) != 24 {
		fmt.Println("DecodeElevatorFromString: invalid length:", len(s))
		return elevator.Elevator{}
	}
	b0, _ := strconv.ParseUint(s[0:8], 2, 8)
	b1, _ := strconv.ParseUint(s[8:16], 2, 8)
	b2, _ := strconv.ParseUint(s[16:24], 2, 8)

	return DecodeElevator([3]byte{byte(b0), byte(b1), byte(b2)})
}

// BoolToInt is a helper for bit manipulation
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func StringCabToRequest(bin string) [4][3]bool {
	var result [4][3]bool

	// Bits 0–3 → [3][2] to [0][2]
	for i := 0; i < 4; i++ {
		bit := bin[7-i] // rightmost bit is index 7
		result[i][2] = bit == '1'
	}

	return result
}
