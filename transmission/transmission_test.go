package transmission

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	// Use a different zstd library from the implementation, for more
	// convincing testing.
	"github.com/DataDog/zstd"
	"gopkg.in/vmihailenco/msgpack.v4"
)

var (
	placeholder = Response{StatusCode: http.StatusTeapot}
)

type errReader struct{}

func (e errReader) Read(b []byte) (int, error) { return 0, errors.New("mystery read error!") }

type timeoutErr struct{}

func (t *timeoutErr) Error() string {
	return "timeout"
}

func (t *timeoutErr) Timeout() bool {
	return true
}

func TestEmptyHoneycombTransmission(t *testing.T) {
	// All fields on the Honeycomb transmission are optional; an empty honeycomb
	// transmission should work (if not very well because of zero length channels)
	tx := &Honeycomb{}
	tx.Start()
	tx.Add(&Event{
		APIKey:  "kiddly",
		Dataset: "diddly",
		APIHost: "doo",
	})
}

func TestHnyTxAdd(t *testing.T) {
	hnyTx := &Honeycomb{
		Logger:  &nullLogger{},
		Metrics: &nullMetrics{},
	}
	hnyTx.muster.Work = make(chan interface{}, 1)
	hnyTx.responses = make(chan Response, 1)
	hnyTx.responses <- placeholder
	hnyTx.responses = hnyTx.responses

	// default successful case
	e := &Event{Metadata: "mmeetta"}
	hnyTx.Add(e)
	added := <-hnyTx.muster.Work
	testEquals(t, e, added)
	rsp := testGetResponse(t, hnyTx.responses)
	testIsPlaceholderResponse(t, rsp, "work was simply queued; no response available yet")

	// make the queue 0 length to force an overflow
	hnyTx.muster.Work = make(chan interface{}, 0)
	hnyTx.Add(e)
	rsp = testGetResponse(t, hnyTx.responses)
	testErr(t, rsp.Err)
	testEquals(t, rsp.Err.Error(), "queue overflow",
		"overflow error should have been put on responses channel immediately")
	// make sure that (default) nonblocking on responses allows execution even if
	// responses channel is full
	hnyTx.responses <- placeholder
	hnyTx.Add(e)
	rsp = testGetResponse(t, hnyTx.responses)
	testIsPlaceholderResponse(t, rsp,
		"placeholder was blocking responses channel but .Add should have continued")

	// test blocking on send still gets it down the channel
	hnyTx.BlockOnSend = true
	hnyTx.muster.Work = make(chan interface{}, 1)
	hnyTx.responses <- placeholder

	hnyTx.Add(e)
	added = <-hnyTx.muster.Work
	testEquals(t, e, added)
	rsp = testGetResponse(t, hnyTx.responses)
	testIsPlaceholderResponse(t, rsp, "BlockOnSend doesn't affect the responses queue")

	// test blocking on response still gets an overflow down the channel
	hnyTx.BlockOnSend = false
	hnyTx.BlockOnResponse = true
	hnyTx.muster.Work = make(chan interface{}, 0)

	hnyTx.responses <- placeholder
	go hnyTx.Add(e)
	rsp = testGetResponse(t, hnyTx.responses)
	testIsPlaceholderResponse(t, rsp, "should pull placeholder response off channel first")
	rsp = testGetResponse(t, hnyTx.responses)
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

	// Honeycomb servers response to msgpack requests with msgpack responses,
	// but for convenience our tests speak json. Translate as needed.
	if r.Header.Get("Content-Type") == "application/msgpack" &&
		f.resp != nil &&
		f.resp.Body != nil &&
		(f.resp.Header == nil || f.resp.Header.Get("Content-Type") != "application/msgpack") {
		var v interface{}
		switch {
		case f.resp.StatusCode != http.StatusOK:
			v = &Response{}
		case strings.Contains(r.URL.Path, "/1/batch"):
			v = &[]Response{}
		default:
			panic(fmt.Sprintf("unhandled path: %s", r.URL.Path))
		}
		err := json.NewDecoder(f.resp.Body).Decode(&v)
		if err == nil {
			var buf bytes.Buffer
			err = msgpack.NewEncoder(&buf).Encode(&v)
			if err != nil {
				return nil, err
			}

			f.resp.Body = ioutil.NopCloser(&buf)
			if f.resp.Header == nil {
				f.resp.Header = http.Header{}
			}
			f.resp.Header.Set("Content-Type", "application/msgpack")
		}
	}

	return f.resp, f.respErr
}

