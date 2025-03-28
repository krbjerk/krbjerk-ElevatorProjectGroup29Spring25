package elevator

import (
	"fmt"
	"root/elevio"
	"root/timer"
)

const DOOR_OPEN_DURATION = 3.0
const NUM_FLOORS = 4

const (
	CV_All    = 0 // Clear all requests at the current floor
	CV_InDirn = 1 // Clear requests in the current direction
)

type Elevator struct {
	m_id          int
	m_floor       int
	m_dirn        elevio.MotorDirection
	m_requests    [NUM_FLOORS][3]bool
	m_behavior    ElevatorBehavior
	m_obstruction bool
	config        struct {
		clearRequestVariant int
	}
}

type ElevatorList []Elevator

type ElevatorBehavior int

const (
	EB_Idle     ElevatorBehavior = 0
	EB_DoorOpen                  = 1
	EB_Moving                    = 2
)

type Button int

const (
	B_HallUp   Button = 0
	B_HallDown        = 1
	B_Cab             = 2
)

type ButtonEvent struct {
	Floor  int
	Button Button
}

// Global elevator instance
//var g_elevator Elevator

func InitElevator() Elevator {

	return Elevator{
		m_id:       0,
		m_floor:    0,
		m_dirn:     0,
		m_behavior: 0,
		m_requests: [NUM_FLOORS][3]bool{{false, false, false}, {false, false, false}, {false, false, false}, {false, false, false}},
		config: struct {
			clearRequestVariant int
		}{
			clearRequestVariant: CV_InDirn,
		},
	}
}

// Initialize the elevator
func (_e *Elevator) InitBetweenFloors() {
	if elevio.GetFloor() == -1 {
		// Make the elevator move to an actual floor on startup. Necessary for the state machine.

		elevio.SetMotorDirection(elevio.MD_Down)
		_e.m_dirn = elevio.MD_Down
		_e.m_behavior = EB_Moving
		_e.m_obstruction = false
		_e.config.clearRequestVariant = CV_InDirn
	}
}

// Handle a button press
func (_e *Elevator) HandleButtonPress(_btnFloor int, _btnType elevio.ButtonType, storedElevator *Elevator, _connection bool, otherRequest [4][3]bool) {
	fmt.Println("-----------------")
	fmt.Println("Button press")
	fmt.Println("-----------------")
	//_e.printElevatorState()
	if _connection {
		// Store requests
		if _btnType != elevio.BT_Cab {
			storedElevator.setStoredRequests(_btnFloor, _btnType)
		} else {
			_e.ToElevator(_btnFloor, _btnType, otherRequest)
		}

	} else {
		_e.ToElevator(_btnFloor, _btnType, otherRequest)
	}
}

func (_e *Elevator) ToElevator(_btnFloor int, _btnType elevio.ButtonType, otherRequest [4][3]bool) {
	switch _e.m_behavior {
	case EB_DoorOpen:
		//fmt.Println("Door is open.")
		if _e.m_floor == _btnFloor {
			timer.Start(DOOR_OPEN_DURATION)
			//fmt.Println("door timeout 1")
		} else {
			_e.m_requests[_btnFloor][_btnType] = true
			if timer.IsExpired() {
				_e.ProcessRequest(otherRequest)
				//fmt.Println("Acted on request.")
			}
		}

	case EB_Moving:
		_e.m_requests[_btnFloor][_btnType] = true
	case EB_Idle:
		_e.m_requests[_btnFloor][_btnType] = true
		if timer.IsExpired() {
			_e.ProcessRequest(otherRequest)
			//fmt.Println("Acted on request.")
		}
	}
	_e.UpdateLights(otherRequest)
	//_e.printElevatorState()
}

