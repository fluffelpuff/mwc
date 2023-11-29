package mwc

import (
	"fmt"
	"net"
	"sync"
)

type ConnChannel struct {
	net.Conn
	motherConn  *WrappedConnection
	Channel     uint8
	readChannel chan []byte
	nextBytes   []byte
}

func (wc *ConnChannel) Write(b []byte) (int, error) {
	// Diese Gruppe wird verwendet um zu warten bis die Daten gesendet wurden
	wGroup := &sync.WaitGroup{}
	wGroup.Add(1)

	// Die Bytes werden vorbereitet
	newBytes := []byte{wc.Channel}
	newBytes = append(newBytes, b...)

	// Die Daten werden in den Kanal geschrieben
	sendableDataResult := &DataSendPaket{false, nil, 0, wGroup, newBytes}
	wc.motherConn.writeChannel <- sendableDataResult

	// Es wird gewartet bis die Daten gesendet wruden
	wGroup.Wait()

	// Es wird geprüft ob die Daten gesendet werden konnten
	if sendableDataResult.SendError != nil {
		return 0, sendableDataResult.SendError
	}

	// Die Verbindung ist geschlossen
	if sendableDataResult.WasClosed {
		return 0, fmt.Errorf("connection is closed")
	}

	// Die Anzahl der Gesendeten Bytes wird zurückgegebn
	return len(b), nil
}

func (wc *ConnChannel) Read(b []byte) (int, error) {
	// Es wird geprüft ob es noch Rest Daten gibt
	if len(wc.nextBytes) > 0 {
		// Die Restliche Größe der Daten wird ermittelt
		if len(wc.nextBytes) > len(b) {
			returnValue := wc.nextBytes[:len(b)]
			wc.nextBytes = wc.nextBytes[len(b):]
			copy(b, returnValue)
			return len(returnValue), nil
		} else {
			returnValue := wc.nextBytes
			wc.nextBytes = []byte{}
			copy(b, returnValue)
			return len(returnValue), nil
		}
	}

	// Die Daten werden gelesen
	result, isOk := <-wc.readChannel
	if !isOk {
		return 0, fmt.Errorf("connection is closed")
	}

	if len(result) > len(b) {
		wc.nextBytes = result[len(b):]
		result = result[:len(b)]
	}

	// Die Daten werden in b Kopiert
	copy(b, result)

	// Die Größe der Daten wird zurückgegebn
	return len(result), nil
}
