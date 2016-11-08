package libhoney

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	placeholder = Response{StatusCode: http.StatusTeapot}
)

func TestTxAdd(t *testing.T) {
	dc := &txDefaultClient{}
	dc.muster.Work = make(chan interface{}, 1)
	responses = make(chan Response, 1)
	responses <- placeholder

	// default successful case
	e := &Event{Metadata: "mmeetta"}
	dc.Add(e)
	added := <-dc.muster.Work
	testEquals(t, e, added)
	rsp := testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp, "work was simply queued; no response available yet")

	// make the queue 0 length to force an overflow
	dc.muster.Work = make(chan interface{}, 0)
	dc.Add(e)
	rsp = testGetResponse(t, responses)
	testErr(t, rsp.Err)
	testEquals(t, rsp.Err.Error(), "queue overflow",
		"overflow error should have been put on responses channel immediately")
	// make sure that (default) nonblocking on responses allows execution even if
	// responses channel is full
	responses <- placeholder
	dc.Add(e)
	rsp = testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp,
		"placeholder was blocking responses channel but .Add should have continued")

	// test blocking on send still gets it down the channel
	dc.blockOnSend = true
	dc.muster.Work = make(chan interface{}, 1)
	responses <- placeholder

	dc.Add(e)
	added = <-dc.muster.Work
	testEquals(t, e, added)
	rsp = testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp, "blockOnSend doesn't affect the responses queue")

	// test blocking on response still gets an overflow down the channel
	dc.blockOnSend = false
	dc.blockOnResponses = true
	dc.muster.Work = make(chan interface{}, 0)

	responses <- placeholder
	go dc.Add(e)
	rsp = testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp, "should pull placeholder response off channel first")
	rsp = testGetResponse(t, responses)
	testErr(t, rsp.Err)
	testEquals(t, rsp.Err.Error(), "queue overflow",
		"overflow error should have been pushed into channel")
}

type FakeRoundTripper struct {
	req     *http.Request
	reqBody string
	resp    *http.Response
	respErr error
}

func (f *FakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	f.req = r
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	f.reqBody = string(bodyBytes)
	return f.resp, f.respErr
}

type testNotifier struct{}

func (tn *testNotifier) Done() {}

