package libhoney

// txClient handles the transmission of events to Honeycomb.
//
// Overview
//
// Create a new instance of Client.
// Set any of the public fields for which you want to override the defaults.
// Call Start() to spin up the background goroutines necessary for transmission
// Call Add(Event) to queue an event for transmission
// Ensure Stop() is called to flush all in-flight messages.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/facebookgo/muster"
)

type txClient interface {
	Start() error
	Stop() error
	Add(*Event)
}

type txDefaultClient struct {
	maxBatchSize         uint          // how many events to collect into a batch before sending
	batchTimeout         time.Duration // how often to send off batches
	maxConcurrentBatches uint          // how many batches can be inflight simultaneously
	pendingWorkCapacity  uint          // how many events to allow to pile up
	blockOnSend          bool          // whether to block or drop events when the queue fills
	blockOnResponses     bool          // whether to block or drop responses when the queue fills

	transport http.RoundTripper

	muster muster.Client
}

func (t *txDefaultClient) Start() error {
	t.muster.MaxBatchSize = t.maxBatchSize
	t.muster.BatchTimeout = t.batchTimeout
	t.muster.MaxConcurrentBatches = t.maxConcurrentBatches
	t.muster.PendingWorkCapacity = t.pendingWorkCapacity
	t.muster.BatchMaker = func() muster.Batch {
		return &batch{
			events:           map[string][]*Event{},
			httpClient:       &http.Client{Transport: t.transport},
			blockOnResponses: t.blockOnResponses,
		}
	}
	return t.muster.Start()
}

func (t *txDefaultClient) Stop() error {
	return t.muster.Stop()
}

func (t *txDefaultClient) Add(ev *Event) {
	// don't block if we can't send events fast enough
	sd.Gauge("queue_length", len(t.muster.Work))
	if t.blockOnSend {
		t.muster.Work <- ev
		sd.Increment("messages_queued")
	} else {
		select {
		case t.muster.Work <- ev:
			sd.Increment("messages_queued")
		default:
			sd.Increment("queue_overflow")
			r := Response{
				Err:      errors.New("queue overflow"),
				Metadata: ev.Metadata,
			}
			if t.blockOnResponses {
				responses <- r
			} else {
				select {
				case responses <- r:
				default:
				}
			}
		}
	}
}

type txTestClient struct {
	Timestamps  []time.Time
	datas       [][]byte
	sampleRates []uint
}

func (t *txTestClient) Start() error {
	t.Timestamps = make([]time.Time, 0)
	t.datas = make([][]byte, 0)
	return nil
}

func (t *txTestClient) Stop() error {
	return nil
}

func (t *txTestClient) Add(ev *Event) {
	t.Timestamps = append(t.Timestamps, ev.Timestamp)
	blob, err := json.Marshal(ev.data)
	if err != nil {
		panic(err)
	}
	t.datas = append(t.datas, blob)
	t.sampleRates = append(t.sampleRates, ev.SampleRate)
}

type batch struct {
	events           map[string][]*Event
	httpClient       *http.Client
	blockOnResponses bool

	// allows manipulation of the value of "now" for testing
	testNower   nower
	testBlocker *sync.WaitGroup
}

func (b *batch) Add(ev interface{}) {
	// from muster godoc: "The Batch does not need to be safe for concurrent
	// access; synchronization will be handled by the Client."
	if b.events == nil {
		b.events = map[string][]*Event{}
	}
	e := ev.(*Event)
	if _, ok := b.events[e.Dataset]; !ok {
		b.events[e.Dataset] = make([]*Event, 0, 1)
	}
	b.events[e.Dataset] = append(b.events[e.Dataset], e)
}

