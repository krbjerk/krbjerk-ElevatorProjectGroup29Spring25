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

var slaveOrderChans = make(map[int32]chan [][]int)

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

func MakeRequest(ELS []Elevator) [][]int {
	var EL_requests = make([][]int, 3)
	var Finished_EL_requests = make([][]int, 3)
	EL_requests = [][]int{{0, 0, 0, 0, 0, 0, 0, 0}, {0, 0, 0, 0, 0, 0, 0, 0}, {0, 0, 0, 0, 0, 0, 0, 0}}
	var Time_Between_floors = 5
	for i := range ELS {
		for j := range ELS {
			for ii := 0; ii < numFloors; ii++ {
				if ELS[j].m_requests[ii][0] {
					EL_requests[i][ii*2] = int(math.Abs(float64(ELS[i].m_floor)-float64(ii)))*Time_Between_floors + 2*int(ELS[i].m_dirn)*-int(math.Pow(float64(int(ELS[i].m_floor)-int(ii)), 0)) + int(ELS[i].m_behavior)
				}
				if ELS[j].m_requests[ii][1] {
					EL_requests[i][ii*2+1] = int(math.Abs(float64(ELS[i].m_floor)-float64(ii)))*Time_Between_floors - 2*int(ELS[i].m_dirn)*-int(math.Pow(float64(int(ELS[i].m_floor)-int(ii)), 0)) + int(ELS[i].m_behavior)
				}
			}
		}
	}
	fmt.Printf("Requests: %+v", EL_requests)
	var while_v = 0
	for while_v < 1 {
		var lowest = 100
		var index = 0
		var floor = 0
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
		}
		if lowest != 100 {
			Finished_EL_requests[index] = append(Finished_EL_requests[index], floor)
			EL_requests[0][floor] = 0
			EL_requests[1][floor] = 0
			EL_requests[2][floor] = 0
			for i := range EL_requests[index] {
				if EL_requests[index][i] != 0 {
					EL_requests[index][i] = EL_requests[index][i] + 3
					if floor%2 == 0 {
						if i > floor {
							EL_requests[index][i] = EL_requests[index][i] - 5
						}
					}
					if floor%2 != 0 {
						if i < floor {
							EL_requests[index][i] = EL_requests[index][i] - 5
						}
					}
				}
			}
		}
	}
	return Finished_EL_requests
}
func MakeElevator(a string) (b Elevator) {
	//[00000000000000000]"01"=floor"23"=dir"45"behavior"6-15"request"16"id
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
	fmt.Println("KCP Master (Server) listening on port 4000...")

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
		orderChan := make(chan [][]int, 10)

		// Associate the order channel with the slave ID
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
func HandleConnections(conn *kcp.UDPSession, receive chan<- string, id int32, orderChan <-chan [][]int, host string) {
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

		response := "0"
		select {
		case masterOrder := <-orderChan:
			if len(masterOrder) > int(id) && len(masterOrder[id]) > 0 {
				response = strconv.Itoa(masterOrder[id][0])
			}
		case <-time.After(100 * time.Millisecond):
			log.Printf("No master order available for Slave %d, sending default response.\n", id)
		}

		_, err = conn.Write([]byte(response))
		if err != nil {
			log.Printf("Failed to send response to Slave %d: %v\n", id, err)
			return
		}
	}
}

func SendToMaster(receiver chan<- string, EL Elevator) {
	conn, err := kcp.DialWithOptions("10.22.168.192:4001", nil, 10, 3)
	if err != nil {
		log.Fatalf("Failed to connect to master: %v", err)
	}
	defer conn.Close()

	fmt.Println("Connected to Master!")

	for {
		var package1 uint8 = uint8(BoolToInt(EL.m_requests[0][2])&0b1 | BoolToInt(EL.m_requests[0][0])&0b1<<1 | int(EL.m_behavior)&0b11<<2 | int(EL.m_dirn+1)&0b11<<4 | int(EL.m_floor)&0b11<<6)
		var package2 uint8 = uint8(BoolToInt(EL.m_requests[3][2])&0b1 | BoolToInt(EL.m_requests[3][0])&0b1<<1 | BoolToInt(EL.m_requests[2][2])&0b1<<2 | BoolToInt(EL.m_requests[2][1])&0b1<<3 | BoolToInt(EL.m_requests[2][0])&0b1<<4 | BoolToInt(EL.m_requests[1][2])&0b1<<5 | BoolToInt(EL.m_requests[1][1])&0b1<<6 | BoolToInt(EL.m_requests[1][0])&0b1<<7)

		_, err := conn.Write([]byte{package1, package2})
		if err != nil {
			log.Println("Failed to send data:", err)
			return
		}
		fmt.Println("Sent to Master:", []byte{package1, package2})

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
