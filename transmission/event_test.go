package transmission

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/ianwilkes/msgp/msgp"
)

func TestEventMarshal(t *testing.T) {
	e := &Event{SampleRate: 1}
	e.Data = map[string]interface{}{
		"a": int64(1),
		"b": float64(1.0),
		"c": true,
		"d": "foo",
		"e": time.Second,
	}
	b, err := json.Marshal(e)
	testOK(t, err)
	testEquals(t, string(b), `{"data":{"a":1,"b":1,"c":true,"d":"foo","e":1000000000}}`)

	e.Timestamp = time.Unix(1476309645, 0).UTC()
	e.SampleRate = 5
	b, err = json.Marshal(e)
	testOK(t, err)
	testEquals(t, string(b), `{"data":{"a":1,"b":1,"c":true,"d":"foo","e":1000000000},"samplerate":5,"time":"2016-10-12T22:00:45Z"}`)

	var buf bytes.Buffer
	err = msgp.Encode(&buf, e)
	testOK(t, err)

	e2 := &Event{}
	err = msgp.Decode(&buf, e2)
	testOK(t, err)

	// a time.Duration is encoded as a plain int.
	if asInt, ok := e2.Data["e"].(int64); ok {
		e2.Data["e"] = time.Duration(asInt)
	} else {
		t.Errorf(`expected falue for "e" missing from decoded map`)
	}

	testEquals(t, e2, e)
}

func BenchmarkEventEncode(b *testing.B) {
	tm, err := time.Parse(time.RFC3339, "2001-02-03T04:05:06Z")
	testOK(b, err)
	evt := Event{
		SampleRate: 2,
		Timestamp:  tm,
		Data: map[string]interface{}{
			"a": int64(1),
			"b": float64(1.0),
			"c": true,
			"d": "foo",
		},
	}

	var buf bytes.Buffer
	b.Run("json", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			buf.Reset()
			err := json.NewEncoder(&buf).Encode(&evt)
			testOK(b, err)
		}
	})

	b.Run("msgpack", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			buf.Reset()
			err := msgp.Encode(&buf, &evt)
			testOK(b, err)
		}
	})
}