type OneTimeoutRountTripper struct {
	*FakeRoundTripper
	doTimeout bool
}

func (f *OneTimeoutRountTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if f.doTimeout {
		f.doTimeout = false
		return nil, &timeoutErr{}
	}
	return f.FakeRoundTripper.RoundTrip(r)
}

type testNotifier struct{}

func (tn *testNotifier) Done() {}

// test the mechanics of sending / receiving responses
func TestTxSendSingle(t *testing.T) {
	var doMsgpack bool
	withJSONAndMsgpack(t, &doMsgpack, func(t *testing.T) {
		frt := &FakeRoundTripper{}
		b := &batchAgg{
			httpClient:            &http.Client{Transport: frt},
			testNower:             &fakeNower{},
			testBlocker:           &sync.WaitGroup{},
			responses:             make(chan Response, 1),
			metrics:               &nullMetrics{},
			enableMsgpackEncoding: doMsgpack,
		}
		Version = "1.2.3"
		reset := func(b *batchAgg, frt *FakeRoundTripper, statusCode int, body string, err error) {
			if body == "" {
				frt.resp = nil
			} else {
				frt.resp = &http.Response{
					StatusCode: statusCode,
					Body:       ioutil.NopCloser(strings.NewReader(body)),
				}
			}
			frt.respErr = err
			b.batches = nil
		}

		fhData := map[string]interface{}{"foo": "bar"}
		e := &Event{
			Data:       fhData,
			SampleRate: 4,
			APIHost:    "http://fakeHost:8080",
			APIKey:     "written",
			Dataset:    "ds1",
			Metadata:   "emmetta",
		}
		reset(b, frt, 200, `[{"status":202}]`, nil)
		b.Add(e)
		b.Fire(&testNotifier{})
		expectedURL := fmt.Sprintf("%s/1/batch/%s", e.APIHost, e.Dataset)
		testEquals(t, frt.req.URL.String(), expectedURL)
		versionedUserAgent := fmt.Sprintf("libhoney-go/%s", Version)
		testEquals(t, frt.req.Header.Get("User-Agent"), versionedUserAgent)
		testEquals(t, frt.req.Header.Get("X-Honeycomb-Team"), e.APIKey)
		buf := &bytes.Buffer{}
		g := zstd.NewWriter(buf)
		if doMsgpack {
			v := []*Event{{
				SampleRate: 4,
				Data: map[string]interface{}{
					"foo": "bar",
				},
			}}
			var buf bytes.Buffer
			err := msgpack.NewEncoder(&buf).UseJSONTag(true).Encode(&v)
			testOK(t, err)
			_, err = g.Write(buf.Bytes())
			testOK(t, err)
		} else {
			_, err := g.Write([]byte(`[{"data":{"foo":"bar"},"samplerate":4}]`))
			testOK(t, err)
		}
		testOK(t, g.Close())

		actual, err := zstd.Decompress(nil, []byte(frt.reqBody))
		testOK(t, err)
		expected, err := zstd.Decompress(nil, buf.Bytes())
		testOK(t, err)
		testEquals(t, actual, expected)

		rsp := testGetResponse(t, b.responses)
		testEquals(t, rsp.Duration, time.Second*10)
		testEquals(t, rsp.Metadata, "emmetta")
		testEquals(t, rsp.StatusCode, 202)
		testOK(t, rsp.Err)

		// test UserAgentAddition
		b.userAgentAddition = "  fancyApp/3 "
		expectedUserAgentAddition := "fancyApp/3"
		longUserAgent := fmt.Sprintf("%s %s", versionedUserAgent, expectedUserAgentAddition)
		reset(b, frt, 200, `[{"status":202}]`, nil)
		b.Add(e)
		b.Fire(&testNotifier{})
		testEquals(t, frt.req.Header.Get("User-Agent"), longUserAgent)
		rsp = testGetResponse(t, b.responses)
		testEquals(t, rsp.StatusCode, 202)
		testOK(t, rsp.Err)
		b.userAgentAddition = ""

		// test unsuccessful send
		reset(b, frt, 0, "", errors.New("testing error handling"))
		b.Add(e)
		b.Fire(&testNotifier{})
		rsp = testGetResponse(t, b.responses)
		testErr(t, rsp.Err)
		testEquals(t, rsp.StatusCode, 0)
		testEquals(t, len(rsp.Body), 0)

		// test repeated http timeout
		reset(b, frt, 0, "", &timeoutErr{})
		b.Add(e)
		b.Fire(&testNotifier{})
		rsp = testGetResponse(t, b.responses)
		testErr(t, rsp.Err)
		testEquals(t, rsp.StatusCode, 0)
		testEquals(t, len(rsp.Body), 0)

		// test nonblocking response path is actually nonblocking, drops response
		b.responses <- placeholder
		reset(b, frt, 0, "", errors.New("err"))
		b.testBlocker.Add(1)
		b.Add(e)
		go b.Fire(&testNotifier{})
		b.testBlocker.Wait() // triggered on drop
		rsp = testGetResponse(t, b.responses)
		testIsPlaceholderResponse(t, rsp,
			"should pull placeholder response and only placeholder response off channel")

		// test blocking response path, error
		b.blockOnResponse = true
		reset(b, frt, 0, "", errors.New("err"))
		b.responses <- placeholder
		b.Add(e)
		go b.Fire(&testNotifier{})
		rsp = testGetResponse(t, b.responses)
		testIsPlaceholderResponse(t, rsp,
			"should pull placeholder response off channel first")
		rsp = testGetResponse(t, b.responses)
		testErr(t, rsp.Err)
		testEquals(t, rsp.StatusCode, 0)
		testEquals(t, len(rsp.Body), 0)

		// test blocking response path, request completed but got HTTP error code
		b.blockOnResponse = true
		reset(b, frt, 400, `{"error":"unknown Team key - check your credentials"}`, nil)
		b.responses <- placeholder
		b.Add(e)
		go b.Fire(&testNotifier{})
		rsp = testGetResponse(t, b.responses)
		testIsPlaceholderResponse(t, rsp,
			"should pull placeholder response off channel first")
		rsp = testGetResponse(t, b.responses)
		testEquals(t, rsp.StatusCode, 400)
		testEquals(t, string(rsp.Body), `{"error":"unknown Team key - check your credentials"}`)

		// test the case that our POST request completed, we got an HTTP error
		// code, but then got an error reading HTTP response body. An unlikely
		// scenario but technically possible.
		b.blockOnResponse = true
		frt.resp = &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(errReader{}),
		}
		frt.respErr = nil
		b.batches = nil
		b.responses <- placeholder
		b.Add(e)
		go b.Fire(&testNotifier{})
		rsp = testGetResponse(t, b.responses)
		testIsPlaceholderResponse(t, rsp,
			"should pull placeholder response off channel first")
		rsp = testGetResponse(t, b.responses)
		testEquals(t, rsp.Err, errors.New("Got HTTP error code but couldn't read response body: mystery read error!"))

		// Some error statuses may come back with no body at all, so we should
		// attach our own error.
		frt.resp = &http.Response{
			StatusCode: 504,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte{})),
		}
		frt.respErr = nil
		b.batches = nil
		b.Add(e)
		go b.Fire(&testNotifier{})
		rsp = testGetResponse(t, b.responses)
		testEquals(t, rsp.Err, errors.New("got unexpected HTTP status 504: Gateway Timeout"))

		// test blocking response path, no error
		b.responses <- placeholder
		reset(b, frt, 200, `[{"status":202}]`, nil)
		b.Add(e)
		go b.Fire(&testNotifier{})
		rsp = testGetResponse(t, b.responses)
		testIsPlaceholderResponse(t, rsp,
			"should pull placeholder response off channel first")
		rsp = testGetResponse(t, b.responses)
		testEquals(t, rsp.Duration, time.Second*10)
		testEquals(t, rsp.Metadata, "emmetta")
		testEquals(t, rsp.StatusCode, 202)
		testOK(t, rsp.Err)

		// test single http timeout
		reset(b, frt, 200, `[{"status":202}]`, nil)
		b.httpClient.Transport = &OneTimeoutRountTripper{
			FakeRoundTripper: frt,
			doTimeout:        true,
		}
		b.Add(e)
		b.Fire(&testNotifier{})
		rsp = testGetResponse(t, b.responses)
		testOK(t, rsp.Err)
		testEquals(t, rsp.StatusCode, 202)
	})
}

