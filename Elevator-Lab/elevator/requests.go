package elevator

import (
	"fmt"
	"math"
	"root/elevio"
)

type Twin struct {
	m_dirn     elevio.MotorDirection
	m_behavior ElevatorBehavior
}

/*var storedElevator = Elevator{
	m_id:       0,
	m_floor:    0,
	m_dirn:     0,
	m_behavior: 0,
	m_requests: [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}},
	m_peers:    []string{},
}*/

// Check if there are requests above the current floor
func (e Elevator) RequestsAbove() bool {
	for _floor := e.m_floor + 1; _floor < 4; _floor++ {
		for _btn := 0; _btn < 3; _btn++ {
			if e.m_requests[_floor][_btn] {
				return true
			}
		}
	}
	return false
}

// Check if there are requests below the current floor
func (e Elevator) RequestsBelow() bool {
	for _floor := 0; _floor < e.m_floor; _floor++ {
		for _btn := 0; _btn < 3; _btn++ {
			if e.m_requests[_floor][_btn] {
				return true
			}
		}
	}
	return false
}

// Check if there is a request at the current floor
func (e Elevator) RequestsHere() bool {
	for _btn := 0; _btn < 3; _btn++ {
		if e.m_requests[e.m_floor][_btn] {
			return true
		}
	}
	return false
}

// Determine the next direction based on current requests
func (_e *Elevator) determineDirection() Twin {
	switch _e.m_dirn {
	case elevio.MD_Up:
		if _e.RequestsAbove() {
			return Twin{elevio.MD_Up, EB_Moving}
		} else if _e.RequestsHere() {
			return Twin{elevio.MD_Down, EB_DoorOpen}
		} else if _e.RequestsBelow() {
			return Twin{elevio.MD_Down, EB_Moving}
		} else {
			return Twin{elevio.MD_Stop, EB_Idle}
		}

	case elevio.MD_Down:
		if _e.RequestsBelow() {
			return Twin{elevio.MD_Down, EB_Moving}
		} else if _e.RequestsHere() {
			return Twin{elevio.MD_Up, EB_DoorOpen}
		} else if _e.RequestsAbove() {
			return Twin{elevio.MD_Up, EB_Moving}
		} else {
			return Twin{elevio.MD_Stop, EB_Idle}
		}

	case elevio.MD_Stop:
		if _e.RequestsHere() {
			return Twin{elevio.MD_Stop, EB_DoorOpen}
		} else if _e.RequestsAbove() {
			return Twin{elevio.MD_Up, EB_Moving}
		} else if _e.RequestsBelow() {
			return Twin{elevio.MD_Down, EB_Moving}
		} else {
			return Twin{elevio.MD_Stop, EB_Idle}
		}
	default:
		return Twin{elevio.MD_Stop, EB_Idle} // Must include default to avoid missing return error
	}
}

// Determine if the elevator should stop at the current floor
func (e *Elevator) shouldStopAtCurrentFloor() bool {
	switch e.m_dirn {
	case elevio.MD_Down:
		return e.m_requests[e.m_floor][B_HallDown] ||
			e.m_requests[e.m_floor][B_Cab] ||
			!e.RequestsBelow()

	case elevio.MD_Up:
		return e.m_requests[e.m_floor][B_HallUp] ||
			e.m_requests[e.m_floor][B_Cab] ||
			!e.RequestsAbove()

	case elevio.MD_Stop:
		fallthrough
	default:
		return true
	}
}

