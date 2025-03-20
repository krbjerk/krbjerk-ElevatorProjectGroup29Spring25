package main

import (
	"fmt"
	"root/elevio"
)

type Twin struct {
	m_dirn     elevio.MotorDirection
	m_behavior ElevatorBehavior
}

var storedElevator Elevator

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

func (_e *Elevator) verifyRequest(_requestsFromMaster [4][3]bool) {
	// Declare startFloor and startButton outside the loop so they persist
	startFloor := -1
	startButton := -1

	// Check equality between stored requests and master requests
	for i := 0; i < NUM_FLOORS; i++ {
		for j := 0; j < 3; j++ {
			if storedElevator.m_requests[i][j] && _requestsFromMaster[i][j] {
				_e.m_requests[i][j] = true
				storedElevator.m_requests[i][j] = false // Assuming storedElevator.m_requests belongs to _e

				// Assign only the first matching request
				if startFloor == -1 && startButton == -1 {
					startFloor = i
					startButton = j
				}
			}
		}
	}

	// Only call if a valid request was found
	if startFloor != -1 && startButton != -1 {
		_e.toElevator(startFloor, elevio.ButtonType(startButton))
	}
}

func setStoredRequests(_btnFloor int, _btnType elevio.ButtonType) {
	storedElevator.m_requests[_btnFloor][_btnType] = true
}

func MakeRequestFromMaster([][]int) {

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

// GetFloor extracts the floor number from an order
func GetFloor(order int) int {
	return order / 3
}

// GetButtonType extracts the button type (B_HallUp, B_HallDown, B_Cab) from an order
func GetButtonType(order int) Button {
	return Button(order % 3) // Convert remainder to Button type
}
