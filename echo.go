package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"
)

func main() {

	if len(os.Args) > 1 {
		SimServerPort = string("localhost:" + os.Args[1]) // First argument after "go run ."
	}

	EL1 := elevator{
		id:       0,
		floor:    0,
		dirn:     0,
		behavior: 0,
		request:  make([][]int, numFloors),
		peers:    []string{},
	}

	for i := range ELS {
		ELS[i].request = [][]int{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}}
	}

	for i := range EL1.request {
		EL1.request[i] = make([]int, 3)
	}

	Master = false
	masterTimer := rand.Intn(1500) + 300
	MasterCheck(masterTimer, EL1)

	Init(SimServerPort, numFloors)

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

	for {
		if Master {
			select {
			case a := <-Read:
				//fmt.Println(a)
				slaveID := int(a[16] - '0')
				ELS[0] = EL1
				ELS[slaveID] = MakeElevator(a)
				order := MakeRequest(ELS, numFloors)
				fmt.Println("Updated MasterOrders:", order)
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
			default:
			}

		} else {
			select {
			case a := <-Send:
				fmt.Println(a)
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
			default:
			}
		}
	}
}
