package mwc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// Stellt eine Wrapped Verbindung dar
type WrappedCommConnection struct {
	writeChannel chan *DataSendPaket
	wrappedConn  net.Conn
	active       bool
	wGroup       *sync.WaitGroup
	subChannels  map[uint8]*ConnChannel
	onClose      func(string)
}

// Wird ausgeführt wenn die Verbindung geschlossen wurde
func (o *WrappedCommConnection) killByDisconnect() {
	// Es wird ermittelt ob die Verbindung bereits geschlossen wurde
	if !o.active {
		return
	}

	// Die Verbindung wird auf geschlossen gesetzt
	o.active = false

	// Die Conn Verbindung wird final geschlossen
	o.wrappedConn.Close()

	// Es werden all Kanäle geschlossen
	for item := range o.subChannels {
		close(o.subChannels[item].readChannel)
	}

	// Es wird ermittelt ob es eine Kill Function gibt
	if o.onClose != nil {
		o.onClose("connection closed")
	}
}

// Wird verwendet wenn ein ungültiger Datensatz empfangen wurde
func (o *WrappedCommConnection) killInvalidDataRecived() {
	// Es wird ermittelt ob die Verbindung bereits geschlossen wurde
	if !o.active {
		return
	}

	// Die Verbindung wird auf geschlossen gesetzt
	o.active = false

	// Die Conn Verbindung wird final geschlossen
	o.wrappedConn.Close()

	// Es werden all Kanäle geschlossen
	for item := range o.subChannels {
		close(o.subChannels[item].readChannel)
	}

	// Es wird ermittelt ob es eine Kill Function gibt
	if o.onClose != nil {
		o.onClose("connection closed")
	}
}

// Wird verwendet um die Verbindung zu schließen sollte diese ungültige Daten verwenden
func (o *WrappedCommConnection) killUnkownChannel() {
	// Es wird ermittelt ob die Verbindung bereits geschlossen wurde
	if !o.active {
		return
	}

	// Die Verbindung wird auf geschlossen gesetzt
	o.active = false

	// Die Conn Verbindung wird final geschlossen
	o.wrappedConn.Close()

	// Es werden all Kanäle geschlossen
	for item := range o.subChannels {
		close(o.subChannels[item].readChannel)
	}

	// Es wird ermittelt ob es eine Kill Function gibt
	if o.onClose != nil {
		o.onClose("connection closed")
	}
}

// Wird verwendet um Daten bis zum letzten auszulesen
func (o *WrappedCommConnection) readCompleteFramedOrSingleData() ([]byte, error) {
	// Speichert die eingelesen Daten ab
	readedData := []byte{}

	// Es wird auf Eintreffende Daten gewartet
	for {
		// Speichert die Empfangen Daten zwischen
		temod := make([]byte, 2048)

		// Es wird auf eintreffende Daten gewartet
		n, err := o.wrappedConn.Read(temod)
		if err != nil {
			return nil, err
		}

		// Die Daten werden prepariert
		temod = temod[:n]

		// Es wird ermittelt ob es sich um ein Frame oder ein FINALLY handelt
		if len(temod) < 7 {
			return nil, fmt.Errorf("invalid data recived")
		}

		// Es wird geprüft ob es sich um ein Finally handelt
		if bytes.Equal(temod, []byte("FINALLY")) {
			break
		} else if bytes.Equal(temod[0:1], []byte("F")) {
			readedData = append(readedData, temod[1:]...)
		} else if bytes.Equal(temod[0:1], []byte("S")) {
			readedData = append(readedData, temod[1:]...)
			break
		} else {
			return nil, fmt.Errorf("")
		}
	}

	return readedData, nil
}

// Wird verwendet um Daten zu senden
func (o *WrappedCommConnection) writeCompleteFrameOrSingleData(data []byte) error {
	// Die zu sendenden Daten werden aufgeteilt
	splitedFrames := splitSlice(data, 2047)

	// Sollte es mehr als 1 Frame sein, werden mehre Frames gesendet
	if len(splitedFrames) > 1 {
		// Die Einzelnen Pakete werden übertragen
		for _, frame := range splitedFrames {
			// Es wird ein neues Byte Slice erstellt und gesendet
			newByteSlice := []byte{}
			newByteSlice = append(newByteSlice, []byte("F")...)
			newByteSlice = append(newByteSlice, frame...)

			// Das Paket wird gesendet
			_, err := o.wrappedConn.Write(newByteSlice)
			if err != nil {
				return err
			}
		}

		// Der Gegenseite wird mitgeteitl dass der Stream beendet wurde
		_, err := o.wrappedConn.Write([]byte("FINALLY"))
		if err != nil {
			return err
		}
	} else {
		// Es wird ein neues Byte Slice erstellt und gesendet
		newByteSlice := []byte{}
		newByteSlice = append(newByteSlice, []byte("S")...)
		newByteSlice = append(newByteSlice, splitedFrames[0]...)

		// Das Paket wird gesendet
		_, err := o.wrappedConn.Write(newByteSlice)
		if err != nil {
			return err
		}
	}

	// Es ist kein Fehler aufgetreten
	return nil
}

