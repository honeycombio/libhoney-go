package libhoney

import (
	"errors"
)

// TODO think about whether it would be useful for Client to be an interface instead

type Client interface {
	Add(data interface{}) error
	AddDynamicField(name string, fn func() interface{}) error
	AddField(name string, val interface{})
	Close()
	Flush()
	NewBuilder() *Builder
	NewEvent() *Event
	Responses() chan Response
}

type defaultClient struct {
	conf              Config
	tx                Output
	logger            Logger
	responses         chan Response
	defaultBuilder    *Builder
	userAgentAddition string
}

func NewClient(conf Config) (Client, error) {
	setConfigDefaults(&conf)

	c := &defaultClient{
		conf:   conf,
		logger: conf.Logger,
	}

	if conf.Output == nil {
		// use default Honeycomb output
		c.tx = &txDefaultClient{
			maxBatchSize:         conf.MaxBatchSize,
			batchTimeout:         conf.SendFrequency,
			maxConcurrentBatches: conf.MaxConcurrentBatches,
			pendingWorkCapacity:  conf.PendingWorkCapacity,
			blockOnSend:          conf.BlockOnSend,
			blockOnResponses:     conf.BlockOnResponse,
			transport:            conf.Transport,
			logger:               conf.Logger,
		}
	} else {
		c.tx = conf.Output
	}
	if err := c.tx.Start(); err != nil {
		c.logger.Printf("transmission client failed to start: %s", err.Error())
		return nil, err
	}

	c.responses = make(chan Response, conf.PendingWorkCapacity*2)

	c.defaultBuilder = &Builder{
		WriteKey:   conf.WriteKey,
		Dataset:    conf.Dataset,
		SampleRate: conf.SampleRate,
		APIHost:    conf.APIHost,
		dynFields:  make([]dynamicField, 0, 0),
		fieldHolder: fieldHolder{
			data: make(map[string]interface{}),
		},
		client: c,
	}

	return c, nil
}

// Close waits for all in-flight messages to be sent. You should
// call Close() before app termination.
func (c *defaultClient) Close() {
	c.logger.Printf("closing libhoney client")
	if c.tx != nil {
		c.tx.Stop()
	}
	close(c.responses)
}

// Flush closes and reopens the Output interface, ensuring events
// are sent without waiting on the batch to be sent asyncronously.
// Generally, it is more efficient to rely on asyncronous batches than to
// call Flush, but certain scenarios may require Flush if asynchronous sends
// are not guaranteed to run (i.e. running in AWS Lambda)
// Flush is not thread safe - use it only when you are sure that no other
// parts of your program are calling Send
func (c *defaultClient) Flush() {
	c.logger.Printf("flushing libhoney client")
	if c.tx != nil {
		c.tx.Stop()
		c.tx.Start()
	}
}

// Responses returns the channel from which the caller can read the responses
// to sent events.
func (c *defaultClient) Responses() chan Response {
	return c.responses
}

// AddDynamicField takes a field name and a function that will generate values
// for that metric. The function is called once every time a NewEvent() is
// created and added as a field (with name as the key) to the newly created
// event.
func (c *defaultClient) AddDynamicField(name string, fn func() interface{}) error {
	return c.defaultBuilder.AddDynamicField(name, fn)
}

// AddField adds a Field to the global scope. This metric will be inherited by
// all builders and events.
func (c *defaultClient) AddField(name string, val interface{}) {
	c.defaultBuilder.AddField(name, val)
}

// Add adds its data to the global scope. It adds all fields in a struct or all
// keys in a map as individual Fields. These metrics will be inherited by all
// builders and events.
func (c *defaultClient) Add(data interface{}) error {
	return c.defaultBuilder.Add(data)
}

// NewEvent creates a new event prepopulated with any Fields present in the
// global scope.
func (c *defaultClient) NewEvent() *Event {
	return c.defaultBuilder.NewEvent()
}

// NewBuilder creates a new event builder. The builder inherits any
// Dynamic or Static Fields present in the global scope.
func (c *defaultClient) NewBuilder() *Builder {
	return c.defaultBuilder.Clone()
}

// sendResponse sends a dropped event response down the response channel
func (c *defaultClient) sendDroppedResponse(e *Event, message string) {
	r := Response{
		Err:      errors.New(message),
		Metadata: e.Metadata,
	}
	c.logger.Printf("got response code %d, error %s, and body %s",
		r.StatusCode, r.Err, string(r.Body))
	writeToResponse(c.responses, r, c.conf.BlockOnResponse)
}
