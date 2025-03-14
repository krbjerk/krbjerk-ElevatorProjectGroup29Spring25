package main

import (
	"Driver-go/elevio"
)

type Twin struct {
	m_dirn     elevio.MotorDirection
	m_behavior ElevatorBehavior
}

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
