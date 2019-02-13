package transmission

import (
	"sync"
)

// MockOutput implements the Output interface by retaining a slice of added
// events, for use in unit tests.
type MockOutput struct {
	Started          int
	Stopped          int
	EventsCalled     int
	events           []*Event
	responses        chan Response
	BlockOnResponses bool
	sync.Mutex
}

func (m *MockOutput) Add(ev *Event) {
	m.Lock()
	m.events = append(m.events, ev)
	m.Unlock()
}

func (m *MockOutput) Start() error {
	m.Started += 1
	m.responses = make(chan Response, 1)
	return nil
}
func (m *MockOutput) Stop() error {
	m.Stopped += 1
	return nil
}

func (m *MockOutput) Events() []*Event {
	m.EventsCalled += 1
	m.Lock()
	defer m.Unlock()
	output := make([]*Event, len(m.events))
	copy(output, m.events)
	return output
}

func (m *MockOutput) Responses() chan Response {
	return m.responses
}

func (m *MockOutput) SendResponse(r Response) bool {
	if m.BlockOnResponses {
		m.responses <- r
	} else {
		select {
		case m.responses <- r:
		default:
			return true
		}
	}
	return false
}
