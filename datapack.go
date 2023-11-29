package mwc

import "sync"

type DataSendPaket struct {
	WasClosed bool
	SendError error
	NData     int
	SendWait  *sync.WaitGroup
	Data      []byte
}
