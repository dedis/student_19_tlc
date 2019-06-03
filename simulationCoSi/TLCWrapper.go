package main

import (
	"fmt"
	"time"

	tlccosi "github.com/dedis/student_19_tlc/tlcCoSi"
)

// TLCWrapper is a wrapper for the TLC object to implement special server side behaviour
type TLCWrapper struct {
	*tlccosi.TLC
	TestingRounds int
}

// Dispatch will call the dispatch method of the underlying TLC protocol instance,
// as well as concurrently execute code that interacts with the tlc instance, for testing
// purposes (behaviour one would expect from a service).
func (tlcW *TLCWrapper) Dispatch() error {
	if !tlcW.IsRoot() {
		go func() {
			for i := 0; i < tlcW.TestingRounds; i++ {
				msg := []byte(fmt.Sprintf("%v Hello! Message: %v", tlcW.Name(), i))
				tlcW.Message <- msg
				<-tlcW.ThresholdSet
			}
			select {
			case <-time.After(time.Second * 2):
				tlcW.Terminate()
			}
		}()
	}
	return tlcW.TLC.Dispatch()
}
