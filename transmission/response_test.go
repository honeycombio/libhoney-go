package transmission

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

// response struct sent from API
type responseInBatch struct {
	ErrorStr string `json:"error,omitempty"`
	Status   int    `json:"status,omitempty"`
}

var (
	srcBatchResponseSingle = []responseInBatch{
		{ErrorStr: "something bad happened", Status: 500},
	}
	srcBatchResponseMultiple = []responseInBatch{
		{ErrorStr: "something bad happened", Status: 500},
		{Status: 202},
	}
)

func TestUnmarshalJSONResponse(t *testing.T) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(srcBatchResponseMultiple)
	testOK(t, err)

	var responses []Response
	err = json.NewDecoder(buf).Decode(&responses)
	testOK(t, err)

	testEquals(t, responses[0].StatusCode, 500)
	testEquals(t, responses[0].Err.Error(), "something bad happened")
	testEquals(t, responses[1].StatusCode, 202)
	testEquals(t, responses[1].Err, nil)
}

func TestUnmarshalJSONResponseSingle(t *testing.T) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(srcBatchResponseSingle)
	testOK(t, err)

	var responses []Response
	err = json.NewDecoder(buf).Decode(&responses)
	testOK(t, err)

	testEquals(t, responses[0].StatusCode, 500)
	testEquals(t, responses[0].Err.Error(), "something bad happened")
}

func TestUnmarshalMsgpackResponse(t *testing.T) {
	buf := &bytes.Buffer{}
	encoder := msgpack.NewEncoder(buf)
	encoder.SetCustomStructTag("json")
	err := encoder.Encode(srcBatchResponseMultiple)
	testOK(t, err)

	var responses []Response
	err = msgpack.NewDecoder(buf).Decode(&responses)
	testOK(t, err)

	testEquals(t, responses[0].StatusCode, 500)
	testEquals(t, responses[0].Err.Error(), "something bad happened")
	testEquals(t, responses[1].StatusCode, 202)
	testEquals(t, responses[1].Err, nil)
}

func TestUnmarshalMsgpackResponseSingle(t *testing.T) {
	buf := &bytes.Buffer{}
	encoder := msgpack.NewEncoder(buf)
	encoder.SetCustomStructTag("json")
	err := encoder.Encode(srcBatchResponseSingle)
	testOK(t, err)

	var responses []Response
	err = msgpack.NewDecoder(buf).Decode(&responses)
	testOK(t, err)

	testEquals(t, responses[0].StatusCode, 500)
	testEquals(t, responses[0].Err.Error(), "something bad happened")
}
