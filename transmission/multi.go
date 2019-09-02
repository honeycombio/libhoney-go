package transmission

import (
	"errors"

	"github.com/honeycombio/libhoney-go"
)

type MultiSender struct {
	Senders []Sender
}

func NewStandardPlusStdout() *MultiSender {
	return &MultiSender{
		Senders: []Sender{
			&Honeycomb{
				MaxBatchSize:         libhoney.DefaultMaxBatchSize,
				BatchTimeout:         libhoney.DefaultBatchTimeout,
				MaxConcurrentBatches: libhoney.DefaultMaxConcurrentBatches,
				PendingWorkCapacity:  libhoney.DefaultPendingWorkCapacity,
				UserAgentAddition:    libhoney.UserAgentAddition,
				Logger:               &libhoney.DefaultLogger{},
			},
			&WriterSender{},
		},
	}
}

// Add calls Add on every configured Sender
func (s *MultiSender) Add(ev *Event) {
	for _, tx := range s.Senders {
		tx.Add(ev)
	}
}

// Start calls Start on every configured Sender, aborting on the first error
func (s *MultiSender) Start() error {
	if len(s.Senders) == 0 {
		return errors.New("no senders configured")
	}
	for _, tx := range s.Senders {
		if err := tx.Start(); err != nil {
			return err
		}
	}
	return nil
}

// Stop calls Stop on every configured Sender, aborting on the first error
func (s *MultiSender) Stop() error {
	for _, tx := range s.Senders {
		if err := tx.Stop(); err != nil {
			return err
		}
	}
	return nil
}

// Responses returns the response channel from the first Sender only
func (s *MultiSender) TxResponses() chan Response {
	return s.Senders[0].TxResponses()
}

// SendResponse calls SendResponse on every configured Sender
func (s *MultiSender) SendResponse(resp Response) bool {
	for _, tx := range s.Senders {
		if ok := tx.SendResponse(resp); !ok {
			return false
		}
	}
	return true
}
