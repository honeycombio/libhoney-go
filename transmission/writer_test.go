package transmission

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWriterOutputStart(t *testing.T) {
	// Starting a writer does nothing but creates the responses channel.
	w := &WriterOutput{}
	assert.Nil(t, w.responses, "before starting, responses should be nil")
	w.Start()
	assert.NotNil(t, w.responses, "after starting, responses should be a real channel")
}

type mockIO struct {
	written []byte
}

func (m *mockIO) Write(p []byte) (int, error) {
	m.written = p
	return 0, nil
}

func TestWriterOutput(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	writer := WriterOutput{
		W: buf,
	}
	ev := Event{
		Timestamp:  time.Time{}.Add(time.Second),
		SampleRate: 2,
		Dataset:    "dataset",
		Data:       map[string]interface{}{"key": "val"},
	}

	writer.Add(&ev)
	testEquals(
		t,
		strings.TrimSpace(buf.String()),
		`{"data":{"key":"val"},"samplerate":2,"time":"0001-01-01T00:00:01Z","dataset":"dataset"}`,
	)
}

func TestWriterOutputAdd(t *testing.T) {
	// Adding an event to a WriterOutput should write the event in json form to
	// the writer that is configured on the output. It should also generate one
	// Response per event Added.
	mocked := &mockIO{}
	w := &WriterOutput{
		W:                 mocked,
		ResponseQueueSize: 1,
	}
	w.Start()

	ev := &Event{
		Metadata: "adder",
		Data:     map[string]interface{}{"foo": "bar"},
	}
	w.Add(ev)
	assert.Equal(t, "{\"data\":{\"foo\":\"bar\"}}\n", string(mocked.written), "emty event should still make JSON")

	select {
	case res := <-w.TxResponses():
		assert.Equal(t, ev.Metadata, res.Metadata, "Response off the queue should have the right metadata")
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting on channel")
	}
}

func TestWriterOutputAddingResponsesNonblocking(t *testing.T) {
	// using the public SendRespanse method should add the response to the queue
	// while honoring the block setting
	w := &WriterOutput{
		W:                 ioutil.Discard,
		BlockOnResponses:  false,
		ResponseQueueSize: 1,
	}
	w.Start()
	ev := &Event{
		Metadata: "adder",
	}

	// one ev should get one response
	w.Add(ev)
	select {
	case res := <-w.TxResponses():
		assert.Equal(t, ev.Metadata, res.Metadata, "Response off the queue should have the right metadata")
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting on channel")
	}

	// three evs should get one response (because the response queue size is
	// one; the second and third should be dropped)
	w.Add(ev)
	w.Add(ev)
	w.Add(ev)

	select {
	case res := <-w.TxResponses():
		assert.Equal(t, ev.Metadata, res.Metadata, "Response off the queue should have the right metadata")
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting on channel")
	}

	select {
	case res := <-w.TxResponses():
		t.Errorf("shouldn't have gotten a second response, but got %+v", res)
	default:
		// good, we shouldn't have had a second one.
	}

}
func TestWriterOutputAddingResponsesBlocking(t *testing.T) {
	// this test has a few timeout checks. don't wait to run other tests.
	t.Parallel()
	// using the public SendRespanse method should add the response to the queue
	// while honoring the block setting
	w := &WriterOutput{
		W:                 ioutil.Discard,
		BlockOnResponses:  true,
		ResponseQueueSize: 1,
	}
	w.Start()
	ev := &Event{
		Metadata: "adder",
	}

	happenings := make(chan interface{}, 2)

	// push one thing in the queue. This should fill it up; the next call should block.
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
