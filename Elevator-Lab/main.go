package main

import (
	"fmt"
	"root/elevio"
	"time"
)

func main() {

	init_elevator := Elevator{
		m_id:       0,
		m_floor:    0,
		m_dirn:     0,
		m_behavior: 0,
		m_requests: [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}},
		m_peers:    []string{},
	}

	g_elevator := init_elevator
	var localOtherRequest [4][3]bool // TODO: Musyt define it at start. can be all false

	var Master bool = true

	var ELS []Elevator = make([]Elevator, 1)
	var storedELS []Elevator = make([]Elevator, 1)
	/*for i := range ELS {
		ELS[i].m_requests = [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}}
		storedELS[i].m_requests = [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}}
	}*/

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
				slaveID := int(a[24] - '0') // Adjust this as needed
				ELS[0] = g_elevator
				ELS[0].m_requests = storedElevator.m_requests

				if len(ELS) < slaveID+1 {
					ELS = append(ELS, init_elevator)
					storedELS = append(storedELS, init_elevator)
					fmt.Println("New elevator")
				}
				fmt.Println(a[:24])
				ELS[slaveID] = DecodeElevatorFromString(a[:24])
				ELS[slaveID].printElevatorState()
				fmt.Println(ELS[slaveID].m_requests)
				// ------
				// if new ELS != storedELS
				// 		then remove the overlapping requests from new ELS
				//		storedELS = new ELSz
				for k := 0; k < len(ELS); k++ {
					for i := 0; i < NUM_FLOORS; i++ {
						for j := 0; j < 3; j++ {
							if ELS[k].m_requests[i][j] && storedELS[k].m_requests[i][j] {
								ELS[k].m_requests[i][j] = false
								//storedELS[k].m_requests[i][j] = false // Assuming storedElevator.m_requests belongs to _e
							} else {
								storedELS[k].m_requests[i][j] = ELS[k].m_requests[i][j]
							}
						}
					}

				}
				// ------
				order := MakeRequest(ELS) // WHAT WILL BE SENT TO SLAVE
				fmt.Println("0")
				//fmt.Println(order)

				// ---
				otherRequests := order[:slaveID]
				otherRequests = append(otherRequests, order[slaveID+1:]...)
				otherRequest := MergeRequestsSlice(otherRequests)
				// TODO: Move this code into handleconnections
				// Send this list to the slaves and make a function that turns on and off the lights
				// based on the values in it

				localOtherRequests := order[1:]
				localOtherRequest = MergeRequestsSlice(localOtherRequests)

				// ---
				slaveMapMutex.Lock()
				ch, ok := slaveOrderChans[int32(slaveID)]
				slaveMapMutex.Unlock()
				if ok {
					select {
					case ch <- order: // ACTUALLY SEND TO SLAVE
						fmt.Println("In select, ch<-order", order)
						// We could change the elevators here so that it drops the request.
					default:
						fmt.Printf("Slave %d's order channel is full; skipping update.\n", slaveID)
					}
				}
				if len(order) > 0 {
					//g_elevator.verifyRequest(ConvertToElevatorRequests(order[0][0]))
					//g_elevator.m_requests = MergeRequests(g_elevator.m_requests, ConvertToElevatorRequests(order[0][0]))
					fmt.Println("Sending to local elevator.")
					g_elevator.m_requests = MergeRequests(g_elevator.m_requests, order[0])
					TriggerFirstRequest(order[0], g_elevator.toElevator)
					//g_elevator.toElevator(GetFloor(order[0][0]), elevio.ButtonType(GetButtonType(order[0][0])))

				} else {
					fmt.Println("Order list is empty or improperly formatted.")
				}
				// -------------------------------------------------------------------------------------------------------------
			case a := <-drv_buttons:
				g_elevator.handleButtonPress(a.Floor, a.Button, true)

				ELS[0] = g_elevator
				ELS[0].m_requests = storedElevator.m_requests
				order := MakeRequest(ELS)
				if len(order) > 0 && len(order[0]) > 0 {
					g_elevator.verifyRequest(order[0], localOtherRequest)
				} else {
					fmt.Println("Order list is empty or improperly formatted.")
				}

			case a := <-drv_floors:
				g_elevator.handleFloorArrival(a, localOtherRequest)

			case a := <-drv_obstr:
				g_elevator.setObstruction(a)

			case <-drv_stop:

			case <-timeoutTicker.C:
				if g_timer.timedOut() {
					g_elevator.handleDoorTimeout(localOtherRequest)
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
				fmt.Println("H")
				storedElevator.printElevatorState()
				// Function that will verify request and give them to the elevator, and from there also start elevator if necessary.
				if string(a) != "n" {
					b := DecodeStringToMatrix(a)
					fmt.Println("strconv")
					g_elevator.verifyRequest(b, localOtherRequest) // WHAT WILL ACTUALLY BE SENT TO THE SLAVE FROM MASTER??
				}

			case a := <-drv_buttons:
				g_elevator.handleButtonPress(a.Floor, a.Button, ActiveConnection) // Also need paramter for connection. Necessary to differ between button-light contract and standalone

			case a := <-drv_floors:
				g_elevator.handleFloorArrival(a, localOtherRequest)

			case a := <-drv_obstr:
				g_elevator.setObstruction(a)

			case <-drv_stop:

			case <-timeoutTicker.C:
				if g_timer.timedOut() {
					g_elevator.handleDoorTimeout(localOtherRequest)
				}
			}
			temp_requests := storedElevator.m_requests
			temp_elevator := g_elevator
			temp_elevator.m_requests = temp_requests
			storedElevator = temp_elevator

		}
	}
}
