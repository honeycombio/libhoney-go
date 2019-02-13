package transmission

type Logger interface {
	// Printf accepts the same msg, args style as fmt.Printf().
	Printf(msg string, args ...interface{})
}

