package transmission

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/tinylib/msgp/msgp"
)

// Response is a record of an event sent. It includes information about sending
// the event - how long it took, whether it succeeded, and so on. It also has a
// metadata field that is just a pass through - populate an Event's Metadata
// field and what you put there will be on the Response that corresponds to
// that Event. This allows you to track specific events.
type Response struct {

	// Err contains any error returned by the httpClient on sending or an error
	// indicating queue overflow
	Err error

	// StatusCode contains the HTTP Status Code returned by the Honeycomb API
	// server
	StatusCode int

	// Body is the body of the HTTP response from the Honeycomb API server.
	Body []byte

	// Duration is a measurement of how long the HTTP request to send an event
	// took to process. The actual time it takes libhoney to send an event may
	// be longer due to any time the event spends waiting in the queue before
	// being sent.
	Duration time.Duration

	// Metadata is whatever content you put in the Metadata field of the event for
	// which this is the response. It is passed through unmodified.
	Metadata interface{}
}

func (r *Response) UnmarshalJSON(b []byte) error {
	aux := struct {
		Error  string
		Status int
	}{}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	r.StatusCode = aux.Status
	if aux.Error != "" {
		r.Err = errors.New(aux.Error)
	}
	return nil
}

type responseSlice []Response

func (r *responseSlice) EncodeMsg(w *msgp.Writer) error {
	v := make(MsgpackResponseSlice, len(*r))
	for i, resp := range *r {
		v[i].Status = resp.StatusCode
		if resp.Err != nil {
			v[i].ErrorStr = resp.Err.Error()
		}
	}

	return v.EncodeMsg(w)
}

func (r *responseSlice) DecodeMsg(reader *msgp.Reader) error {
	var v MsgpackResponseSlice
	err := v.DecodeMsg(reader)
	if err != nil {
		return nil
	}

	*r = make(responseSlice, len(v))
	for i := range v {
		(*r)[i] = Response{
			StatusCode: v[i].Status,
		}
		if v[i].ErrorStr != "" {
			(*r)[i].Err = errors.New(v[i].ErrorStr)
		}
	}
	return nil
}

// writeToResponse adds the response to the response queue. Returns true if it
// dropped the response because it's set to not block on the queue being full
// and the queue was full.
func writeToResponse(responses chan Response, resp Response, block bool) (dropped bool) {
	if block {
		responses <- resp
	} else {
		select {
		case responses <- resp:
		default:
			return true
		}
	}
	return false
}