// Clear requests at the current floor
func (_e *Elevator) clearRequestsAtCurrentFloor() {
	switch _e.config.clearRequestVariant {
	case CV_All:
		// Clear all types of requests at the floor
		for btn := 0; btn < 3; btn++ {
			_e.m_requests[_e.m_floor][btn] = false
		}

	case CV_InDirn:
		// Clear requests in the direction of movement
		_e.m_requests[_e.m_floor][elevio.BT_Cab] = false

		switch _e.m_dirn {
		case elevio.MD_Up:
			_e.m_requests[_e.m_floor][elevio.BT_HallUp] = false
			// If no more requests above, clear down request at this floor
			if !_e.RequestsAbove() && !_e.m_requests[_e.m_floor][elevio.BT_HallUp] {
				_e.m_requests[_e.m_floor][elevio.BT_HallDown] = false
			}

		case elevio.MD_Down:
			_e.m_requests[_e.m_floor][elevio.BT_HallDown] = false
			// If no more requests below, clear up request at this floor
			if !_e.RequestsBelow() && !_e.m_requests[_e.m_floor][elevio.BT_HallDown] {
				_e.m_requests[_e.m_floor][elevio.BT_HallUp] = false
			}

		default:
			// If stopped, clear both up and down hall requests
			_e.m_requests[_e.m_floor][elevio.BT_HallUp] = false
			_e.m_requests[_e.m_floor][elevio.BT_HallDown] = false
		}
	}
}

// MakeRequest dynamically assigns hall requests among all connected elevators.
func MakeRequest(ELS []Elevator) [][NUM_FLOORS][3]bool {
	n := len(ELS)
	if n == 0 {
		// No elevators
		return nil
	}

	// cost array: EL_requests[i][0..7] for each elevator i (ifNUM_FLOORS=4 => 4 floors×2 btn=8)
	EL_requests := make([][]int, n)
	for i := 0; i < n; i++ {
		EL_requests[i] = make([]int, NUM_FLOORS*2)
	}

	// Track final assigned requests per elevator
	Finished_EL_requests := make([][]int, n)

	Time_Between_floors := 5

	// 1) Build cost matrix
	for i := 0; i < n; i++ { // the elevator we might assign to
		for j := 0; j < n; j++ { // the elevator that *has* requests
			for floor := 0; floor < NUM_FLOORS; floor++ {
				// up request?
				if ELS[j].m_requests[floor][0] {
					cost := int(math.Abs(float64(ELS[i].m_floor)-float64(floor))) * Time_Between_floors
					// cast dirn/behavior to int
					cost += 2 * int(ELS[i].m_dirn) * -int(math.Pow(float64(ELS[i].m_floor-floor), 0))
					cost += int(ELS[i].m_behavior)
					EL_requests[i][floor*2] = cost
				}
				// down request?
				if ELS[j].m_requests[floor][1] {
					cost := int(math.Abs(float64(ELS[i].m_floor)-float64(floor))) * Time_Between_floors
					cost -= 2 * int(ELS[i].m_dirn) * -int(math.Pow(float64(ELS[i].m_floor-floor), 0))
					cost += int(ELS[i].m_behavior)
					EL_requests[i][floor*2+1] = cost
				}
			}
		}
	}

	// 2) Repeatedly pick lowest cost request
	for {
		lowest := 100
		chosenElevator := -1
		chosenRequest := -1

		// Find absolute lowest cost across all
		for i := 0; i < n; i++ {
			for r := 0; r < NUM_FLOORS*2; r++ {
				c := EL_requests[i][r]
				if c != 0 && c < lowest {
					lowest = c
					chosenElevator = i
					chosenRequest = r
				}
			}
		}

		if lowest == 100 || chosenElevator < 0 {
			break // no more requests
		}

		// assign it
		Finished_EL_requests[chosenElevator] = append(
			Finished_EL_requests[chosenElevator], chosenRequest,
		)

		// clear that request from all elevators
		for i := 0; i < n; i++ {
			EL_requests[i][chosenRequest] = 0
		}

		// Increase cost of remaining requests for chosen elevator
		for r := 0; r < NUM_FLOORS*2; r++ {
			if EL_requests[chosenElevator][r] != 0 {
				EL_requests[chosenElevator][r] += 3
				// If the assigned was "up" (even index)
				if chosenRequest%2 == 0 && r > chosenRequest {
					EL_requests[chosenElevator][r] -= 5
				}
				// If the assigned was "down" (odd index)
				if chosenRequest%2 == 1 && r < chosenRequest {
					EL_requests[chosenElevator][r] -= 5
				}
			}
		}
	}

	// 3) Convert to a slice of [numFloors][3]bool
	result := make([][NUM_FLOORS][3]bool, n)

	for elIndex, assigned := range Finished_EL_requests {
		for _, flatFloor := range assigned {
			floor := flatFloor / 2
			btnType := flatFloor % 2 // 0=up, 1=down
			// skip out-of-bounds / cabin index2
			if floor >= 0 && floor < NUM_FLOORS && (btnType == 0 || btnType == 1) {
				result[elIndex][floor][btnType] = true
			}
		}
	}

	return result
}

