package main

import (
	"fmt"
	"math/rand"
	"root/elevator"
	"root/elevio"
	"root/timer"
	"time"
)

func main() {

	g_elevator := elevator.InitElevator()
	storedElevator := elevator.InitElevator()

	var localOtherRequest [4][3]bool = [4][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}} // TODO: Musyt define it at start. can be all false

	var ELS elevator.ElevatorList = make([]elevator.Elevator, 1)

	masterTimer := rand.Intn(1500) + 300
	MasterCheck(masterTimer, &ELS, &storedElevator) // Pass by reference and synchronize storedElevator being sent

	elevio.Init("localhost:15657", elevator.NUM_FLOORS)

	g_elevator.InitBetweenFloors()

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
				fmt.Println(elevator.GetRequests(storedElevator))
				// -------------------------------------------------------------------------------------------------------------
				slaveID := int(a[24] - '0') // Adjust this as needed
				ELS[0] = g_elevator
				ELS[0].SetRequests(elevator.GetRequests(storedElevator))
				// REMEMBER TO FIX SYNCHRONIZATION OF ELS, DEEM IF NECESSARY.

				if len(ELS) < slaveID+1 {
					ELS = append(ELS, elevator.InitElevator())
					fmt.Println("New elevator")
				}
				ELS[slaveID] = DecodeElevatorFromString(a[:24])

				fmt.Println("Elevator:", 0)
				ELS[0].PrintElevatorState()
				fmt.Println("Elevator:", slaveID)
				ELS[slaveID].PrintElevatorState()

				order := elevator.MakeRequest(ELS) // WHAT WILL BE SENT TO SLAVE
				fmt.Println("Order made:")
				fmt.Println(order)

				for i := 0; i < elevator.NUM_FLOORS; i++ {
					for j := 0; j < 3; j++ {
						if elevator.GetIndRequest(storedElevator, i, j) && elevator.MergeRequestsSlice(order)[i][j] {
							storedElevator.SetIndRequest(false, i, j)
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
					localOtherRequest = elevator.MergeRequestsSlice(localOtherRequests)
					g_elevator.SetRequests(elevator.MergeRequests(elevator.GetRequests(g_elevator), order[0]))
					elevator.TriggerFirstRequest(order[0], g_elevator.ToElevator, localOtherRequest)

				} else {
					fmt.Println("Order list is empty or improperly formatted.")
				}
				// -------------------------------------------------------------------------------------------------------------
			case a := <-drv_buttons:
				g_elevator.HandleButtonPress(a.Floor, a.Button, &storedElevator, true, localOtherRequest)

				fmt.Println("StoredElevator")
				fmt.Println(elevator.GetRequests(storedElevator))

				ELS[0] = g_elevator
				ELS[0].SetRequests(elevator.GetRequests(storedElevator))
				order := elevator.MakeRequest(ELS)

				fmt.Println("Order made after button press")
				fmt.Println(order)

				localOtherRequests := order[1:]
				localOtherRequest = elevator.MergeRequestsSlice(localOtherRequests)

				if len(order) > 0 {
					fmt.Println("time to verify.")
					g_elevator.VerifyRequest(true, &storedElevator, order[0], localOtherRequest)
				} else {
					fmt.Println("Order list is empty or improperly formatted.")
				}

			case a := <-drv_floors:
				g_elevator.HandleFloorArrival(a, localOtherRequest)

			case a := <-drv_obstr:
				g_elevator.SetObstruction(a)

			case <-drv_stop:

			case <-timeoutTicker.C:
				if timer.TimedOut() {
					g_elevator.HandleDoorTimeout(localOtherRequest)
				}
			}

		} else {
			select {
			case a := <-Send:
				fmt.Println(a)
				fmt.Println("H")
				storedElevator.PrintElevatorState()
				// Function that will verify request and give them to the elevator, and from there also start elevator if necessary.
				if len(a) == 24 {
					b := elevator.DecodeStringToMatrix(a[:12]) // need to decode. can use a[:11]
					c := elevator.DecodeStringToMatrix(a[12:])
					fmt.Println("requests from master")
					fmt.Println(b)
					g_elevator.VerifyRequest(false, &storedElevator, b, c) // WHAT WILL ACTUALLY BE SENT TO THE SLAVE FROM MASTER??
				} else if len(a) == 8 {
					g_elevator.SetRequests(elevator.MergeRequests(elevator.GetRequests(g_elevator), stringCabToRequest(a)))
				}

			case a := <-drv_buttons:
				g_elevator.HandleButtonPress(a.Floor, a.Button, &storedElevator, ActiveConnection, localOtherRequest) // Also need paramter for connection. Necessary to differ between button-light contract and standalone
				// localotherrequest Placeholder for request from master. all false will not affect.
			case a := <-drv_floors:
				g_elevator.HandleFloorArrival(a, localOtherRequest)

			case a := <-drv_obstr:
				g_elevator.SetObstruction(a)

			case <-drv_stop:

			case <-timeoutTicker.C:
				if timer.TimedOut() {
					g_elevator.HandleDoorTimeout(localOtherRequest)
				}
			}
			temp_elevator := g_elevator
			temp_elevator.SetRequests(elevator.GetRequests(storedElevator))
			storedElevator = temp_elevator
		}
	}
}
