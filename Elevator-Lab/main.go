package main

import (
	"fmt"
	"root/elevio"
	"strconv"
	"time"
)

func main() {

	g_elevator := Elevator{
		m_id:       0,
		m_floor:    0,
		m_dirn:     0,
		m_behavior: 0,
		m_requests: [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}},
		m_peers:    []string{},
	}

	var Master bool = false

	var ELS []Elevator = make([]Elevator, 3)
	for i := range ELS {
		ELS[i].m_requests = [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}}
	}

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

	if Master {
		Read := make(chan string)
		go ReadFromSlave(Read)
		for {
			select {
			case a := <-Read:
				// -------------------------------------------------------------------------------------------------------------
				slaveID := int(a[16] - '0') // Adjust this as needed
				ELS[0] = g_elevator
				ELS[slaveID] = MakeElevator(a)
				order := MakeRequest(ELS) // WHAT WILL BE SENT TO SLAVE
				fmt.Println("0")
				slaveMapMutex.Lock()
				ch, ok := slaveOrderChans[int32(slaveID)]
				slaveMapMutex.Unlock()
				if ok {
					select {
					case ch <- order: // ACTUALLY SEND TO SLAVE
					default:
						fmt.Printf("Slave %d's order channel is full; skipping update.\n", slaveID)
					}
				}
				// -------------------------------------------------------------------------------------------------------------
			case a := <-drv_buttons:
				g_elevator.handleButtonPress(a.Floor, a.Button, true)
				/*if there is connection {
					ELS[0] = g_elevator
					order := MakeRequest(ELS)
					g_elevator.verifyRequest(ELS[0])
				}*/

			case a := <-drv_floors:
				g_elevator.handleFloorArrival(a)

			case a := <-drv_obstr:
				g_elevator.setObstruction(a)

			case <-drv_stop:

			case <-timeoutTicker.C:
				if g_timer.timedOut() {
					g_elevator.handleDoorTimeout()
				}
			}
		}
	} else {
		Send := make(chan string)
		go storedElevator.SendToMaster(Send /*storedElevator*/) // CONSTANTLY SENDING ITS OWN ELEVATOR | Here I have a suspicion that we can have problems reading and writing at the same time
		// TODO: MUST FIX SYNCHRONIZATION
		for {
			select {
			case a := <-Send:
				fmt.Println(a)
				// Function that will verify request and give them to the elevator, and from there also start elevator if necessary.
				if len(a) > 0 {
					b, _ := strconv.Atoi(a)

					g_elevator.verifyRequest(ConvertToElevatorRequests(b)) // WHAT WILL ACTUALLY BE SENT TO THE SLAVE FROM MASTER??
				}

			case a := <-drv_buttons:
				g_elevator.handleButtonPress(a.Floor, a.Button, ActiveConnection) // Also need paramter for connection. Necessary to differ between button-light contract and standalone

			case a := <-drv_floors:
				g_elevator.handleFloorArrival(a)

			case a := <-drv_obstr:
				g_elevator.setObstruction(a)

			case <-drv_stop:

			case <-timeoutTicker.C:
				if g_timer.timedOut() {
					g_elevator.handleDoorTimeout()
				}
			}
		}
	}
}