func (_e *Elevator) VerifyRequest(master bool, storedElevator *Elevator, _requestsFromMaster [4][3]bool, otherRequest [4][3]bool) {
	// Declare startFloor and startButton outside the loop so they persist
	startFloor := -1
	startButton := -1

	// Check equality between stored requests and master requests
	for i := 0; i < NUM_FLOORS; i++ {
		for j := 0; j < 3; j++ {
			if _requestsFromMaster[i][j] {
				_e.m_requests[i][j] = true
				storedElevator.m_requests[i][j] = false // Assuming storedElevator.m_requests belongs to _e
				// Assign only the first matching request
				if startFloor == -1 && startButton == -1 {
					startFloor = i
					startButton = j
				}
			} else if otherRequest[i][j] && storedElevator.m_requests[i][j] {
				if !master {
					storedElevator.m_requests[i][j] = false
				}
			}
			// Insert when you have new makeRequest and can send that TODO
		}
	}

	// Only call if a valid request was found
	if startFloor != -1 && startButton != -1 {
		_e.ToElevator(startFloor, elevio.ButtonType(startButton), otherRequest)
	}
}

func (storedElevator *Elevator) setStoredRequests(_btnFloor int, _btnType elevio.ButtonType) {
	storedElevator.m_requests[_btnFloor][_btnType] = true
}

func TriggerFirstRequest(matrix [4][3]bool, sendFunc func(floor int, btn elevio.ButtonType, otherRequest [4][3]bool), otherRequest [4][3]bool) {
	for floor := 0; floor < 4; floor++ {
		for btn := 0; btn < 2; btn++ { // only hall up (0) and down (1)
			if matrix[floor][btn] {
				sendFunc(floor, elevio.ButtonType(btn), otherRequest)
				return
			}
		}
	}
}

func ConvertToElevatorRequests(request int) [4][3]bool {
	var m_requests [4][3]bool // Initialize as all false

	// Ensure request is valid (0 to 7)
	if request < 0 || request > 7 {
		fmt.Printf("Warning: Ignoring invalid request %d\n", request)
	}

	// Determine floor (0–3)
	floor := request / 2

	// Determine button type
	button := 0 // Default to "UP"
	if request%2 == 1 {
		button = 1 // "DOWN"
	}

	// Assign request to correct floor and button
	m_requests[floor][button] = true

	return m_requests
}

func MergeRequests(req1, req2 [4][3]bool) [4][3]bool {
	var merged [4][3]bool

	for floor := 0; floor < 4; floor++ {
		for button := 0; button < 3; button++ {
			// Convert bool to int, use bitwise OR, then convert back to bool
			merged[floor][button] = req1[floor][button] || req2[floor][button]
		}
	}

	return merged
}

func EncodeMatrixToString(matrix [4][3]bool) string {
	var result string
	for i := 0; i < 4; i++ {
		for j := 0; j < 3; j++ {
			if matrix[i][j] {
				result += "1"
			} else {
				result += "0"
			}
		}
	}
	return result
}

func DecodeStringToMatrix(s string) [4][3]bool {
	var matrix [4][3]bool
	if len(s) != 12 {
		fmt.Println("Invalid string length, must be 12. Actual length: ", len(s))
		return matrix
	}

	for i := 0; i < 4; i++ {
		for j := 0; j < 3; j++ {
			index := i*3 + j
			matrix[i][j] = s[index] == '1'
		}
	}
	return matrix
}

func MergeRequestsSlice(requests [][4][3]bool) [4][3]bool {
	var merged [4][3]bool

	for _, r := range requests {
		for floor := 0; floor < 4; floor++ {
			for btn := 0; btn < 3; btn++ {
				merged[floor][btn] = merged[floor][btn] || r[floor][btn]
			}
		}
	}

	return merged
}
