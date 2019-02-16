package libhoney

import (
	"testing"

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