// Handle elevator arriving at a floor
func (_e *Elevator) HandleFloorArrival(_newFloor int, otherRequest [4][3]bool) {
	//fmt.Println("Arrived at floor:", _newFloor)
	_e.m_floor = _newFloor
	elevio.SetFloorIndicator(_e.m_floor)

	if _e.m_behavior == EB_Moving && _e.shouldStopAtCurrentFloor() {
		//fmt.Println("Stopping elevator at floor:", _newFloor)
		elevio.SetMotorDirection(elevio.MD_Stop)
		elevio.SetDoorOpenLamp(true)

		// Fix: Clear requests *before* stopping direction is set
		_e.clearRequestsAtCurrentFloor()

		timer.Start(DOOR_OPEN_DURATION)
		_e.UpdateLights(otherRequest)

		_e.m_behavior = EB_DoorOpen
	}
}

// Handle door timeout event
func (_e *Elevator) HandleDoorTimeout(otherRequest [4][3]bool) {
	if _e.m_obstruction {
		timer.Start(DOOR_OPEN_DURATION)
	} else if _e.m_behavior == EB_DoorOpen {
		twin := _e.determineDirection()
		_e.m_dirn = twin.m_dirn
		_e.m_behavior = twin.m_behavior

		switch _e.m_behavior {
		case EB_DoorOpen:
			timer.Start(DOOR_OPEN_DURATION)
			_e.clearRequestsAtCurrentFloor()
			_e.UpdateLights(otherRequest)
		case EB_Moving:
			elevio.SetMotorDirection(_e.m_dirn)
			elevio.SetDoorOpenLamp(false)
		case EB_Idle:
			elevio.SetDoorOpenLamp(false)
			_e.ProcessRequest(otherRequest)
		}
	}
	//_e.printElevatorState()
}

// Process elevator request
func (_e *Elevator) ProcessRequest(otherRequest [4][3]bool) {
	twin := _e.determineDirection()
	_e.m_dirn = twin.m_dirn
	_e.m_behavior = twin.m_behavior

	switch twin.m_behavior {
	case EB_DoorOpen:
		_e.clearRequestsAtCurrentFloor()
		_e.UpdateLights(otherRequest)
	case EB_Moving:
		elevio.SetMotorDirection(_e.m_dirn)
		elevio.SetDoorOpenLamp(false)
	case EB_Idle:
		elevio.SetDoorOpenLamp(false)
	}
}

func (_e Elevator) UpdateLights(otherElevators [4][3]bool) {
	var BTNS = []elevio.ButtonType{elevio.BT_HallUp, elevio.BT_HallDown, elevio.BT_Cab}
	for _floor := 0; _floor < NUM_FLOORS; _floor++ {
		for n, _btn := range BTNS {
			shouldLight := _e.m_requests[_floor][_btn] || otherElevators[_floor][n]
			elevio.SetButtonLamp(_btn, _floor, shouldLight)
		}
	}
}

func (_e *Elevator) SetObstruction(value bool) {
	_e.m_obstruction = value
	if _e.m_obstruction && _e.m_behavior == EB_Idle {
		_e.m_behavior = EB_DoorOpen
		elevio.SetDoorOpenLamp(true)
		timer.Start(DOOR_OPEN_DURATION)

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
func (_e *Elevator) PrintElevatorState() {
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

func (_e *Elevator) SetRequests(requests [4][3]bool) {
	_e.m_requests = requests
}

func GetRequests(_e Elevator) [4][3]bool {
	return _e.m_requests
}

func (_e *Elevator) SetIndRequest(_value bool, _floor int, _button int) {
	_e.m_requests[_floor][_button] = _value
}

func GetIndRequest(_e Elevator, _floor int, _button int) bool {
	return _e.m_requests[_floor][_button]
}

func (e *Elevator) SetFloor(floor int) {
	e.m_floor = floor
}

func (e *Elevator) SetBehavior(behavior ElevatorBehavior) {
	e.m_behavior = behavior
}

func (e *Elevator) SetDirection(dirn elevio.MotorDirection) {
	e.m_dirn = dirn
}

func GetFloor(e Elevator) int {
	return e.m_floor
}

func GetBehavior(e Elevator) ElevatorBehavior {
	return e.m_behavior
}

func GetDirection(e Elevator) elevio.MotorDirection {
	return e.m_dirn
}
