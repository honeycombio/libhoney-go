package libhoney

import (
	"fmt"
	"time"
)

// Logger is used to log extra info within the SDK detailing what's happening.
// You can set a logger during initialization. If you leave it unititialized, no
// logging will happen. If you set it to the DefaultLogger, you'll get
// timestamped lines sent to STDOUT. Pass in your own implementation of the
// interface to send it in to your own logger.
type Logger interface {
	// Log accepts the same msg, args style as fmt.Printf().
	Log(msg string, args ...interface{})
}

// DefaultLogger implements Logger and prints messages to stdout prepended by a
// timestamp (RFC3339 formatted)
type DefaultLogger struct{}

// Log prints the message to stdout.
func (d *DefaultLogger) Log(msg string, args ...interface{}) {
	// use the same format as the python libhoney:
	// '%(asctime)s - %(name)s - %(levelname)s - %(message)s')
	// except for go's more friendly rfc3339nano rather than asctime
	t := time.Now().Format(time.RFC3339Nano)
	msg = fmt.Sprintf("%s - %s - %s - %s", t, "libhoney", "DEBUG", msg)
	fmt.Printf(msg+"\n", args...)
}

type nullLogger struct{}

// Log swallows messages
func (n *nullLogger) Log(msg string, args ...interface{}) {
	// nothing to see here.
}
