package clock

import "time"

// Real implements ports.Clock using wall-clock time.
type Real struct{}

// Now returns the current time.
func (Real) Now() time.Time {
	return time.Now()
}
