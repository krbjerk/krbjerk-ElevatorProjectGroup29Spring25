package main

import (
	"fmt"
	"time"
)

func main() {

	Master = true

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
		MasterOrders := make(chan [][]int, 1)
		Read := make(chan string)
		go ReadFromSlave(Read, MasterOrders)
		for {
			select {
			case a := <-Read:
				ELS[0] = EL1
				ELS[int(a[16])-'0'] = MakeElevator(a)
				MasterOrders <- MakeRequest(ELS)
				MasterOrder := <-MasterOrders
				fmt.Println("Updated MasterOrders:", MasterOrder)
			case a := <-drv_buttons:
				fmt.Printf("%+v\n", a)
				EL1.request[a.Floor][a.Button] = 1
				SetButtonLamp(a.Button, a.Floor, true)
				TakeRequest(EL1)

			case a := <-drv_floors:
				fmt.Printf("%+v\n", a)
				EL1.floor = a
				if EL1.request[a][2] == 1 {
					EL1.request[a][2] = 0
					SetMotorDirection(MD_Stop)
					SetButtonLamp(2, a, false)
					time.Sleep(time.Duration(FloorTimer) * time.Second)
				}
				TakeRequest(EL1)

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
			case a := <-drv_buttons:
				fmt.Printf("%+v\n", a)
				EL1.request[a.Floor][a.Button] = 1
				SetButtonLamp(a.Button, a.Floor, true)
				TakeRequest(EL1)
			case a := <-drv_floors:
				fmt.Printf("%+v\n", a)
				EL1.floor = a
				if EL1.request[a][2] == 1 {
					EL1.request[a][2] = 0
					SetMotorDirection(MD_Stop)
					SetButtonLamp(2, a, false)
					time.Sleep(time.Duration(FloorTimer) * time.Second)
				}
				TakeRequest(EL1)

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