// test the mechanics of sending / receiving responses
func TestTxSendSingle(t *testing.T) {
	responses = make(chan Response, 1)
	frt := &FakeRoundTripper{}
	b := &batch{
		httpClient:  &http.Client{Transport: frt},
		testNower:   &fakeNower{},
		testBlocker: &sync.WaitGroup{},
	}
	reset := func(b *batch, frt *FakeRoundTripper, body string, err error) {
		if body == "" {
			frt.resp = nil
		} else {
			frt.resp = &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(body)),
			}
		}
		frt.respErr = err
		b.events = nil
		b.numEncoded = 0
	}

	fhData := map[string]interface{}{"foo": "bar"}
	e := &Event{
		fieldHolder: fieldHolder{data: fhData},
		SampleRate:  4,
		APIHost:     "http://fakeHost:8080/",
		WriteKey:    "written",
		Dataset:     "ds1",
		Metadata:    "emmetta",
	}
	reset(b, frt, `{"ds1":[{"status":202}]}`, nil)
	b.Add(e)
	b.Fire(&testNotifier{})
	expectedURL := fmt.Sprintf("%s/1/batch", e.APIHost)
	testEquals(t, frt.req.URL.String(), expectedURL)
	versionedUserAgent := fmt.Sprintf("libhoney-go/%s", version)
	testEquals(t, frt.req.Header.Get("User-Agent"), versionedUserAgent)
	testEquals(t, frt.req.Header.Get("X-Honeycomb-Team"), e.WriteKey)
	buf := &bytes.Buffer{}
	g := gzip.NewWriter(buf)
	_, err := g.Write([]byte(`{"ds1":[{"data":{"foo":"bar"},"samplerate":4}]}`))
	testOK(t, err)
	testOK(t, g.Close())
	testEquals(t, frt.reqBody, buf.String())

	rsp := testGetResponse(t, responses)
	testEquals(t, rsp.Duration, time.Second*10)
	testEquals(t, rsp.Metadata, "emmetta")
	testEquals(t, rsp.StatusCode, 202)
	testOK(t, rsp.Err)

	// test UserAgentAddition
	UserAgentAddition = "  fancyApp/3 "
	expectedUserAgentAddition := "fancyApp/3"
	longUserAgent := fmt.Sprintf("%s %s", versionedUserAgent, expectedUserAgentAddition)
	reset(b, frt, `{"ds1":[{"status":202}]}`, nil)
	b.Add(e)
	b.Fire(&testNotifier{})
	testEquals(t, frt.req.Header.Get("User-Agent"), longUserAgent)
	rsp = testGetResponse(t, responses)
	testEquals(t, rsp.StatusCode, 202)
	testOK(t, rsp.Err)
	UserAgentAddition = ""

	// test unsuccessful send
	reset(b, frt, "", errors.New("testing error handling"))
	b.Add(e)
	b.Fire(&testNotifier{})
	rsp = testGetResponse(t, responses)
	testErr(t, rsp.Err)
	testEquals(t, rsp.StatusCode, 0)
	testEquals(t, len(rsp.Body), 0)

	// test nonblocking response path is actually nonblocking, drops response
	responses <- placeholder
	reset(b, frt, "", errors.New("err"))
	b.testBlocker.Add(1)
	b.Add(e)
	go b.Fire(&testNotifier{})
	b.testBlocker.Wait() // triggered on drop
	rsp = testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp,
		"should pull placeholder response and only placeholder response off channel")

	// test blocking response path, error
	b.blockOnResponses = true
	reset(b, frt, "", errors.New("err"))
	responses <- placeholder
	b.Add(e)
	go b.Fire(&testNotifier{})
	rsp = testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp,
		"should pull placeholder response off channel first")
	rsp = testGetResponse(t, responses)
	testErr(t, rsp.Err)
	testEquals(t, rsp.StatusCode, 0)
	testEquals(t, len(rsp.Body), 0)

	// test blocking response path, no error
	responses <- placeholder
	reset(b, frt, `{"ds1":[{"status":202}]}`, nil)
	b.Add(e)
	go b.Fire(&testNotifier{})
	rsp = testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp,
		"should pull placeholder response off channel first")
	rsp = testGetResponse(t, responses)
	testEquals(t, rsp.Duration, time.Second*10)
	testEquals(t, rsp.Metadata, "emmetta")
	testEquals(t, rsp.StatusCode, 202)
	testOK(t, rsp.Err)
}

