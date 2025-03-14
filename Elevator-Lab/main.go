package main

import (
	"Driver-go/elevio"
	"fmt"
	"time"
)

func main() {

	elevio.Init("localhost:15657", NUM_FLOORS)

	if elevio.GetFloor() == -1 {
		// Make the elevator move to an actual floor on startup. Necessary for the state machine.
		g_elevator.initElevator()
	}

	drv_buttons := make(chan elevio.ButtonEvent)
	drv_floors := make(chan int)
	drv_obstr := make(chan bool)
	drv_stop := make(chan bool)

	go elevio.PollButtons(drv_buttons)
	go elevio.PollFloorSensor(drv_floors)
	go elevio.PollObstructionSwitch(drv_obstr)
	go elevio.PollStopButton(drv_stop)

	// Create a ticker that triggers every 500ms to check the timer
	timeoutTicker := time.NewTicker(500 * time.Millisecond)

	for {
		select {
		case a := <-drv_buttons:
			fmt.Printf("%+v\n", a)
			elevio.SetButtonLamp(a.Button, a.Floor, true)

			g_elevator.handleButtonPress(a.Floor, a.Button)

		case a := <-drv_floors:
			fmt.Printf("%+v\n", a)
			g_elevator.handleFloorArrival(a)

		case a := <-drv_obstr:
			g_elevator.setObstruction(a)

		case <-drv_stop:

		case <-timeoutTicker.C:
			if g_timer.timedOut() {
				fmt.Println("Timed out in main.")
				g_elevator.handleDoorTimeout()
			}
		}
	}
}
