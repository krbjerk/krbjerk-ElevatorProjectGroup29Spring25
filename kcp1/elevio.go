package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"sync"
	"time"

	"github.com/xtaci/kcp-go/v5"
)

const _pollRate = 20 * time.Millisecond

var _initialized bool = false
var _numFloors int = 4
var _mtx sync.Mutex
var _conn net.Conn

type MotorDirection int

const (
	MD_Stop MotorDirection = 0
	MD_Up   MotorDirection = 1
	MD_Down MotorDirection = -1
)

type ButtonType int

const (
	BT_HallUp   ButtonType = 0
	BT_HallDown ButtonType = 1
	BT_Cab      ButtonType = 2
)

type ElevatorBehavior int

var FloorTimer = 2
var numFloors = 4
var Master bool

type elevator struct {
	id       int
	floor    int
	dirn     MotorDirection
	request  [][]int
	behavior ElevatorBehavior
	peers    []string
}

const (
	EB_idle ElevatorBehavior = iota
	EB_Moving
	EB_DoorOpen
)

type ButtonEvent struct {
	Floor  int
	Button ButtonType
}

func MasterCheck(EL elevator) {
	for _, peer := range EL.peers {
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

func TakeRequest(EL elevator) {
	for i := range EL.request {
		if EL.request[i][2] == 1 {
			if EL.floor > i {
				SetMotorDirection(MD_Down)
				EL.dirn = MD_Down
				return
			}
			if EL.floor < i {
				SetMotorDirection(MD_Up)
				EL.dirn = MD_Up
				return
			}
		}
	}
}

func MakeRequest(ELS []elevator) [][]int {
	var EL_requests = make([][]int, 3)
	var Finished_EL_requests = make([][]int, 3)
	EL_requests = [][]int{{0, 0, 0, 0, 0, 0, 0, 0}, {0, 0, 0, 0, 0, 0, 0, 0}, {0, 0, 0, 0, 0, 0, 0, 0}}
	var Time_Between_floors = 5
	for i := range ELS {
		for j := range ELS {
			for ii := 0; ii < numFloors; ii++ {
				if ELS[j].request[ii][0] == 1 {
					EL_requests[i][ii*2] = int(math.Abs(float64(ELS[i].floor)-float64(ii)))*Time_Between_floors + 2*int(ELS[i].dirn)*-int(math.Pow(float64(int(ELS[i].floor)-int(ii)), 0)) + int(ELS[i].behavior)
				}
				if ELS[j].request[ii][1] == 1 {
					EL_requests[i][ii*2+1] = int(math.Abs(float64(ELS[i].floor)-float64(ii)))*Time_Between_floors - 2*int(ELS[i].dirn)*-int(math.Pow(float64(int(ELS[i].floor)-int(ii)), 0)) + int(ELS[i].behavior)
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

func ReadFromSlave(receiver chan<- string) {
	listener, err := kcp.ListenWithOptions(":4000", nil, 10, 3)
	if err != nil {
		log.Fatalf("Failed to start KCP server: %v", err)
	}
	defer listener.Close()
	fmt.Println("KCP Master (Server) listening on port 4000...")

	for {
		// Accept a new connection
		conn, err := listener.AcceptKCP()
		if err != nil {
			fmt.Printf("Error accepting connection: %v", err)
			continue
		}
		fmt.Println("Slave connected!")

		// Handle the connection in a separate goroutine
		receive := make(chan string) // Buffer size to avoid blocking
		go HandleConnections(conn, receive)

		for data := range receive { // Continuously process messages
			receiver <- data
		}
	}
}

func HandleConnections(conn *kcp.UDPSession, receive chan<- string) {
	defer conn.Close()
	buffer := make([]byte, 1024)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				log.Println("Connection closed by client")
				return
			}
			log.Println("Read error:", err)
			continue // Don't exit on minor errors
		}

		var data string = ""
		for _, b := range buffer[:n] {
			data += fmt.Sprintf("%08b", b)
		}
		receive <- data

		// Respond to client
		response := "Hello from Master!"
		_, err = conn.Write([]byte(response))
		if err != nil {
			log.Println("Failed to send response:", err)
			return
		}
		log.Println("Sending response to Slave: Hello from Master!")
	}
}

func SendToMaster(receiver chan<- string, EL elevator) {
	conn, err := kcp.DialWithOptions("10.22.113.44:4000", nil, 10, 3)
	if err != nil {
		log.Fatalf("Failed to connect to master: %v", err)
	}
	defer conn.Close()

	fmt.Println("Connected to Master!")

	for {
		var package1 uint8 = uint8(EL.request[0][2]&0b1 | EL.request[0][0]&0b1<<1 | int(EL.behavior)&0b11<<2 | int(EL.dirn+1)&0b11<<4 | int(EL.floor)&0b11<<6)
		var package2 uint8 = uint8(EL.request[3][2]&0b1 | EL.request[3][0]&0b1<<1 | EL.request[2][2]&0b1<<2 | EL.request[2][1]&0b1<<3 | EL.request[2][0]&0b1<<4 | EL.request[1][2]&0b1<<5 | EL.request[1][1]&0b1<<6 | EL.request[1][0]&0b1<<7)

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
			continue
		}
		receiver <- string(buffer[:n])

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

func SetMotorDirection(dir MotorDirection) {
	write([4]byte{1, byte(dir), 0, 0})
}

func SetButtonLamp(button ButtonType, floor int, value bool) {
	write([4]byte{2, byte(button), byte(floor), toByte(value)})
}

func SetFloorIndicator(floor int) {
	write([4]byte{3, byte(floor), 0, 0})
}

func SetDoorOpenLamp(value bool) {
	write([4]byte{4, toByte(value), 0, 0})
}

func SetStopLamp(value bool) {
	write([4]byte{5, toByte(value), 0, 0})
}

func PollButtons(receiver chan<- ButtonEvent) {
	prev := make([][3]bool, _numFloors)
	for {
		time.Sleep(_pollRate)
		for f := 0; f < _numFloors; f++ {
			for b := ButtonType(0); b < 3; b++ {
				v := GetButton(b, f)
				if v != prev[f][b] && !v {
					receiver <- ButtonEvent{f, ButtonType(b)}
				}
				prev[f][b] = v
			}
		}
	}
}

func PollFloorSensor(receiver chan<- int) {
	prev := -1
	for {
		time.Sleep(_pollRate)
		v := GetFloor()
		if v != prev && v != -1 {
			receiver <- v
		}
		prev = v
	}
}

func PollStopButton(receiver chan<- bool) {
	prev := false
	for {
		time.Sleep(_pollRate)
		v := GetStop()
		if v != prev {
			receiver <- v
		}
		prev = v
	}
}

func PollObstructionSwitch(receiver chan<- bool) {
	prev := false
	for {
		time.Sleep(_pollRate)
		v := GetObstruction()
		if v != prev {
			receiver <- v
		}
		prev = v
	}
}

func GetButton(button ButtonType, floor int) bool {
	a := read([4]byte{6, byte(button), byte(floor), 0})
	return toBool(a[1])
}

func GetFloor() int {
	a := read([4]byte{7, 0, 0, 0})
	if a[1] != 0 {
		return int(a[2])
	} else {
		return -1
	}
}

func GetStop() bool {
	a := read([4]byte{8, 0, 0, 0})
	return toBool(a[1])
}

func GetObstruction() bool {
	a := read([4]byte{9, 0, 0, 0})
	return toBool(a[1])
}

func read(in [4]byte) [4]byte {
	_mtx.Lock()
	defer _mtx.Unlock()

	_, err := _conn.Write(in[:])
	if err != nil {
		panic("Lost connection to Elevator Server")
	}

	var out [4]byte
	_, err = _conn.Read(out[:])
	if err != nil {
		panic("Lost connection to Elevator Server")
	}

	return out
}

func write(in [4]byte) {
	_mtx.Lock()
	defer _mtx.Unlock()

	_, err := _conn.Write(in[:])
	if err != nil {
		panic("Lost connection to Elevator Server")
	}
}

func toByte(a bool) byte {
	var b byte = 0
	if a {
		b = 1
	}
	return b
}

func toBool(a byte) bool {
	var b bool = false
	if a != 0 {
		b = true
	}
	return b
}
