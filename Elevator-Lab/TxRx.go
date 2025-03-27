package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"root/elevio"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtaci/kcp-go/v5"
)

const _pollRate = 20 * time.Millisecond

var slaveOrderChans = make(map[int32]chan [][4][3]bool)

var _initialized bool = false
var _numFloors int = 4
var _mtx sync.Mutex
var _conn net.Conn

var ActiveConnection bool = false
var FloorTimer = 2
var numFloors = 4
var Master bool

type ButtonEvent struct {
	Floor  int
	Button Button
}

func MasterCheck(EL Elevator) {
	for _, peer := range EL.m_peers {
		conn, err := kcp.DialWithOptions(peer, nil, 10, 3)
		if err == nil {
			fmt.Printf("Connected to higher priority node: %s. Staying slave.\n", peer)
			conn.Close()
			Master = false
			return
		}
	}
	fmt.Println("No higher priority nodes available. Becoming master.")
	Master = true
}

func TakeRequest(EL Elevator, request string) {
	value, err := strconv.Atoi(request)
	if err != nil {
		// Handle the error appropriately
		fmt.Println("Error converting string to int:", err)
		return
	}
	if value%2 == 0 {
		elevio.SetMotorDirection(elevio.MD_Up)
		EL.m_dirn = elevio.MD_Up
	}
}

/*
func MakeRequest(ELS []Elevator) [3][4][3]bool {
	var EL_requests = [][]int{
		make([]int, numFloors*2),
		make([]int, numFloors*2),
		make([]int, numFloors*2),
	}
	var Finished_EL_requests = make([][]int, 3)
	var Time_Between_floors = 5

	for i := range ELS {
		for j := range ELS {
			for ii := 0; ii < numFloors; ii++ {
				if ELS[j].m_requests[ii][0] {
					EL_requests[i][ii*2] = int(math.Abs(float64(ELS[i].m_floor)-float64(ii)))*Time_Between_floors +
						2*int(ELS[i].m_dirn)*-int(math.Pow(float64(int(ELS[i].m_floor)-int(ii)), 0)) +
						int(ELS[i].m_behavior)
				}
				if ELS[j].m_requests[ii][1] {
					EL_requests[i][ii*2+1] = int(math.Abs(float64(ELS[i].m_floor)-float64(ii)))*Time_Between_floors -
						2*int(ELS[i].m_dirn)*-int(math.Pow(float64(int(ELS[i].m_floor)-int(ii)), 0)) +
						int(ELS[i].m_behavior)
				}
			}
		}
	}

	var while_v = 0
	for while_v < 1 {
		lowest := 100
		index := 0
		floor := 0

		for i := range EL_requests {
			for j := range EL_requests[i] {
				if EL_requests[i][j] < lowest && EL_requests[i][j] != 0 {
					lowest = EL_requests[i][j]
					index = i
					floor = j
				}
			}
		}

		if lowest == 100 {
			while_v = 1
		} else {
			Finished_EL_requests[index] = append(Finished_EL_requests[index], floor)
			EL_requests[0][floor] = 0
			EL_requests[1][floor] = 0
			EL_requests[2][floor] = 0

			for i := range EL_requests[index] {
				if EL_requests[index][i] != 0 {
					EL_requests[index][i] += 3
					if floor%2 == 0 && i > floor {
						EL_requests[index][i] -= 5
					}
					if floor%2 != 0 && i < floor {
						EL_requests[index][i] -= 5
					}
				}
			}
		}
	}

	// Final assignment into a [3][4][3]bool result matrix
	var result [3][4][3]bool

	for elIndex, requests := range Finished_EL_requests {
		for _, flatFloor := range requests {
			floor := flatFloor / 2
			btnType := flatFloor % 2 // 0 = up, 1 = down
			if btnType == 0 || btnType == 1 {
				result[elIndex][floor][btnType] = true
			}
			// do NOT touch cab buttons (index 2)
		}
	}

	return result
}

*/

