package main

import (
	"fmt"
	"math/rand"
	"root/communication" // Import the new package
	"root/elevator"
	"root/elevio"
	"root/timer"
	"time"
)

func main() {
	g_elevator := elevator.InitElevator()
	storedElevator := elevator.InitElevator()

	var localOtherRequest [4][3]bool // default all false
	var ELS elevator.ElevatorList = make([]elevator.Elevator, 1)

	masterTimer := rand.Intn(1500) + 300
	communication.MasterCheck(masterTimer, &ELS, &storedElevator)

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

	timeoutTicker := time.NewTicker(500 * time.Millisecond)
	defer timeoutTicker.Stop()

	for {
		if communication.Master {
			select {
			case a := <-communication.Read:
				fmt.Println("StoredElevator")
				fmt.Println(elevator.GetRequests(storedElevator))

				// 24 bits for elevator data + 1 digit for ID?
				slaveID := int(a[24] - '0') // careful with indexing
				ELS[0] = g_elevator
				ELS[0].SetRequests(elevator.GetRequests(storedElevator))

				if len(ELS) < slaveID+1 {
					ELS = append(ELS, elevator.InitElevator())
					fmt.Println("New elevator")
				}
				ELS[slaveID] = communication.DecodeElevatorFromString(a[:24])

				fmt.Println("Elevator:", 0)
				ELS[0].PrintElevatorState()
				fmt.Println("Elevator:", slaveID)
				ELS[slaveID].PrintElevatorState()

				order := elevator.MakeRequest(ELS)
				fmt.Println("Order made:")
				fmt.Println(order)

				for i := 0; i < elevator.NUM_FLOORS; i++ {
					for j := 0; j < 3; j++ {
						if elevator.GetIndRequest(storedElevator, i, j) &&
							elevator.MergeRequestsSlice(order)[i][j] {
							storedElevator.SetIndRequest(false, i, j)
						}
					}
				}

				communication.SlaveMapMutex.Lock()
				ch, ok := communication.SlaveOrderChans[int32(slaveID)]
				communication.SlaveMapMutex.Unlock()

				if ok {
					select {
					case ch <- order:
					default:
						fmt.Printf("Slave %d's order channel is full; skipping update.\n", slaveID)
					}
				}

				if len(order) > 0 {
					fmt.Println("Sending to local elevator.")
					localOtherRequests := order[1:]
					localOtherRequest = elevator.MergeRequestsSlice(localOtherRequests)

					g_elevator.SetRequests(
						elevator.MergeRequests(
							elevator.GetRequests(g_elevator),
							order[0],
						),
					)
					elevator.TriggerFirstRequest(order[0], g_elevator.ToElevator, localOtherRequest)
				} else {
					fmt.Println("Order list is empty or improperly formatted.")
				}

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
				// handle stop if needed

			case <-timeoutTicker.C:
				if timer.TimedOut() {
					g_elevator.HandleDoorTimeout(localOtherRequest)
				}
			}

		} else {
			select {
			case a := <-communication.Send:
				fmt.Println(a)
				fmt.Println("H")
				storedElevator.PrintElevatorState()

				if len(a) == 24 {
					b := elevator.DecodeStringToMatrix(a[:12])
					c := elevator.DecodeStringToMatrix(a[12:])
					fmt.Println("requests from master")
					fmt.Println(b)
					g_elevator.VerifyRequest(false, &storedElevator, b, c)
				} else if len(a) == 8 {
					g_elevator.SetRequests(
						elevator.MergeRequests(
							elevator.GetRequests(g_elevator),
							communication.StringCabToRequest(a),
						),
					)
				}

			case a := <-drv_buttons:
				g_elevator.HandleButtonPress(a.Floor, a.Button, &storedElevator, communication.ActiveConnection, localOtherRequest)

			case a := <-drv_floors:
				g_elevator.HandleFloorArrival(a, localOtherRequest)

			case a := <-drv_obstr:
				g_elevator.SetObstruction(a)

			case <-drv_stop:
				// handle stop if needed

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
