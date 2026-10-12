package transmission

import (
	"bytes"
	"fmt"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type Event struct {
	// APIKey, if set, overrides whatever is found in Config
	APIKey string
	// Dataset, if set, overrides whatever is found in Config
	Dataset string
	// SampleRate, if set, overrides whatever is found in Config
	SampleRate uint
	// APIHost, if set, overrides whatever is found in Config
	APIHost string
	// Timestamp, if set, specifies the time for this event. If unset, defaults
	// to Now()
	Timestamp time.Time
	// Metadata is a field for you to add in data that will be handed back to you
	// on the Response object read off the Responses channel. It is not sent to
	// Honeycomb with the event.
	Metadata interface{}

	// Data contains the content of the event (all the fields and their values)
	Data map[string]interface{}
}

// Marshaling an Event for batching up to the Honeycomb servers. Omits fields
// that aren't specific to this particular event, and allows for behavior like
// omitempty'ing a zero'ed out time.Time.
func (e *Event) MarshalJSON() ([]byte, error) {
	return e.appendJSON(nil)
}

func (e *Event) MarshalMsgpack() (byts []byte, err error) {
	tPointer := &(e.Timestamp)
	if e.Timestamp.IsZero() {
		tPointer = nil
	}

	// don't include sample rate if it's 1; this is the default
	sampleRate := e.SampleRate
	if sampleRate == 1 {
		sampleRate = 0
	}

	defer func() {
		if p := recover(); p != nil {
			byts = nil
			err = fmt.Errorf("msgpack panic: %v, trying to encode: %#v", p, e)
		}
	}()

	var buf bytes.Buffer
	encoder := msgpack.NewEncoder(&buf)
	encoder.SetCustomStructTag("json")
	err = encoder.Encode(struct {
		Data       map[string]interface{} `msgpack:"data"`
		SampleRate uint                   `msgpack:"samplerate,omitempty"`
		Timestamp  *time.Time             `msgpack:"time,omitempty"`
	}{e.Data, sampleRate, tPointer})
	return buf.Bytes(), err
}