// MakeRequest dynamically assigns hall requests among all connected elevators.
func MakeRequest(ELS []Elevator) [][NUM_FLOORS][3]bool {
	n := len(ELS)
	if n == 0 {
		// No elevators
		return nil
	}

	// cost array: EL_requests[i][0..7] for each elevator i (if numFloors=4 => 4 floors×2 btn=8)
	EL_requests := make([][]int, n)
	for i := 0; i < n; i++ {
		EL_requests[i] = make([]int, numFloors*2)
	}

	// Track final assigned requests per elevator
	Finished_EL_requests := make([][]int, n)

	Time_Between_floors := 5

	// 1) Build cost matrix
	for i := 0; i < n; i++ { // the elevator we might assign to
		for j := 0; j < n; j++ { // the elevator that *has* requests
			for floor := 0; floor < numFloors; floor++ {
				// up request?
				if ELS[j].m_requests[floor][0] {
					cost := int(math.Abs(float64(ELS[i].m_floor)-float64(floor))) * Time_Between_floors
					// cast dirn/behavior to int
					cost += 2 * int(ELS[i].m_dirn) * -int(math.Pow(float64(ELS[i].m_floor-floor), 0))
					cost += int(ELS[i].m_behavior)
					EL_requests[i][floor*2] = cost
				}
				// down request?
				if ELS[j].m_requests[floor][1] {
					cost := int(math.Abs(float64(ELS[i].m_floor)-float64(floor))) * Time_Between_floors
					cost -= 2 * int(ELS[i].m_dirn) * -int(math.Pow(float64(ELS[i].m_floor-floor), 0))
					cost += int(ELS[i].m_behavior)
					EL_requests[i][floor*2+1] = cost
				}
			}
		}
	}

	// 2) Repeatedly pick lowest cost request
	for {
		lowest := 100
		chosenElevator := -1
		chosenRequest := -1

		// Find absolute lowest cost across all
		for i := 0; i < n; i++ {
			for r := 0; r < numFloors*2; r++ {
				c := EL_requests[i][r]
				if c != 0 && c < lowest {
					lowest = c
					chosenElevator = i
					chosenRequest = r
				}
			}
		}

		if lowest == 100 || chosenElevator < 0 {
			break // no more requests
		}

		// assign it
		Finished_EL_requests[chosenElevator] = append(
			Finished_EL_requests[chosenElevator], chosenRequest,
		)

		// clear that request from all elevators
		for i := 0; i < n; i++ {
			EL_requests[i][chosenRequest] = 0
		}

		// Increase cost of remaining requests for chosen elevator
		for r := 0; r < numFloors*2; r++ {
			if EL_requests[chosenElevator][r] != 0 {
				EL_requests[chosenElevator][r] += 3
				// If the assigned was "up" (even index)
				if chosenRequest%2 == 0 && r > chosenRequest {
					EL_requests[chosenElevator][r] -= 5
				}
				// If the assigned was "down" (odd index)
				if chosenRequest%2 == 1 && r < chosenRequest {
					EL_requests[chosenElevator][r] -= 5
				}
			}
		}
	}

	// 3) Convert to a slice of [numFloors][3]bool
	result := make([][NUM_FLOORS][3]bool, n)

	for elIndex, assigned := range Finished_EL_requests {
		for _, flatFloor := range assigned {
			floor := flatFloor / 2
			btnType := flatFloor % 2 // 0=up, 1=down
			// skip out-of-bounds / cabin index2
			if floor >= 0 && floor < numFloors && (btnType == 0 || btnType == 1) {
				result[elIndex][floor][btnType] = true
			}
		}
	}

	return result
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

func MakeElevator(a string) (b Elevator) {
	fmt.Println(a)
	//[00010 0000 0000 0000]"01"=floor"23"=dir"45"behavior"6-15"request"16"id
	EL := Elevator{
		m_id:       int(a[16]) - '0',
		m_floor:    (int(a[0])-'0')*2 + (int(a[1]) - '0'),
		m_dirn:     elevio.MotorDirection((int(a[2])-'0')*2 + (int(a[3]) - '0') - 1),
		m_behavior: ElevatorBehavior((int(a[5])-'0')*2 + (int(a[6]) - '0')),
		// Problem. Amount of floors is hard coded.
		m_requests: [4][3]bool{{a[6] == '1', false, a[7] == '1'}, {a[8] == '1', a[9] == '1', a[10] == '1'}, {a[11] == '1', a[12] == '1', a[13] == '1'}, {false, a[14] == '1', a[15] == '1'}},
		m_peers:    []string{},
	}
	return EL
}

// TROUBLESHOOTING:
/*func MakeElevator(a string) (b Elevator) {
	fmt.Println(a)
	// Adjusted bit indices for correct extraction
	EL := Elevator{
		m_id:       int(a[16]) - '0',
		m_floor:    (int(a[3])-'0')*2 + (int(a[4]) - '0'), // Corrected indices
		m_dirn:     elevio.MotorDirection((int(a[2])-'0')*2 + (int(a[3]) - '0') - 1),
		m_behavior: ElevatorBehavior((int(a[5])-'0')*2 + (int(a[6]) - '0')),
		m_requests: [4][3]bool{
			{a[6] == '1', false, a[7] == '1'},
			{a[8] == '1', a[9] == '1', a[10] == '1'},
			{a[11] == '1', a[12] == '1', a[13] == '1'},
			{false, a[14] == '1', a[15] == '1'},
		},
		m_peers: []string{},
	}
	return EL
}*/

var (
	slaveMap      = make(map[string]int32)
	slaveMapMutex sync.Mutex
	IDCounter     int32 = 0
)

// ReadFromSlave accepts connections from slaves.
func ReadFromSlave(receiver chan<- string) {
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
		// Use only the host (IP) part of the address
		addr := conn.RemoteAddr().String()
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr // Fallback if splitting fails
		}

		// Look up existing ID for this host, or assign a new one.
		slaveMapMutex.Lock()
		id, exists := slaveMap[host]
		if !exists {
			id = atomic.AddInt32(&IDCounter, 1)
			slaveMap[host] = id
		}
		slaveMapMutex.Unlock()

		fmt.Printf("Slave %d connected from %s!\n", id, host)

		receive := make(chan string, 10)
		orderChan := make(chan [][4][3]bool, 10)

		// Associate the order channel with the slave ID
		slaveMapMutex.Lock()
		slaveOrderChans[id] = orderChan
		slaveMapMutex.Unlock()

		go HandleConnections(conn, receive, id, orderChan, host)
		go func(slaveID int32) {

			for data := range receive {
				//encoded := fmt.Sprintf("%08b%08b%08b", data[0], data[1], data[2])
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
				log.Printf("Slave %d (%s) disconnected\n", id, host)
			} else {
				log.Printf("Read error from Slave %d: %v\n", id, err)
			}
			return
		}

		var data string
		for _, b := range buffer[:n] {
			data += fmt.Sprintf("%08b", b)
		}
		receive <- data

		response := "n"
		response2 := []byte(response)

		select {
		case masterOrder := <-orderChan:
			fmt.Println("LEN MASTER")
			fmt.Println(len(masterOrder))
			fmt.Println(int(id))
			fmt.Println(masterOrder)
			if len(masterOrder) > int(id) && len(masterOrder[id]) > 0 {
				fmt.Println("if")
				response2 = []byte(EncodeMatrixToString(masterOrder[id]))
				fmt.Println("If-sentence")
			}
		case <-time.After(100 * time.Millisecond):
			log.Printf("No master order available for Slave %d, sending default response.\n", id)
		}

		_, err = conn.Write(response2)
		if err != nil {
			log.Printf("Failed to send response to Slave %d: %v\n", id, err)
			return
		}
	}
}