// test the details of handling batch behavior on a batch with a single dataset
func TestTxSendBatchSingleDataset(t *testing.T) {
	tsts := []struct {
		in       []map[string]interface{} // datas
		response string                   // JSON from server
		expected []Response
	}{
		{
			[]map[string]interface{}{
				{"a": 1},
				{"b": 2, "bb": 22},
				{"c": 3.1},
			},
			`[{"status":202},{"status":202},{"status":429,"error":"bratelimited"}]`,
			[]Response{
				{StatusCode: 202, Metadata: "emmetta0"},
				{StatusCode: 202, Metadata: "emmetta1"},
				{Err: errors.New("bratelimited"), StatusCode: 429, Metadata: "emmetta2"},
			},
		},
		{
			[]map[string]interface{}{
				{"a": 1},
				{"b": nil},
				{"c": 3.1},
			},
			`[{"status":202},{"status":202},{"status":202}]`,
			[]Response{
				{StatusCode: 202, Metadata: "emmetta0"},
				{StatusCode: 202, Metadata: "emmetta1"},
				{StatusCode: 202, Metadata: "emmetta2"},
			},
		},
	}

	var doMsgpack bool
	withJSONAndMsgpack(t, &doMsgpack, func(t *testing.T) {
		for _, tt := range tsts {
			frt := &FakeRoundTripper{
				resp: &http.Response{
					StatusCode: 200,
				},
			}

			b := &batchAgg{
				httpClient:            &http.Client{Transport: frt},
				responses:             make(chan Response, len(tt.expected)),
				metrics:               &nullMetrics{},
				enableMsgpackEncoding: doMsgpack,
			}
			frt.resp.Body = ioutil.NopCloser(strings.NewReader(tt.response))
			for i, data := range tt.in {
				b.Add(&Event{
					Data:     data,
					APIHost:  "fakeHost",
					APIKey:   "written",
					Dataset:  "ds1",
					Metadata: fmt.Sprint("emmetta", i), // tracking insertion order
				})
			}
			b.Fire(&testNotifier{})
			for _, expResp := range tt.expected {
				resp := testGetResponse(t, b.responses)
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
	})
}

// FancyFakeRoundTripper gets built with a map of incoming URL/Header components
// to the body that's expected and the response that's appropriate for that
// request. It'll send a different response depending on what it gets as well as
// error if the body was wrong
type FancyFakeRoundTripper struct {
	req       *http.Request
	reqBody   string
	reqBodies map[string]string
	resp      *http.Response

	// map of request apihost/writekey/dataset to intended response
	respBody   string
	respBodies map[string]string
	respErr    error
}

func (f *FancyFakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	var bodyFound, responseFound bool
	f.req = r
	for reqHeader, reqBody := range f.reqBodies {
		// respHeader is apihost,writekey,dataset
		headerKeys := strings.Split(reqHeader, ",")
		expectedURL, _ := url.Parse(fmt.Sprintf("%s/1/batch/%s", headerKeys[0], headerKeys[2]))
		if r.Header.Get("X-Honeycomb-Team") == headerKeys[1] && r.URL.String() == expectedURL.String() {
			bodyBytes, _ := ioutil.ReadAll(r.Body)
			f.reqBody = string(bodyBytes)

			// make sure body is legitimately compressed json
			if r.Header.Get("Content-Encoding") == "zstd" {
				var err error
				bodyBytes, err = zstd.Decompress(nil, bodyBytes)
				if err != nil {
					return nil, err
				}
			}

			var decoded interface{}
			if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
				return nil, err
			}

			f.resp.Body = ioutil.NopCloser(strings.NewReader(reqBody))
			bodyFound = true
			break
		}
	}
	if !bodyFound {
		f.resp.Body = ioutil.NopCloser(strings.NewReader(`{"error":"ffrt body not found"}`))
		return f.resp, f.respErr
	}
	for respHeader, respBody := range f.respBodies {
		// respHeader is apihost,writekey,dataset
		headerKeys := strings.Split(respHeader, ",")
		expectedURL, _ := url.Parse(fmt.Sprintf("%s/1/batch/%s", headerKeys[0], headerKeys[2]))
		if r.Header.Get("X-Honeycomb-Team") == headerKeys[1] && r.URL.String() == expectedURL.String() {
			f.resp.Body = ioutil.NopCloser(strings.NewReader(respBody))
			responseFound = true
			break
		}
	}
	if !responseFound {
		f.resp.Body = ioutil.NopCloser(strings.NewReader(`{"error":"ffrt response not found"}`))
	}
	return f.resp, f.respErr
}

// batch behavior on a batch with a multiple datasets/writekeys/apihosts
func TestTxSendBatchMultiple(t *testing.T) {
	tsts := []struct {
		in           map[string][]map[string]interface{} // datas
		expReqBodies map[string]string
		respBodies   map[string]string // JSON from server
		expected     map[string]Response
	}{
		{
			map[string][]map[string]interface{}{
				"ah1,wk1,ds1": {
					{"a": 1},
					{"b": 2, "bb": 22},
					{"c": 3.1},
				},
				"ah1,wk1,ds2": {
					{"a": 12},
					{"b": 22, "bb": 33},
					{"c": 39.2},
				},
				"ah3,wk3,ds3": {
					{"a": 32},
					{"b": 32, "bb": 39},
					{"c": 3.8},
				},
			},
			map[string]string{
				"ah1,wk1,ds1": `[{"data":{"a":1}},{"data":{"b":2,"bb":22}},{"data":{"c":3.1}}]`,
				"ah1,wk1,ds2": `[{"data":{"a":12}},{"data":{"b":22,"bb":33}},{"data":{"c":39.2}}]`,
				"ah3,wk3,ds3": `[{"data":{"a":32}},{"data":{"b":32,"bb":39}},{"data":{"c":3.8}}]`,
			},
			map[string]string{
				"ah1,wk1,ds1": `[{"status":202},{"status":203},{"status":204}]`,
				"ah1,wk1,ds2": `[{"status":202},{"status":202},{"status":429,"error":"bratelimited"}]`,
				"ah3,wk3,ds3": `[{"status":200},{"status":201},{"status":202}]`,
			},
			map[string]Response{
				"emmetta0": {StatusCode: 202, Metadata: "emmetta0"},
				"emmetta1": {StatusCode: 203, Metadata: "emmetta1"},
				"emmetta2": {StatusCode: 204, Metadata: "emmetta2"},
				"emmetta3": {StatusCode: 202, Metadata: "emmetta3"},
				"emmetta4": {StatusCode: 202, Metadata: "emmetta4"},
				"emmetta5": {Err: errors.New("bratelimited"), StatusCode: 429, Metadata: "emmetta5"},
				"emmetta6": {StatusCode: 200, Metadata: "emmetta6"},
				"emmetta7": {StatusCode: 201, Metadata: "emmetta7"},
				"emmetta8": {StatusCode: 202, Metadata: "emmetta8"},
			},
		},
	}

	ffrt := &FancyFakeRoundTripper{
		resp: &http.Response{
			StatusCode: 200,
		},
	}

	for _, tt := range tsts {
		b := &batchAgg{
			httpClient:            &http.Client{Transport: ffrt},
			responses:             make(chan Response, len(tt.expected)),
			metrics:               &nullMetrics{},
			enableMsgpackEncoding: false,
		}
		ffrt.reqBodies = tt.expReqBodies
		ffrt.respBodies = tt.respBodies
		// insert events in sorted order to check responses
		keys := []string{}
		for k := range tt.in {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var i int // counter to identify response metadata
		for _, k := range keys {
			headers := strings.Split(k, ",")
			for _, data := range tt.in[k] {
				b.Add(&Event{
					Data:     data,
					APIHost:  headers[0],
					APIKey:   headers[1],
					Dataset:  headers[2],
					Metadata: fmt.Sprint("emmetta", i), // tracking insertion order
				})
				i++
			}
		}
		b.Fire(&testNotifier{})
		// go through the right number of expected responses, look for matches
		for range tt.expected {
			var found bool
			resp := testGetResponse(t, b.responses)
			for meta, expResp := range tt.expected {
				if meta == resp.Metadata {
					found = true
					testEquals(t, resp.StatusCode, expResp.StatusCode)
					if expResp.Err != nil {
						if !strings.Contains(resp.Err.Error(), expResp.Err.Error()) {
							t.Errorf("expected error to contain '%s', got: '%s'", expResp.Err.Error(), resp.Err.Error())
						}
					} else {
						testEquals(t, resp.Err, expResp.Err)
					}
				}
			}
			if !found {
				t.Errorf("couldn't find expected response for %+v\n", resp)
			}
		}
	}
}

func TestRenqueueEventsAfterOverflow(t *testing.T) {
	var doMsgpack bool
	withJSONAndMsgpack(t, &doMsgpack, func(t *testing.T) {
		frt := &FakeRoundTripper{}
		b := &batchAgg{
			httpClient:            &http.Client{Transport: frt},
			testNower:             &fakeNower{},
			responses:             make(chan Response, 1),
			metrics:               &nullMetrics{},
			enableMsgpackEncoding: doMsgpack,
		}

		events := make([]*Event, 100)
		// we make the event bodies 99KB to allow for the column name and sampleRate/Timestamp
		// payload
		fhData := map[string]interface{}{"reallyBigColumn": randomString(99 * 1000)}
		for i := range events {
			events[i] = &Event{
				Data:       fhData,
				SampleRate: 4,
				APIHost:    "http://fakeHost:8080",
				APIKey:     "written",
				Dataset:    "ds1",
				Metadata:   "emmetta",
			}
		}

		reset := func(b *batchAgg, frt *FakeRoundTripper, statusCode int, body string, err error) {
			if body == "" {
				frt.resp = nil
			} else {
				frt.resp = &http.Response{
					StatusCode: statusCode,
					Body:       ioutil.NopCloser(strings.NewReader(body)),
				}
			}
			frt.respErr = err
			b.batches = nil
		}

		key := "http://fakeHost:8080_written_ds1"

		reset(b, frt, 200, `[{"status":202}]`, nil)
		b.fireBatch(events)
		testEquals(t, len(b.overflowBatches), 1)
		testEquals(t, len(b.overflowBatches[key]), 50)
	})
}

type testRoundTripper struct {
	callCount int
}

func (t *testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	t.callCount++

	ioutil.ReadAll(r.Body)

	return &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(strings.NewReader(`[{"status":202}]`)),
	}, nil
}

// Verify that events over the batch size limit are requeued and sent
func TestFireBatchLargeEventsSent(t *testing.T) {
	var doMsgpack bool
	withJSONAndMsgpack(t, &doMsgpack, func(t *testing.T) {
		trt := &testRoundTripper{}
		b := &batchAgg{
			httpClient:            &http.Client{Transport: trt},
			testNower:             &fakeNower{},
			responses:             make(chan Response, 1),
			metrics:               &nullMetrics{},
			enableMsgpackEncoding: doMsgpack,
		}

		events := make([]*Event, 150)
		fhData := map[string]interface{}{"reallyBigColumn": randomString(99 * 1000)}
		for i := range events {
			events[i] = &Event{
				Data:       fhData,
				SampleRate: 4,
				APIHost:    "http://fakeHost:8080",
				APIKey:     "written",
				Dataset:    "ds1",
				Metadata:   "emmetta",
			}
			b.Add(events[i])
		}

		key := "http://fakeHost:8080_written_ds1"

		b.Fire(&testNotifier{})
		testEquals(t, len(b.overflowBatches), 0)
		testEquals(t, len(b.overflowBatches[key]), 0)
		testEquals(t, trt.callCount, 3)
	})
}

// Ensure we handle events greater than the limit by enqueuing a response
func TestFireBatchWithTooLargeEvent(t *testing.T) {
	var doMsgpack bool
	withJSONAndMsgpack(t, &doMsgpack, func(t *testing.T) {
		trt := &testRoundTripper{}
		b := &batchAgg{
			httpClient:            &http.Client{Transport: trt},
			testNower:             &fakeNower{},
			testBlocker:           &sync.WaitGroup{},
			responses:             make(chan Response, 1),
			metrics:               &nullMetrics{},
			enableMsgpackEncoding: doMsgpack,
		}

		events := make([]*Event, 1)
		for i := range events {
			fhData := map[string]interface{}{"reallyREALLYBigColumn": randomString(1024 * 1024)}
			events[i] = &Event{
				Data:       fhData,
				SampleRate: 4,
				APIHost:    "http://fakeHost:8080",
				APIKey:     "written",
				Dataset:    "ds1",
				Metadata:   fmt.Sprintf("meta %d", i),
			}
			b.Add(events[i])
		}

		key := "http://fakeHost:8080_written_ds1"

		b.Fire(&testNotifier{})
		b.testBlocker.Wait()
		resp := testGetResponse(t, b.responses)
		testEquals(t, resp.Err.Error(), "event exceeds max event size of 100000 bytes, API will not accept this event")

		testEquals(t, len(b.overflowBatches), 0)
		testEquals(t, len(b.overflowBatches[key]), 0)
		testEquals(t, trt.callCount, 0)
	})
}

// Ensure we can deal with batches whose first event won't json encode
func TestFireBatchWithBrokenFirstEvent(t *testing.T) {
	var doMsgpack bool
	withJSONAndMsgpack(t, &doMsgpack, func(t *testing.T) {
		trt := &testRoundTripper{}
		b := &batchAgg{
			httpClient:            &http.Client{Transport: trt},
			testNower:             &fakeNower{},
			testBlocker:           &sync.WaitGroup{},
			responses:             make(chan Response, 2),
			metrics:               &nullMetrics{},
			enableMsgpackEncoding: doMsgpack,
		}

		// add two events, a broken and a valid.
		b.Add(&Event{
			Data:       map[string]interface{}{"reallyREALLYBigColumn": randomString(1024 * 1024)},
			SampleRate: 1,
			APIHost:    "http://fakeHost:8080",
			APIKey:     "written",
			Dataset:    "ds1",
			Metadata:   fmt.Sprintf("meta %d", 0),
		})
		b.Add(&Event{
			Data:       map[string]interface{}{"all_good_data": "tast"},
			SampleRate: 1,
			APIHost:    "http://fakeHost:8080",
			APIKey:     "written",
			Dataset:    "ds1",
			Metadata:   fmt.Sprintf("meta %d", 1),
		})

		b.Fire(&testNotifier{})
		b.testBlocker.Wait()
		resp := testGetResponse(t, b.responses)
		testEquals(t, resp.Metadata, "meta 0")
		testEquals(t, resp.Err.Error(), "event exceeds max event size of 100000 bytes, API will not accept this event")
		resp = testGetResponse(t, b.responses)
		testEquals(t, resp.Metadata, "meta 1")
		testEquals(t, resp.StatusCode, 202)

		testEquals(t, len(b.overflowBatches), 0)
		testEquals(t, trt.callCount, 1)
	})
}

func TestHoneycombSenderAddingResponsesBlocking(t *testing.T) {
	// this test has a few timeout checks. don't wait to run other tests.
	t.Parallel()
	// using the public SendRespanse method should add the response to the queue
	// while honoring the block setting
	w := &Honeycomb{
		BlockOnResponse:     true,
		PendingWorkCapacity: 1, // pwc of 1 means response queue size of 2
		Logger:              &nullLogger{},
		Metrics:             &nullMetrics{},
	}
	w.Start()
	ev := &Event{
		Metadata: "adder",
	}

	happenings := make(chan interface{}, 2)

	// push two things in the queue. This should fill it up; the next call should block.
	w.Add(ev)
	w.Add(ev)
	go func() {
		// indicate that we've successfully started the goroutine
		happenings <- struct{}{}
		// push a second event in the responses queue.  This should block
		w.Add(ev)
		// indicate that we've gotten past the blocking portion
		happenings <- struct{}{}
	}()

	// Ok, now we have a situation where:
	// * we can block until something comes in to happeninings, to confirm we started the goroutine
	// * we can wait a bit on happenings to verify that we have _not_ gotten past w.Add()
	// * then we unblock the responses channel by reading the thing off it
	// * then verify that happenings has gotten its second message and the goroutine has gotten past the block

	// block until we have proof the goroutine has run
	select {
	case <-happenings:
		// cool, the goroutine has started.
	case <-time.After(1 * time.Second):
		// bummer,  the goroutine never started.
		t.Error("timed out waiting for the blocking Add to start")
	}

	// verify we have _not_ gotten a second message on the happenings channel,
	// meaning that the Add didn't block
	select {
	case <-happenings:
		t.Error("w.Add() didn't block on the response channel")
	case <-time.After(1 * time.Second):
		// ok, it took a second to make sure, but we can continue now.
	}

	// unblock the response queue by reading the event off it
	select {
	case <-w.TxResponses():
		// good, this is expected
	case <-time.After(1 * time.Second):
		// ehh... there was supposed to be something there.
		t.Error("timed out waiting for the async w.Add to happen")
	}

	// now verify that we get through the second happenings to confirm that we're past the blocked Add
	select {
	case <-happenings:
		// yay
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting for the second happening. we must still be blocked on the response queue")
	}

}

func TestBuildReqReaderNoGzip(t *testing.T) {
	payload := []byte(`{"hello": "world"}`)

	// Ensure that if compress is false, we get expected values
	reader, compressed := buildReqReader([]byte(`{"hello": "world"}`), false)
	testEquals(t, compressed, false)
	readBuffer, err := ioutil.ReadAll(reader)
	testOK(t, err)
	testEquals(t, readBuffer, payload)

	// Ensure that if useGzip is true, we get compressed values
	reader, compressed = buildReqReader([]byte(`{"hello": "world"}`), true)
	testEquals(t, compressed, true)
	readBuffer, err = ioutil.ReadAll(reader)
	testOK(t, err)

	decompressed, err := zstd.Decompress(nil, readBuffer)
	testOK(t, err)
	testEquals(t, decompressed, payload)
}

func TestMsgpackArrayEncoding(t *testing.T) {
	t.Parallel()

	frt := &FakeRoundTripper{}
	b := &batchAgg{
		httpClient:            &http.Client{Transport: frt},
		testNower:             &fakeNower{},
		responses:             make(chan Response, 1),
		metrics:               &nullMetrics{},
		disableCompression:    true,
		enableMsgpackEncoding: true,
	}
	e := &Event{
		Data: map[string]interface{}{
			"a": 1,
		},
	}

	for _, evCount := range []int{5, 25000, 100000} {
		frt.resp = &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       ioutil.NopCloser(errReader{}),
		}
		b.batches = nil

		for i := 0; i < evCount; i++ {
			b.Add(e)
		}
		b.Fire(&testNotifier{})

		var v []interface{}
		err := msgpack.Unmarshal([]byte(frt.reqBody), &v)
		testOK(t, err)
		testEquals(t, len(v), evCount)
	}
}