// test the details of handling batch behavior
func TestTxSendBatch(t *testing.T) {
	tsts := []struct {
		in         []map[string]interface{} // datas
		expReqBody string
		response   string // JSON from server
		expected   []Response
	}{
		{
			[]map[string]interface{}{
				{"a": 1},
				{"b": 2, "bb": 22},
				{"c": 3.1},
			},
			`{"ds1":[{"data":{"a":1}},{"data":{"b":2,"bb":22}},{"data":{"c":3.1}}]}`,
			`{"ds1":[{"status":202},{"status":202},{"status":429,"error":"bratelimited"}]}`,
			[]Response{
				{StatusCode: 202, Metadata: "emmetta0"},
				{StatusCode: 202, Metadata: "emmetta1"},
				{Err: errors.New("bratelimited"), StatusCode: 429, Metadata: "emmetta2"},
			},
		},
		{
			[]map[string]interface{}{
				{"a": 1},
				{"b": func() {}},
				{"c": 3.1},
			},
			`{"ds1":[{"data":{"a":1}},{"data":{"c":3.1}}]}`,
			`{"ds1":[{"status":202},{"status":202}]}`,
			[]Response{ // specific order!
				{Err: errors.New("unsupported type"), Metadata: "emmetta1"},
				{StatusCode: 202, Metadata: "emmetta0"},
				{StatusCode: 202, Metadata: "emmetta2"},
			},
		},
	}

	frt := &FakeRoundTripper{
		resp: &http.Response{
			StatusCode: 200,
		},
	}

	for _, tt := range tsts {
		b := &batch{httpClient: &http.Client{Transport: frt}}
		responses = make(chan Response, len(tt.expected))
		frt.resp.Body = ioutil.NopCloser(strings.NewReader(tt.response))
		for i, data := range tt.in {
			b.Add(&Event{
				fieldHolder: fieldHolder{data: data},
				APIHost:     "fakeHost",
				WriteKey:    "written",
				Dataset:     "ds1",
				Metadata:    fmt.Sprint("emmetta", i), // tracking insertion order
			})
		}
		b.Fire(&testNotifier{})
		for _, expResp := range tt.expected {
			resp := testGetResponse(t, responses)
			testEquals(t, resp.StatusCode, expResp.StatusCode)
			testEquals(t, resp.Metadata, expResp.Metadata)
			if expResp.Err != nil {
				if !strings.Contains(resp.Err.Error(), expResp.Err.Error()) {
					t.Errorf("expected error to contain '%s', got: '%s'", expResp.Err.Error(), resp.Err.Error())
				}
			} else {
				testEquals(t, resp.Err, expResp.Err)
			}
		}
	}
}

func TestBatchMarshal(t *testing.T) {
	tsts := []struct {
		in          map[string][]map[string]interface{} // map of {dataset to list of events}
		niledEvents map[string][]bool
		expected    string
		errMsgs     []string
	}{
		{
			map[string][]map[string]interface{}{
				"alpha": {
					{"a": 1},
					{"b": func() {}},
					{"c": 3},
				},
				"beta": {
					{"d": make(chan string)},
					{"e": 5},
				},
				"gamma": {
					{"f": 6},
				},
			},
			map[string][]bool{
				"alpha": {false, true, false},
				"beta":  {true, false},
				"gamma": {false},
			},
			`{"alpha":[{"data":{"a":1}},{"data":{"c":3}}],"beta":[{"data":{"e":5}}],"gamma":[{"data":{"f":6}}]}`,
			[]string{"MarshalJSON", "MarshalJSON"},
		},
		{ // shouldn't happen based on how muster works, but just in case
			map[string][]map[string]interface{}{"nada": {}},
			map[string][]bool{"nada": []bool{}},
			"{}",
			nil,
		},
		{
			map[string][]map[string]interface{}{
				"nada": {
					{"a": func() {}},
					{"b": make(chan string)},
				},
				"zip": {
					{"c": func() {}},
				},
			},
			map[string][]bool{
				"nada": {true, true},
				"zip":  {true},
			},
			"{}",
			[]string{"MarshalJSON", "MarshalJSON", "MarshalJSON"},
		},
	}

	for _, tt := range tsts {
		b := &batch{blockOnResponses: true}
		for dName, datas := range tt.in {
			for _, d := range datas {
				b.Add(&Event{
					Dataset: dName,
					fieldHolder: fieldHolder{
						data: d,
					},
				})
			}
		}
		responses = make(chan Response, len(tt.errMsgs))

		encoded, err := json.Marshal(b)
		testOK(t, err)
		testEquals(t, string(encoded), tt.expected)
		for datasetName, nilList := range tt.niledEvents {
			for i, ev := range b.events[datasetName] {
				if nilList[i] != (ev == nil) {
					t.Errorf("expected %s event at index %d to be nil, but was %+v",
						datasetName, i, ev)
				}
			}
		}
		for _, errMsg := range tt.errMsgs {
			rsp := testGetResponse(t, responses)
			testErr(t, rsp.Err)
			if !strings.Contains(rsp.Err.Error(), errMsg) {
				t.Errorf("expected json encoding error, got: %s", rsp.Err.Error())
			}
		}
	}
}
