package tlc

import (
	"time"
)

// RepeatingWrapper is a wrapper for the TLC object to implement special server side behaviour
type RepeatingWrapper struct {
	*TLC
	Message chan []byte
}

// Dispatch will call the dispatch method of the underlying TLC protocol instance,
// as well as concurrently execute code that interacts with the tlc instance, for testing
// purposes (behaviour one would expect from a service).
func (tlcR *RepeatingWrapper) Dispatch() error {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for {
			select {
			case msg, _ := <-tlcR.Message:
				tlcR.TLC.Message <- msg
				ticker.Stop()
				ticker = time.NewTicker(5 * time.Second)
			case <-ticker.C:
				tlcR.TLC.Message <- []byte("Empty payload")
			}
		}
	}()

	return tlcR.TLC.Dispatch()
}
