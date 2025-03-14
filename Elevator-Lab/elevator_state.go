package main

import (
	"Driver-go/elevio"
	"fmt"
)

const DOOR_OPEN_DURATION = 3.0
const NUM_FLOORS = 4

const (
	CV_All    = 0 // Clear all requests at the current floor
	CV_InDirn = 1 // Clear requests in the current direction
)

type Elevator struct {
	m_floor    int
	m_dirn     elevio.MotorDirection
	m_requests [NUM_FLOORS][3]bool
	config     struct {
		clearRequestVariant int
	}
	m_behavior    ElevatorBehavior
	m_obstruction bool
}

type ElevatorBehavior int

const (
	EB_Idle     ElevatorBehavior = 0
	EB_DoorOpen                  = 1
	EB_Moving                    = 2
)

type Direction int

// TODO:
// Not used in code for now. Make decision about it being removed or change from MD.
const (
	D_Down Direction = -1
	D_Stop           = 0
	D_Up             = 1
)

type Button int

const (
	B_HallUp   Button = 0
	B_HallDown        = 1
	B_Cab             = 2
)

// Global elevator instance
var g_elevator Elevator

// Initialize the elevator
func (_e *Elevator) initElevator() {
	elevio.SetMotorDirection(elevio.MD_Down)
	_e.m_dirn = elevio.MD_Down
	_e.m_behavior = EB_Moving
	_e.m_obstruction = false
	_e.config.clearRequestVariant = CV_InDirn
}

// Handle a button press
func (_e *Elevator) handleButtonPress(_btnFloor int, _btnType elevio.ButtonType) {
	fmt.Println("Button press")

	switch _e.m_behavior {
	case EB_DoorOpen:
		fmt.Println("Door is open.")
		if _e.m_floor == _btnFloor {
			g_timer.startTimer(DOOR_OPEN_DURATION)
			fmt.Println("door timeout 1")
		} else {
			_e.m_requests[_btnFloor][_btnType] = true
			if checkTimerExpired(g_timer) {
				_e.processRequest()
				fmt.Println("Acted on request.")
			}
		}

	case EB_Moving:
		_e.m_requests[_btnFloor][_btnType] = true
	case EB_Idle:
		_e.m_requests[_btnFloor][_btnType] = true
		if checkTimerExpired(g_timer) {
			_e.processRequest()
			fmt.Println("Acted on request.")
		}
	}
	_e.updateLights()
	_e.printElevatorState()
}

// Handle elevator arriving at a floor
func (_e *Elevator) handleFloorArrival(_newFloor int) {
	fmt.Println("Arrived at floor:", _newFloor)
	_e.m_floor = _newFloor
	elevio.SetFloorIndicator(_e.m_floor)

	if _e.m_behavior == EB_Moving && _e.shouldStopAtCurrentFloor() {
		fmt.Println("Stopping elevator at floor:", _newFloor)
		elevio.SetMotorDirection(elevio.MD_Stop)
		elevio.SetDoorOpenLamp(true)
		_e.clearRequestsAtCurrentFloor()
		g_timer.startTimer(DOOR_OPEN_DURATION)
		_e.updateLights()
		_e.m_behavior = EB_DoorOpen
		//_e.m_dirn = elevio.MD_Stop
	}
	_e.printElevatorState()
}

// Handle door timeout event
func (_e *Elevator) handleDoorTimeout() {
	fmt.Println("Door timeout, checking requests.")
	if _e.m_obstruction {
		g_timer.startTimer(DOOR_OPEN_DURATION)
	} else if _e.m_behavior == EB_DoorOpen {
		twin := _e.determineDirection()
		_e.m_dirn = twin.m_dirn
		_e.m_behavior = twin.m_behavior

		switch _e.m_behavior {
		case EB_DoorOpen:
			g_timer.startTimer(DOOR_OPEN_DURATION)
			_e.clearRequestsAtCurrentFloor()
			_e.updateLights()
		case EB_Moving:
			elevio.SetMotorDirection(_e.m_dirn)
			elevio.SetDoorOpenLamp(false)
		case EB_Idle:
			elevio.SetDoorOpenLamp(false)
			_e.processRequest()
		}
	}
	_e.printElevatorState()
}

// Process elevator request
func (_e *Elevator) processRequest() {
	twin := _e.determineDirection()
	_e.m_dirn = twin.m_dirn
	_e.m_behavior = twin.m_behavior

	switch twin.m_behavior {
	case EB_DoorOpen:
		_e.clearRequestsAtCurrentFloor()
		_e.updateLights()
	case EB_Moving:
		elevio.SetMotorDirection(_e.m_dirn)
		elevio.SetDoorOpenLamp(false)
	case EB_Idle:
		elevio.SetDoorOpenLamp(false)
	}
}

// Update elevator lights
func (_e Elevator) updateLights() {
	var BTNS = []elevio.ButtonType{elevio.BT_HallUp, elevio.BT_HallDown, elevio.BT_Cab}
	for _floor := 0; _floor < NUM_FLOORS; _floor++ {
		for _, _btn := range BTNS {
			elevio.SetButtonLamp(_btn, _floor, _e.m_requests[_floor][_btn])
		}
	}
}

// Convert direction to string
func directionToString(_dirn elevio.MotorDirection) string {
	switch _dirn {
	case 1:
		return "Up"
	case -1:
		return "Down"
	case 0:
		return "Stop"
	default:
		return "Unknown"
	}
}

// Convert behavior to string
func behaviorToString(_behavior ElevatorBehavior) string {
	switch _behavior {
	case EB_Idle:
		return "Idle"
	case EB_DoorOpen:
		return "DoorOpen"
	case EB_Moving:
		return "Moving"
	default:
		return "Unknown"
	}
}

// Print elevator state
func (_e *Elevator) printElevatorState() {
	fmt.Println("  +--------------------+")
	fmt.Printf(
		"  | Floor = %-2d         |\n"+
			"  | Dirn  = %-10s |\n"+
			"  | Behav = %-10s |\n",
		_e.m_floor,
		directionToString(_e.m_dirn),
		behaviorToString(_e.m_behavior),
	)
	fmt.Println("  +--------------------+")
	fmt.Println("  |  | up  | dn  | cab |")

	for _floor := NUM_FLOORS - 1; _floor >= 0; _floor-- {
		fmt.Printf("  | %d", _floor)
		for _btn := 0; _btn < 3; _btn++ {
			if (_floor == NUM_FLOORS-1 && _btn == int(B_HallUp)) || (_floor == 0 && _btn == B_HallDown) {
				fmt.Print("|     ")
			} else {
				if _e.m_requests[_floor][_btn] {
					fmt.Print("|  #  ")
				} else {
					fmt.Print("|  -  ")
				}
			}
		}
		fmt.Println("|")
	}
	fmt.Println("  +--------------------+")
}

func (_e *Elevator) setObstruction(value bool) {
	_e.m_obstruction = value
	if _e.m_obstruction && _e.m_behavior == EB_Idle {
		_e.m_behavior = EB_DoorOpen
		elevio.SetDoorOpenLamp(true)
		g_timer.startTimer(DOOR_OPEN_DURATION)

	}

}
