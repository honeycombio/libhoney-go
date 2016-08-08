package libhoney

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

func TestTxAdd(t *testing.T) {
	dc := &txDefaultClient{}
	conf := &Config{}
	dc.muster.Work = make(chan interface{}, 1)
	responses = make(chan Response, 1)
	e := &Event{Metadata: "mmeetta"}
	dc.Add(e)
	added := <-dc.muster.Work
	testEquals(t, e, added)
	testResponsesChannelEmpty(t, responses)
	// make the queue 0 length to force an overflow
	dc.muster.Work = make(chan interface{}, 0)
	dc.Add(e)
	t.Log("1")
	testChannelHasResponse(t, responses)
	t.Log("2")
	// test blocking on send still gets it down the channel
	conf.BlockOnSend = true
	dc.muster.Work = make(chan interface{}, 1)
	dc.Add(e)
	added = <-dc.muster.Work
	t.Log("3")
	testEquals(t, e, added)
	testResponsesChannelEmpty(t, responses)
	t.Log("4")
	// test blocking on response still gets an overflow down the channel
	conf.BlockOnSend = false
	conf.BlockOnResponse = true
	dc.muster.Work = make(chan interface{}, 0)
	dc.Add(e)
	t.Log("5")
	testChannelHasResponse(t, responses)
	t.Log("6")
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
	conf := &Config{}
	responses = make(chan Response, 1)
	fn := &fakeNower{}
	fn.init()
	b := &batch{
		httpClient: &http.Client{},
		n:          fn,
	}

	frt := &FakeRoundTripper{}
	frt.resp = &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(&FakeBody{}),
	}
	frt.respErr = nil
	b.httpClient.Transport = frt
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

	rsp := <-responses
	testEquals(t, rsp.Duration, time.Second*10)
	testEquals(t, rsp.Metadata, "emmetta")
	testEquals(t, rsp.StatusCode, 200)
	testOK(t, rsp.Err)

	// test UserAgentAddition
	fn.init()
	UserAgentAddition = "  fancyApp/3 "
	expectedUserAgentAddition := "fancyApp/3"
	longUserAgent := fmt.Sprintf("%s %s", versionedUserAgent, expectedUserAgentAddition)
	b.sendRequest(e)
	testEquals(t, frt.req.Header.Get("User-Agent"), longUserAgent)
	rsp = <-responses

	// test unsuccessful send
	fn.init()
	frt.resp = nil
	frt.respErr = errors.New("testing error handling")
	b.sendRequest(e)
	rsp = <-responses
	testErr(t, rsp.Err)
	testEquals(t, rsp.StatusCode, 0)
	testEquals(t, len(rsp.Body), 0)

	// test blocking response path, error
	conf.BlockOnResponse = true
	fn.init()
	frt.resp = nil
	frt.respErr = errors.New("testing error handling")
	b.sendRequest(e)
	rsp = <-responses
	testErr(t, rsp.Err)
	testEquals(t, rsp.StatusCode, 0)
	testEquals(t, len(rsp.Body), 0)

	// test blocking response path, no error
	conf.BlockOnResponse = true
	fn.init()
	frt.resp = &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(&FakeBody{}),
	}
	frt.respErr = nil
	b.sendRequest(e)
	rsp = <-responses
	testEquals(t, rsp.Duration, time.Second*10)
	testEquals(t, rsp.Metadata, "emmetta")
	testEquals(t, rsp.StatusCode, 200)
	testOK(t, rsp.Err)

}
