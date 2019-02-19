package libhoney

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/stretchr/testify/assert"
)

func TestClientAdding(t *testing.T) {
	b := &Builder{
		dynFields: make([]dynamicField, 0, 0),
		fieldHolder: fieldHolder{
			data: make(map[string]interface{}),
		},
	}
	c := &Client{
		builder: b,
	}

	c.AddDynamicField("dynamo", func() interface{} { return nil })
	assert.Equal(t, 1, len(b.dynFields), "after adding dynamic field, builder should have it")

	c.AddField("pine", 34)
	assert.Equal(t, 34, b.data["pine"], "after adding field, builder should have it")

	c.Add(map[string]interface{}{"birch": 45})
	assert.Equal(t, 45, b.data["birch"], "after adding complex field, builder should have it")

	ev := c.NewEvent()
	assert.Equal(t, 34, ev.data["pine"], "with default content, created events should be prepopulated")

	b2 := c.NewBuilder()
	assert.Equal(t, 34, b2.data["pine"], "with default content, cloned builders should be prepopulated")
}

func TestNewClient(t *testing.T) {
	conf := ClientConfig{
		APIKey: "Oliver",
	}
	c, err := NewClient(conf)

	assert.NoError(t, err, "new client should not error")
	assert.Equal(t, "Oliver", c.builder.WriteKey, "initialized client should respect config")
}

func TestClientIsolated(t *testing.T) {
	c1, _ := NewClient(ClientConfig{})
	c2, _ := NewClient(ClientConfig{})

	AddField("Mary", 83)
	c1.AddField("Ursula", 88)
	c2.AddField("Philip", 53)

	ed := NewEvent()
	assert.Equal(t, 83, ed.data["Mary"], "global libhoney should have global content")
	assert.Equal(t, nil, ed.data["Ursula"], "global libhoney should not have client content")
	assert.Equal(t, nil, ed.data["Philip"], "global libhoney should not have client content")

	e1 := c1.NewEvent()
	assert.Equal(t, 88, e1.data["Ursula"], "client events should have client-scoped date")
	assert.Equal(t, nil, e1.data["Philip"], "client events should not have other client's content")
	assert.Equal(t, nil, e1.data["Mary"], "client events should not have global content")

	e2 := c2.NewEvent()
	assert.Equal(t, 53, e2.data["Philip"], "client events should have client-scoped data")
	assert.Equal(t, nil, e2.data["Ursula"], "client events should not have other client's content")
	assert.Equal(t, nil, e2.data["Mary"], "client events should not have global content")
}

func TestClientRaces(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			c, _ := NewClient(ClientConfig{})
			c.AddField("name", "val")
			ev := c.NewEvent()
			ev.AddField("argel", "bargle")
			ev.Send()
			c.Close()
			wg.Done()
		}()
	}
	wg.Wait()
}

// dirtySender is a transmisison Sender that reads and writes all the event's
// fields in an attempt to create a data race
type dirtySender struct{}

func (ds *dirtySender) Start() error                            { return nil }
func (ds *dirtySender) Stop() error                             { return nil }
func (ds *dirtySender) TxResponses() chan transmission.Response { return nil }
func (ds *dirtySender) SendResponse(transmission.Response) bool { return true }
func (ds *dirtySender) Add(ev *transmission.Event) {
	// fmt.Printf("transmission address of ev.Data is %v", ev.Data)
	oldAPIKey := ev.APIKey
	ev.APIKey = "some new value"
	oldDataset := ev.Dataset
	ev.Dataset = "some different value"
	oldSampleRate := ev.SampleRate
	ev.SampleRate = 10
	oldAPIHost := ev.APIHost
	ev.APIHost = "some wacky value"
	oldTimestamp := ev.Timestamp
	ev.Timestamp = time.Now()
	oldMetadata := ev.Metadata
	ev.Metadata = "some intertype value"
	ev.Data["new value"] = fmt.Sprintf("%s %s %d %s %s %+v", oldAPIKey,
		oldDataset, oldSampleRate, oldAPIHost, oldTimestamp.Format(time.RFC3339), oldMetadata)
	vals := make([]interface{}, 0)
	keys := make([]string, 0)
	for key, val := range ev.Data {
		time.Sleep(1)
		keys = append(keys, key)
		vals = append(vals, val)
	}
	for _, key := range keys {
		time.Sleep(1)
		ev.Data[key] = "overwrite all keys in the event"
	}
}

func TestAddSendRaces(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	tx := &dirtySender{}
	c, _ := NewClient(ClientConfig{
		Transmission: tx,
	})
	ev := c.NewEvent()
	ev.WriteKey = "oh my, a write"
	ev.Dataset = "there is no data in this set"
	ev.APIHost = "APIHostess with the mostess"
	ev.AddField("preexisting", "condition")
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(num int) {
			ev.AddField(fmt.Sprintf("argle%d", num), fmt.Sprintf("bargle%d", num))
			wg.Done()
		}(i)
	}
	go func() {
		// fmt.Printf("libh address of ev.data is %v", &ev.data)
		err := ev.Send()
		assert.NoError(t, err, "sending event shouldn't error")
		wg.Done()
	}()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(num int) {
			ev.AddField(fmt.Sprintf("foogle%d", num), fmt.Sprintf("boogle%d", num))
			wg.Done()
		}(i)
	}
	wg.Wait()
	// fmt.Printf("after, event data is %v", ev.data)
}

func TestEnsureLoggerRaces(t *testing.T) {
	c := Client{}

	// Close() ensures the Logger exists.
	go func() { c.Close() }()
	go func() { c.Close() }()
}

func TestEnsureTransmissionRaces(t *testing.T) {
	c := Client{}

	// TxResponses() ensures the Transmission exists.
	go func() { c.TxResponses() }()
	go func() { c.TxResponses() }()
}

func TestEnsureBuilderRaces(t *testing.T) {
	c := Client{}

	// AddField() ensures the Builder exists.
	go func() { c.AddField("ready, set", "GO") }()
	go func() { c.AddField("ready, set", "GO") }()
}