func randomString(length int) string {
	b := make([]byte, length/2)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func withJSONAndMsgpack(t *testing.T, doMsgpack *bool, f func(t *testing.T)) {
	*doMsgpack = false
	t.Run("json", f)
	*doMsgpack = true
	t.Run("msgpack", f)
}

// Not entirely realistic; most real-world data has far more repetition and
// therefor achieves better compression.
func TestCompressionRatio(t *testing.T) {
	payloads := make([]map[string]interface{}, 1000)
	for i := range payloads {
		payloads[i] = fakePayload(50)
	}

	payload, err := json.Marshal(payloads)
	testOK(t, err)

	z, _ := buildReqReader(payload, true)
	zData, err := ioutil.ReadAll(z)
	testOK(t, err)

	g, _ := buildGzipReader(payload, true)
	gData, err := ioutil.ReadAll(g)
	testOK(t, err)

	t.Logf(
		"JSON uncompressed: %d, gzip: %0.2g, zstd: %0.2g",
		len(payload),
		float64(len(gData))/float64(len(payload)),
		float64(len(zData))/float64(len(payload)),
	)

	payload, err = msgpack.Marshal(payloads)
	testOK(t, err)

	z, _ = buildReqReader(payload, true)
	zData, err = ioutil.ReadAll(z)
	testOK(t, err)

	g, _ = buildGzipReader(payload, true)
	gData, err = ioutil.ReadAll(g)
	testOK(t, err)

	t.Logf(
		"Msgpack uncompressed: %d, gzip: %0.2g, zstd: %0.2g",
		len(payload),
		float64(len(gData))/float64(len(payload)),
		float64(len(zData))/float64(len(payload)),
	)
}

func BenchmarkCompression(b *testing.B) {
	payloads := make([]map[string]interface{}, 50)
	for i := range payloads {
		payloads[i] = fakePayload(100)
	}

	payload, err := json.Marshal(payloads)
	testOK(b, err)

	buf := make([]byte, len(payload))
	b.Run("raw", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			reader, _ := buildReqReader(payload, false)
			reader.Read(buf)
			reader.Close()
		}
	})

	b.Run("zstd", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			reader, _ := buildReqReader(payload, true)
			reader.Read(buf)
			reader.Close()
		}
	})

	b.Run("gzip", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			reader, _ := buildGzipReader(payload, true)
			reader.Read(buf)
		}
	})
}

// Legacy gzip compression code, for benchmark comparison purposes
func buildGzipReader(jsonEncoded []byte, compress bool) (io.Reader, bool) {
	if compress {
		buf := bytes.Buffer{}
		g := gzip.NewWriter(&buf)
		if _, err := g.Write(jsonEncoded); err == nil {
			if err = g.Close(); err == nil { // flush
				return &buf, true
			}
		}
	}
	return bytes.NewReader(jsonEncoded), false
}