func (EL *Elevator) SendToMaster(receiver chan<- string) {
	conn, err := kcp.DialWithOptions("192.168.0.176:4001", nil, 10, 3)
	if err != nil {
		log.Fatalf("Failed to connect to master: %v", err)
	}
	defer conn.Close()

	fmt.Println("Connected to Master!")

	for {
		//EL.printElevatorState()
		//var package1 uint8 = uint8(BoolToInt(EL.m_requests[0][2])&0b1 | BoolToInt(EL.m_requests[0][0])&0b1<<1 | int(EL.m_behavior)&0b11<<2 | int(EL.m_dirn+1)&0b11<<4 | int(EL.m_floor)&0b11<<6)
		//var package2 uint8 = uint8(BoolToInt(EL.m_requests[3][2])&0b1 | BoolToInt(EL.m_requests[3][0])&0b1<<1 | BoolToInt(EL.m_requests[2][2])&0b1<<2 | BoolToInt(EL.m_requests[2][1])&0b1<<3 | BoolToInt(EL.m_requests[2][0])&0b1<<4 | BoolToInt(EL.m_requests[1][2])&0b1<<5 | BoolToInt(EL.m_requests[1][1])&0b1<<6 | BoolToInt(EL.m_requests[1][0])&0b1<<7)
		fmt.Println("Rett for")
		EL.printElevatorState()
		packet := EncodeElevator(EL)
		fmt.Println()
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
			ActiveConnection = false
			continue
		}
		receiver <- string(buffer[:n])
		ActiveConnection = true

		time.Sleep(2 * time.Second)
	}
}

func Init(addr string, numFloors int) {
	if _initialized {
		fmt.Println("Driver already initialized!")
		return
	}
	_numFloors = numFloors
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
