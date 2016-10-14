package libhoney

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
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

type FakeBody struct{}

func (f *FakeBody) Read(p []byte) (int, error) {
	l := copy(p, []byte("foo"))
	return l, io.EOF
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

func TestTxSendRequest(t *testing.T) {
	responses = make(chan Response, 1)
	frt := &FakeRoundTripper{}
	frt.resp = &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(&FakeBody{}),
	}

	b := &batch{
		httpClient:  &http.Client{Transport: frt},
		testNower:   &fakeNower{},
		testBlocker: &sync.WaitGroup{},
	}

	fhData := map[string]interface{}{
		"foo": "bar",
	}
	e := &Event{
		fieldHolder: fieldHolder{data: fhData},
		SampleRate:  4,
		APIHost:     "fakeHost",
		WriteKey:    "written",
		Dataset:     "settled",
		Metadata:    "emmetta",
	}
	b.sendRequest(e)
	expectedURL := fmt.Sprintf("%s/1/events/%s", e.APIHost, e.Dataset)
	testEquals(t, frt.req.URL.String(), expectedURL)
	versionedUserAgent := fmt.Sprintf("libhoney-go/%s", version)
	testEquals(t, frt.req.Header.Get("User-Agent"), versionedUserAgent)
	testEquals(t, frt.req.Header.Get("X-Honeycomb-Team"), e.WriteKey)
	testEquals(t, frt.req.Header.Get("X-Honeycomb-SampleRate"), "4")
	testEquals(t, frt.reqBody, `{"foo":"bar"}`)

	rsp := testGetResponse(t, responses)
	testEquals(t, rsp.Duration, time.Second*10)
	testEquals(t, rsp.Metadata, "emmetta")
	testEquals(t, rsp.StatusCode, 200)
	testOK(t, rsp.Err)

	// test UserAgentAddition
	UserAgentAddition = "  fancyApp/3 "
	expectedUserAgentAddition := "fancyApp/3"
	longUserAgent := fmt.Sprintf("%s %s", versionedUserAgent, expectedUserAgentAddition)
	b.sendRequest(e)
	testEquals(t, frt.req.Header.Get("User-Agent"), longUserAgent)
	rsp = testGetResponse(t, responses)
	testEquals(t, rsp.StatusCode, 200)
	testOK(t, rsp.Err)

	// test unsuccessful send
	frt.resp = nil
	frt.respErr = errors.New("testing error handling")
	b.sendRequest(e)
	rsp = testGetResponse(t, responses)
	testErr(t, rsp.Err)
	testEquals(t, rsp.StatusCode, 0)
	testEquals(t, len(rsp.Body), 0)

	// test nonblocking response path is actually nonblocking, drops response
	responses <- placeholder
	b.testBlocker.Add(1)
	go b.sendRequest(e)
	b.testBlocker.Wait() // triggered on drop
	rsp = testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp,
		"should pull placeholder response and only placeholder response off channel")

	// test blocking response path, error
	b.blockOnResponses = true
	frt.resp = nil
	frt.respErr = errors.New("testing error handling")
	responses <- placeholder
	go b.sendRequest(e)
	rsp = testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp,
		"should pull placeholder response off channel first")
	rsp = testGetResponse(t, responses)
	testErr(t, rsp.Err)
	testEquals(t, rsp.StatusCode, 0)
	testEquals(t, len(rsp.Body), 0)

	// test blocking response path, no error
	frt.resp = &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(&FakeBody{}),
	}
	frt.respErr = nil
	responses <- placeholder
	go b.sendRequest(e)
	rsp = testGetResponse(t, responses)
	testIsPlaceholderResponse(t, rsp,
		"should pull placeholder response off channel first")
	rsp = testGetResponse(t, responses)
	testEquals(t, rsp.Duration, time.Second*10)
	testEquals(t, rsp.Metadata, "emmetta")
	testEquals(t, rsp.StatusCode, 200)
	testOK(t, rsp.Err)
}
