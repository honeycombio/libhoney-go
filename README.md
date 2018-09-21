# libhoney [![Build Status](https://travis-ci.org/honeycombio/libhoney-go.svg?branch=master)](https://travis-ci.org/honeycombio/libhoney-go)

Go library for sending events to [Honeycomb](https://honeycomb.io). (For more information, see the [documentation](https://honeycomb.io/docs/) and [Go SDK guide](https://honeycomb.io/docs/connect/go).)

## Installation:

```
go get -v github.com/honeycombio/libhoney-go
```

## Documentation

A godoc API reference is available at https://godoc.org/github.com/honeycombio/libhoney-go

## Example

Honeycomb can calculate all sorts of statistics, so send the values you care about and let us crunch the averages, percentiles, lower/upper bounds, cardinality -- whatever you want -- for you.

```go
import "github.com/honeycombio/libhoney-go"

func main() {
  // Call Init to configure libhoney
  libhoney.Init(libhoney.Config{
    WriteKey: "YOUR_WRITE_KEY",
    Dataset: "honeycomb-golang-example",
  })
  // Flush any pending calls to Honeycomb before exiting
  defer libhoney.Close()

  // Create an event, add some data
  ev := libhoney.NewEvent()
  ev.Add(map[string]interface{}{
    "method": "get",
    "hostname": "appserver15",
    "payload_length": 27,
  }))

  // This event will be sent regardless of how we exit
  defer ev.Send()

  if err := myOtherFunc(); err != nil {
    ev.AddField("error", err)
    ev.AddField("success", false)
    return
  }

  // do some work, maybe measure some things

  ev.AddField("duration_ms", 153.12)
  ev.AddField("success", true)
}
```

See the [`examples` directory](examples/read_json_log.go) for more sample code demonstrating
how to use events, builders, fields, and dynamic fields.

## Contributions

Features, bug fixes and other changes to libhoney are gladly accepted. Please
open issues or a pull request with your change. Remember to add your name to the
CONTRIBUTORS file!

All contributions will be released under the Apache License 2.0.

