package main

import (
	"fmt"
	"math/rand"
	"os"
	"root/elevio"
	"time"
)

func main() {

	

	var SimServerPort = "localhost:12345"
	if len(os.Args) > 1 {
		SimServerPort = string("localhost:" + os.Args[1]) // First argument after "go run ."
	}

	init_elevator := Elevator{
		m_id:       0,
		m_floor:    0,
		m_dirn:     0,
		m_behavior: 0,
		m_requests: [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}},
		m_peers:    []string{},
	}

	g_elevator := init_elevator
	var localOtherRequest [4][3]bool = [4][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}} // TODO: Musyt define it at start. can be all false

	var ELS ElevatorList = make([]Elevator, 1)
	var storedELS []Elevator = make([]Elevator, 1)
	/*for i := range ELS {
		ELS[i].m_requests = [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}}
		storedELS[i].m_requests = [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}}
	}*/

	masterTimer := rand.Intn(3000) + 1500
	MasterCheck(masterTimer, &ELS, &storedElevator) // Pass by reference and synchronize storedElevator being sent

	elevio.Init(SimServerPort, NUM_FLOORS)

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
		if Master { // Master is in TxRx
			select {
			case a := <-Read:
				fmt.Println("StoredElevator")
				fmt.Println(storedElevator.m_requests)
				// -------------------------------------------------------------------------------------------------------------
				slaveID := int(a[24] - '0') // Adjust this as needed
				ELS[0] = g_elevator
				ELS[0].m_requests = storedElevator.m_requests
				// REMEMBER TO FIX SYNCHRONIZATION OF ELS, DEEM IF NECESSARY.

				if len(ELS) < slaveID+1 { // Will this introduce problems with slave disconnect?
					ELS = append(ELS, init_elevator)
					storedELS = append(storedELS, init_elevator)
					fmt.Println("New elevator")
				}
				ELS[slaveID] = DecodeElevatorFromString(a[:24])

				fmt.Println("Elevator:", 0)
				ELS[0].printElevatorState()
				fmt.Println("Elevator:", slaveID)
				ELS[slaveID].printElevatorState()
				// ------
				// if new ELS != storedELS
				// 		then remove the overlapping requests from new ELS
				//		storedELS = new ELSz
				/*for k := 0; k < len(ELS); k++ {
					//fmt.Println("orders, elevator number: ", k)
					//fmt.Println(ELS[k].m_requests)
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

				}*/
				// ------
				order := MakeRequest(ELS) // WHAT WILL BE SENT TO SLAVE
				fmt.Println("Order made:")
				fmt.Println(order)

				for i := 0; i < NUM_FLOORS; i++ {
					for j := 0; j < 3; j++ {
						if storedElevator.m_requests[i][j] && MergeRequestsSlice(order)[i][j] {
							storedElevator.m_requests[i][j] = false
						}
					}
				}

				// ---
				slaveMapMutex.Lock()
				ch, ok := slaveOrderChans[int32(slaveID)]
				slaveMapMutex.Unlock()
				if ok {
					select {
					case ch <- order: // ACTUALLY SEND TO SLAVE
						//fmt.Println("In select, ch<-order", order)
						// We could change the elevators here so that it drops the request.
					default:
						fmt.Printf("Slave %d's order channel is full; skipping update.\n", slaveID)
					}
				}
				if len(order) > 0 {
					fmt.Println("Sending to local elevator.")
					localOtherRequests := order[1:]
					localOtherRequest = MergeRequestsSlice(localOtherRequests)
					g_elevator.m_requests = MergeRequests(g_elevator.m_requests, order[0])
					TriggerFirstRequest(order[0], g_elevator.toElevator, localOtherRequest)

				} else {
					fmt.Println("Order list is empty or improperly formatted.")
				}
				// -------------------------------------------------------------------------------------------------------------
			case a := <-drv_buttons:
				g_elevator.handleButtonPress(a.Floor, a.Button, true, localOtherRequest)
				fmt.Println("StoredElevator")
				fmt.Println(storedElevator.m_requests)

				ELS[0] = g_elevator
				ELS[0].m_requests = storedElevator.m_requests //
				order := MakeRequest(ELS)
				// ORDER
				fmt.Println("Order made after button press")
				fmt.Println(order)
				localOtherRequests := order[1:]
				localOtherRequest = MergeRequestsSlice(localOtherRequests)
				if len(order) > 0 {
					fmt.Println("time to verify.")
					g_elevator.verifyRequest(true, order[0], localOtherRequest)
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

		} else {
			select {
			case a := <-Send:
				fmt.Println(a)
				fmt.Println("H")
				storedElevator.printElevatorState()
				// Function that will verify request and give them to the elevator, and from there also start elevator if necessary.
				if len(a) == 24 {
					b := DecodeStringToMatrix(a[:12]) // need to decode. can use a[:11]
					c := DecodeStringToMatrix(a[12:])
					fmt.Println("requests from master")
					fmt.Println(b)
					g_elevator.verifyRequest(false, b, c) // WHAT WILL ACTUALLY BE SENT TO THE SLAVE FROM MASTER??
				} else if len(a) == 8 {
					g_elevator.m_requests = MergeRequests(g_elevator.m_requests, stringCabToRequest(a))
				}

			case a := <-drv_buttons:
				g_elevator.handleButtonPress(a.Floor, a.Button, ActiveConnection, localOtherRequest) // Also need paramter for connection. Necessary to differ between button-light contract and standalone
				// localotherrequest Placeholder for request from master. all false will not affect.
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
