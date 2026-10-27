package transmission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

func TestEventMarshal(t *testing.T) {
	e := &Event{SampleRate: 1}
	e.Data = map[string]interface{}{
		"a": int64(1),
		"b": float64(1.0),
		"c": true,
		"d": "foo",
		"e": time.Microsecond,
		"f": struct {
			Foo int64 `json:"f"`
		}{1},
		"g": map[string]interface{}{
			"g": 1,
		},
	}
	b, err := json.Marshal(e)
	testOK(t, err)
	testEquals(t, string(b), `{"data":{"a":1,"b":1,"c":true,"d":"foo","e":1000,"f":{"f":1},"g":{"g":1}}}`)

	e.Timestamp = time.Unix(1476309645, 0).UTC()
	e.SampleRate = 5
	b, err = json.Marshal(e)
	testOK(t, err)
	testEquals(t, string(b), `{"data":{"a":1,"b":1,"c":true,"d":"foo","e":1000,"f":{"f":1},"g":{"g":1}},"samplerate":5,"time":"2016-10-12T22:00:45Z"}`)

	var buf bytes.Buffer
	err = msgpack.NewEncoder(&buf).Encode(e)
	testOK(t, err)

	var decoded interface{}
	err = msgpack.NewDecoder(&buf).Decode(&decoded)
	testOK(t, err)
	localTime := e.Timestamp.Local()

	testEquals(t, decoded, map[string]interface{}{
		"time":       localTime,
		"samplerate": int8(5),
		"data": map[string]interface{}{
			"a": int64(1),
			"b": float64(1.0),
			"c": true,
			"d": "foo",
			"e": int64(time.Microsecond),
			"f": map[string]interface{}{
				"f": int64(1),
			},
			"g": map[string]interface{}{
				"g": int8(1),
			},
		},
	})
}

func TestEventMarshalMsgpackSortsKeys(t *testing.T) {
	e := &Event{Data: map[string]interface{}{}}
	for i := 0; i < 50; i++ {
		e.Data[fmt.Sprintf("field.%02d", i)] = i
		e.Data[fmt.Sprintf("global.%02d", i)] = fmt.Sprintf("value-%d", i)
	}
	e.Data["nested"] = map[string]interface{}{"z": 1, "y": 2, "x": 3}

	first, err := e.MarshalMsgpack()
	testOK(t, err)
	for i := 0; i < 20; i++ {
		again, err := e.MarshalMsgpack()
		testOK(t, err)
		testEquals(t, again, first)
	}

	dec := msgpack.NewDecoder(bytes.NewReader(first))
	n, err := dec.DecodeMapLen()
	testOK(t, err)
	testEquals(t, n, 1)
	key, err := dec.DecodeString()
	testOK(t, err)
	testEquals(t, key, "data")
	n, err = dec.DecodeMapLen()
	testOK(t, err)
	testEquals(t, n, len(e.Data))
	var keys []string
	for i := 0; i < n; i++ {
		k, err := dec.DecodeString()
		testOK(t, err)
		keys = append(keys, k)
		testOK(t, dec.Skip())
	}
	testEquals(t, sort.StringsAreSorted(keys), true)
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
			err := msgpack.NewEncoder(&buf).Encode(&evt)
			testOK(b, err)
		}
	})
}
