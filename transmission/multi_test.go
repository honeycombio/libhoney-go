package transmission

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMultiSenderStart(t *testing.T) {
	// Errors with no transmissions configured
	s := &MultiSender{}
	err := s.Start()
	assert.NotNil(t, err, "starting an empty multi should be an error")

	// Convenience startup is valid
	s = NewStandardPlusStdout()
	err = s.Start()
	assert.Nil(t, err, "the plus stdout sender should start without errors")
}

func TestMultiSender(t *testing.T) {
	a := &MockSender{}
	b := &MockSender{}
	sender := MultiSender{
		Senders: []Sender{a, b},
	}

	t.Run("Start", func(t *testing.T) {
		err := sender.Start()
		assert.Nil(t, err)
		assert.Equal(t, 1, a.Started)
		assert.Equal(t, 1, b.Started)
	})

	t.Run("Stop", func(t *testing.T) {
		err := sender.Stop()
		assert.Nil(t, err)
		assert.Equal(t, 1, a.Stopped)
		assert.Equal(t, 1, b.Stopped)
	})

	t.Run("Add", func(t *testing.T) {
		ev := Event{
			Timestamp:  time.Time{}.Add(time.Second),
			SampleRate: 2,
			Dataset:    "dataset",
			Data:       map[string]interface{}{"key": "val"},
		}

		sender.Add(&ev)

		assert.Len(t, a.Events(), 1)
		assert.Equal(t, ev, *a.Events()[0])
		assert.Len(t, b.Events(), 1)
		assert.Equal(t, ev, *b.Events()[0])
	})

	t.Run("TxResponses takes the first one", func(t *testing.T) {
		assert.Equal(t, a.TxResponses(), sender.TxResponses())
		assert.NotEqual(t, b.TxResponses(), sender.TxResponses())
	})

	t.Run("SendResponse", func(t *testing.T) {
		sender.SendResponse(Response{})
	})
}
