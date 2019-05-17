package main

import (
	"fmt"
	"time"

	"github.com/dedis/student_19_tlc/tlc"
)

type TLCWrapper struct {
	*tlc.TLC
	TestingRounds int
}

func (tlcW *TLCWrapper) Start() error {
	return tlcW.TLC.Start()
}

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
