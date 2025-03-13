package main

import (
	"fmt"
	"time"
)

func main() {

	var Master bool = false

	EL1 := elevator{
		id:       0,
		floor:    0,
		dirn:     0,
		behavior: 0,
		request:  make([][]int, numFloors),
		peers:    []string{},
	}

	var ELS []elevator = make([]elevator, 3)
	for i := range ELS {
		ELS[i].request = [][]int{{0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}}
	}

	for i := range EL1.request {
		EL1.request[i] = make([]int, 3)
	}

	Init("localhost:12345", numFloors)

	var d MotorDirection = MD_Stop
	SetMotorDirection(d)

	drv_buttons := make(chan ButtonEvent)
	drv_floors := make(chan int)
	drv_obstr := make(chan bool)
	drv_stop := make(chan bool)

	go PollButtons(drv_buttons)
	go PollFloorSensor(drv_floors)
	go PollObstructionSwitch(drv_obstr)
	go PollStopButton(drv_stop)

	if Master {
		Read := make(chan string)
		go ReadFromSlave(Read)

		for {
			select {
			case a := <-Read:
				// Assuming the slave ID is encoded in a specific position in the message
				slaveID := int(a[16] - '0') // Adjust this as needed
				ELS[0] = EL1
				ELS[slaveID] = MakeElevator(a)
				order := MakeRequest(ELS)
				fmt.Println("Updated MasterOrders:", order)

				// Send the order to the specific slave's dedicated channel
				slaveMapMutex.Lock()
				ch, ok := slaveOrderChans[int32(slaveID)]
				slaveMapMutex.Unlock()
				if ok {
					select {
					case ch <- order:
					default:
						fmt.Printf("Slave %d's order channel is full; skipping update.\n", slaveID)
					}
				}
			case a := <-drv_buttons:
				fmt.Printf("%+v\n", a)
				EL1.request[a.Floor][a.Button] = 1
				SetButtonLamp(a.Button, a.Floor, true)

			case a := <-drv_floors:
				fmt.Printf("%+v\n", a)
				EL1.floor = a
				if EL1.request[a][2] == 1 {
					EL1.request[a][2] = 0
					SetMotorDirection(MD_Stop)
					SetButtonLamp(2, a, false)
					time.Sleep(time.Duration(FloorTimer) * time.Second)
				}

			case a := <-drv_obstr:
				fmt.Printf("%+v\n", a)
				if a {
					SetMotorDirection(MD_Stop)
				} else {
					SetMotorDirection(d)
				}

			case a := <-drv_stop:
				fmt.Printf("%+v\n", a)
				for f := 0; f < numFloors; f++ {
					for b := ButtonType(0); b < 3; b++ {
						SetButtonLamp(b, f, false)
					}
				}
			}
		}
	} else {
		Send := make(chan string)
		go SendToMaster(Send, EL1)
		for {
			select {
			case a := <-Send:
				fmt.Printf("%+v\n", a)
				TakeRequest(EL1, a)
			case a := <-drv_buttons:
				fmt.Printf("%+v\n", a)
				EL1.request[a.Floor][a.Button] = 1
				SetButtonLamp(a.Button, a.Floor, true)
			case a := <-drv_floors:
				fmt.Printf("%+v\n", a)
				EL1.floor = a
				if EL1.request[a][2] == 1 {
					EL1.request[a][2] = 0
					SetMotorDirection(MD_Stop)
					SetButtonLamp(2, a, false)
					time.Sleep(time.Duration(FloorTimer) * time.Second)
				}

			case a := <-drv_obstr:
				fmt.Printf("%+v\n", a)
				if a {
					SetMotorDirection(MD_Stop)
				} else {
					SetMotorDirection(d)
				}

			case a := <-drv_stop:
				fmt.Printf("%+v\n", a)
				for f := 0; f < numFloors; f++ {
					for b := ButtonType(0); b < 3; b++ {
						SetButtonLamp(b, f, false)
					}
				}
			}
		}
	}
}
