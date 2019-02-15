package transmission

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventMarshal(t *testing.T) {
	e := &Event{SampleRate: 1}
	e.Data = map[string]interface{}{"a": 1}
	b, err := json.Marshal(e)
	testOK(t, err)
	testEquals(t, string(b), `{"data":{"a":1}}`)

	e.Timestamp = time.Unix(1476309645, 0).UTC()
	e.SampleRate = 5
	b, err = json.Marshal(e)
	testOK(t, err)
	testEquals(t, string(b), `{"data":{"a":1},"samplerate":5,"time":"2016-10-12T22:00:45Z"}`)
}