// Provides finer-tuned control over handling serialization errors (by
// returning individual errors down the responses channel.
func (b *batch) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteByte('{')

	keys := make([]string, 0, len(b.events))
	for datasetName := range b.events {
		keys = append(keys, datasetName)
	}
	sort.Strings(keys) // parity with encoding/json's mapEncoder

	var mapCt int
	var arrCt int
	var totalEncoded int
	for _, datasetName := range keys {
		kBytes, err := json.Marshal(datasetName) // lean on encoding/json's stringEncoder
		if err != nil {
			// return err for all of the dataset's events
			for _, ev := range b.events[datasetName] {
				b.enqueueResponse(Response{
					Err:      err,
					Metadata: ev.Metadata,
				})
			}
			continue
		}
		if mapCt > 0 {
			buf.WriteByte(',')
		}
		mapCt++
		buf.Write(kBytes)
		buf.WriteByte(':')
		buf.WriteByte('[')

		arrCt = 0
		for _, ev := range b.events[datasetName] {
			eBytes, err := json.Marshal(ev)
			if err != nil {
				b.enqueueResponse(Response{
					Err:      err,
					Metadata: ev.Metadata,
				})
				continue
			}
			if arrCt > 0 {
				buf.WriteByte(',')
			}
			arrCt++
			totalEncoded++
			buf.Write(eBytes)
		}
		buf.WriteByte(']')
	}
	if totalEncoded == 0 {
		// If the datasetName was written but no Events were, make sure to return an
		// explicitly empty object
		return []byte("{}"), nil
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (b *batch) enqueueResponse(resp Response) {
	if b.blockOnResponses {
		responses <- resp
	} else {
		select {
		case responses <- resp:
		default: // drop on the floor (and maybe notify tests)
			if b.testBlocker != nil {
				b.testBlocker.Done()
			}
		}
	}
}

func (b *batch) Fire(notifier muster.Notifier) {
	defer notifier.Done()

	start := time.Now().UTC()
	if b.testNower != nil {
		start = b.testNower.Now()
	}

	blob, _ := json.Marshal(b) // always returns nil err

	// If json.Marshal returns the shortest valid JSON object, we don't have
	// anything worth sending to Honeycomb (we may have failed to enqueue any
	// events, for any dataset). Don't bother enqueueing a Response;
	// batch.MarshalJSON likely already did that for us.
	if len(blob) <= 2 {
		return
	}

	// sigh. dislike
	userAgent := fmt.Sprintf("libhoney-go/%s", version)
	if UserAgentAddition != "" {
		userAgent = fmt.Sprintf("%s %s", userAgent, strings.TrimSpace(UserAgentAddition))
	}

	// FML. technically we have nothing that constrains events to a single APIHost
	// or a single WriteKey per batch. ugggg ben suggestions?
	var apiHost string
	var writeKey string
	for _, events := range b.events {
		if len(events) < 1 {
			continue
		}
		apiHost = events[0].APIHost
		writeKey = events[0].WriteKey
		break
	}

	reqBody, gzipped := buildReqReader(blob)
	url := fmt.Sprintf("%s/1/batch", apiHost)
	req, err := http.NewRequest("POST", url, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if gzipped {
		req.Header.Set("Content-Encoding", "gzip")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Add("X-Honeycomb-Team", writeKey)

	// send off batch!
	resp, err := b.httpClient.Do(req)
	end := time.Now().UTC()
	if b.testNower != nil {
		end = b.testNower.Now()
	}
	dur := end.Sub(start)

	batchSize := 0
	for _, events := range b.events {
		batchSize += len(events) // ... missing out on events dropped during json encoding
	}

	if err != nil {
		sd.Increment("send_errors")
		// if there's a top-level error, we should send an error response down for
		// every event?? do we actually have examples that read responses and retry?
		b.enqueueResponse(Response{
			Duration: dur,
			Metadata: fmt.Sprintf("batch containing %d events for %d datasets", batchSize, len(b.events)),
			Err:      err,
		})
		return
	}

	sd.Increment("batches_sent")
	sd.Count("messages_sent", batchSize)
	defer resp.Body.Close()

	batchResponse := map[string][]Response{}
	err = json.NewDecoder(resp.Body).Decode(&batchResponse)
	if err != nil {
		sd.Increment("response_decode_errors")
		b.enqueueResponse(Response{
			Duration: dur,
			Metadata: fmt.Sprintf("batch containing %d events for %d datasets", batchSize, len(b.events)),
			Err:      err,
		})
		return
	}

	for datasetName, resps := range batchResponse {
		if events, ok := b.events[datasetName]; ok { // just in case
			for i, resp := range resps {
				resp.Duration = dur / time.Duration(batchSize)
				if i < len(events) { // just in case
					resp.Metadata = events[i].Metadata
				}
				b.enqueueResponse(resp)
			}
		}
	}
}

// buildReqReader returns an io.Reader and a boolean, indicating whether or not
// the io.Reader is gzip-compressed.
func buildReqReader(jsonEncoded []byte) (io.Reader, bool) {
	buf := bytes.Buffer{}
	g := gzip.NewWriter(&buf)
	if _, err := g.Write(jsonEncoded); err == nil {
		if err = g.Close(); err == nil { // flush
			return &buf, true
		}
	}
	return bytes.NewReader(jsonEncoded), false
}

// nower to make testing easier
type nower interface {
	Now() time.Time
}
