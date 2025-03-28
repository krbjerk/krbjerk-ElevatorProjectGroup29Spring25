package timer

import (
	"time"
)

var (
	endTime time.Time = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC) // Far future date
	active  bool      = false
)

func Start(duration float64) {
	endTime = time.Now().Add(time.Duration(duration * float64(time.Second)))
	active = true
}

func Stop() {
	active = false
}

func TimedOut() bool {
	if active && time.Now().After(endTime) {
		Stop()
		return true
	}
	return false
}

func IsExpired() bool {
	return time.Now().After(endTime)
}
