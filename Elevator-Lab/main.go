package main

import (
	"fmt"
	"math/rand"
	"root/elevio"
	"time"
)

func main() {

	g_elevator := init_elevator()

	var localOtherRequest [4][3]bool = [4][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}} // TODO: Musyt define it at start. can be all false

	var ELS ElevatorList = make([]Elevator, 1)
	var storedELS []Elevator = make([]Elevator, 1)

	masterTimer := rand.Intn(1500) + 300
	MasterCheck(masterTimer, &ELS, &storedElevator) // Pass by reference and synchronize storedElevator being sent

	elevio.Init("localhost:15657", NUM_FLOORS)

	g_elevator.initElevator()

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

				if len(ELS) < slaveID+1 {
					ELS = append(ELS, init_elevator())
					storedELS = append(storedELS, init_elevator())
					fmt.Println("New elevator")
				}
				ELS[slaveID] = DecodeElevatorFromString(a[:24])

				fmt.Println("Elevator:", 0)
				ELS[0].printElevatorState()
				fmt.Println("Elevator:", slaveID)
				ELS[slaveID].printElevatorState()

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
