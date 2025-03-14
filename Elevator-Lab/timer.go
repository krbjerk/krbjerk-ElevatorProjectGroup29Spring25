package main

import (
	"fmt"
	"time"
)

// Timer struct to hold timer state
type Timer struct {
	m_endTime time.Time
	m_active  bool
}

// Declare global timer object
var g_timer Timer

// Start the timer for a given duration in seconds
func (t *Timer) startTimer(_duration float64) {
	t.m_endTime = time.Now().Add(time.Duration(_duration * float64(time.Second)))
	t.m_active = true
	fmt.Println("Timer started.")
}

// Stop the timer
func (t *Timer) stopTimer() {
	t.m_active = false
	fmt.Println("Timer stopped.")
}

// Check if the timer has expired
func (t *Timer) timedOut() bool {
	if t.m_active && time.Now().After(t.m_endTime) {
		t.stopTimer()
		return true
	}
	return false
}

// Check if the given timer has expired
func checkTimerExpired(_timer Timer) bool {
	return time.Now().After(_timer.m_endTime)
}
