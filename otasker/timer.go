// timer
package otasker

import (
	"fmt"
	"sync"
	"time"

	"github.com/vsdutka/metrics"
)

var (
	numberOfUsedTimers       = metrics.NewInt("Timers_Number_Of_Used_Timers", "Server - Number of used timers", "Pieces", "p")
	numberOfCreationOfTimer  = metrics.NewInt("Timers_Number_Of_Creation_Of_Timer", "Server - Number of creation of timer", "Pieces", "p")
	numberOfAcquiringOfTimer = metrics.NewInt("Timers_Number_Of_Acquiring_Of_Timers", "Server - Number of acquiring of timer", "Pieces", "p")
	numberOfReleasingOfTimer = metrics.NewInt("Timers_Number_Of_Releasing_Of_Timers", "Server - Number of releasing of timer", "Pieces", "p")
)

var timerPool sync.Pool

func acquireTimer(timeout time.Duration) *time.Timer {
	numberOfUsedTimers.Add(1)
	tv := timerPool.Get()
	if tv == nil {
		numberOfCreationOfTimer.Add(1)
		return time.NewTimer(timeout)
	}
	numberOfAcquiringOfTimer.Add(1)

	t := tv.(*time.Timer)
	if !t.Stop() {
		fmt.Println("Timer not stopped")
	}
	if t.Reset(timeout) {
		panic("BUG: Active timer trapped into acquireTimer()")
	}
	return t
}

func releaseTimer(t *time.Timer) {
	if !t.Stop() {
		// Collect possibly added time from the channel
		// if timer has been stopped and nobody collected its' value.
		select {
		case <-t.C:
		default:
		}
	}

	timerPool.Put(t)
	numberOfUsedTimers.Add(-1)
	numberOfReleasingOfTimer.Add(1)
}
