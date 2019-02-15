package transmission

// DiscardOutput implements the Output interface and drops all events.
type DiscardOutput struct {
	WriterOutput
}

func (d *DiscardOutput) Add(ev *Event) {}