// Wird im Hintergrund asugeführt und nimmt eintreffende Daten entgegen
func (o *WrappedCommConnection) readerRoutine() {
	// Signalisiert dass der Reader nicht mehr ausgeführt wird
	defer o.wGroup.Done()

	// Die Schleife wird solange ausgeführt, bis die Verbindung getrennt wurde
	for {
		// Speichert die eingelesen Daten ab
		readedData, err := o.readCompleteFramedOrSingleData()
		if err != nil {
			if errors.Is(err, io.EOF) {
				o.killByDisconnect()
				return
			} else {
				o.killInvalidDataRecived()
				return
			}
		}

		// Es wird geprüft ob mindestens 1 Byte vorhanden ist
		if len(readedData) < 1 {
			o.killInvalidDataRecived()
			return
		}

		// Die ersten 2 Bytes werden einzeln ausgelesen
		channel, ok := o.subChannels[readedData[0]]
		if !ok {
			o.killUnkownChannel()
			return
		}

		// Der Channel wird entfernt
		workingData := readedData[1:]

		// Die Daten werden an den Endpunkt übergeben
		channel.readChannel <- workingData
	}
}

// Wird im Hintergrund ausgeführt und sendet Daten ab
func (o *WrappedCommConnection) writerRoutine() {
	// Signalisiert dass der Writer nicht mehr ausgeführt wird
	defer o.wGroup.Done()

	// Wird solange ausgeführt bis die Verbindung geschlpssen wurde
	for {
		// Es wird auf Daten gewartet welche gesendet werden sollen
		sendableData, isOk := <-o.writeChannel
		if !isOk {
			return
		}

		// Die Daten werden übertragen
		if err := o.writeCompleteFrameOrSingleData(sendableData.Data); err != nil {
			// Der Genaue Fehler wird ermittelt
			if errors.Is(err, io.EOF) {
				o.killByDisconnect()
				sendableData.WasClosed = true
			} else {
				sendableData.SendError = err
				o.killInvalidDataRecived()
			}

			// Es wird Signalisiert dass der Vorgang fertig ist
			sendableData.SendWait.Done()

			// Der Vorgang wird abegrbochen
			return
		}

		// Die Menge der gesendeten Daten wird abgespeichert
		sendableData.NData = len(sendableData.Data)

		// Es wird Signalisiert dass die Verbindung nicht getretnnt wurde
		sendableData.WasClosed = false

		// Es wird gemeldet dass der Prozess fertig ist
		sendableData.SendWait.Done()
	}
}

// Öffnet einen neuen Sub Channel
func (o *WrappedCommConnection) OpenSubConnChannel(channel uint8) (*ConnChannel, error) {
	newSubChannel := &ConnChannel{o.wrappedConn, o, channel, make(chan []byte), make([]byte, 0)}
	o.subChannels[channel] = newSubChannel
	return newSubChannel, nil
}

// Setzt die OnClose Event Funktion
func (o *WrappedCommConnection) AddEventByClose(eventFunction func(string)) {
	o.onClose = eventFunction
}

// nimmt eine Verbindung Entgegen und Wrapp diese
func WrappCommConnection(conn net.Conn) *WrappedCommConnection {
	// Die Verbindung wird eingefangen
	revalObject := &WrappedCommConnection{
		wrappedConn:  conn,
		active:       true,
		wGroup:       new(sync.WaitGroup),
		writeChannel: make(chan *DataSendPaket),
		subChannels:  map[uint8]*ConnChannel{},
		onClose:      nil,
	}

	// Die Reader Routine wird gestartet
	revalObject.wGroup.Add(1)
	go revalObject.readerRoutine()

	// Die Writer Routine wird gestartet
	revalObject.wGroup.Add(1)
	go revalObject.writerRoutine()

	// Das Objekt wird zurückgegeb
	return revalObject
}
