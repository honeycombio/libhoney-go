package transmission

import (
	"encoding/binary"
	"fmt"
	"time"
)

//go:generate msgp
//msgp:ignore MsgpTime
//msgp:marshal ignore MsgpackEvent EventSlice MsgpackResponseSlice MsgpackErrorResponse
//msgp:unmarshal ignore MsgpackEvent EventSlice MsgpackResponseSlice MsgpackErrorResponse

// MsgpackEvent is exported solely for use by the msgp library.
type MsgpackEvent struct {
	Timestamp  MsgpTime               `msg:"time,extension"`
	SampleRate uint                   `msg:"samplerate"`
	Data       map[string]interface{} `msg:"data"`
}

// EventSlice is exported solely for use by the msgp library.
// This will produce warnings from go generate, but allows us to serialize
// a []*Event slice directly.
type EventSlice []*Event

// MsgpackResponseSlice is exported solely for use by the msgp library.
type MsgpackResponseSlice []struct {
	ErrorStr string `msg:"error"`
	Status   int    `msg:"status"`
}

type MsgpackErrorResponse struct {
	ErrorStr string `msg:"error" json:"error"`
}

// MsgpTime is exported solely for use by the msgp library.
type MsgpTime struct {
	time.Time
}

func (t *MsgpTime) ExtensionType() int8 {
	return -1
}

func (t *MsgpTime) Len() int {
	return 12
}

func (t *MsgpTime) MarshalBinaryTo(b []byte) error {
	binary.BigEndian.PutUint32(b, uint32(t.Time.Nanosecond()))
	binary.BigEndian.PutUint64(b[4:], uint64(t.Time.Unix()))
	return nil
}

func (t *MsgpTime) UnmarshalBinary(b []byte) error {
	switch len(b) {
	case 4:
		sec := binary.BigEndian.Uint32(b)
		t.Time = time.Unix(int64(sec), 0).UTC()
	case 8:
		sec := binary.BigEndian.Uint64(b)
		nsec := int64(sec >> 34)
		sec &= 0x00000003ffffffff
		t.Time = time.Unix(int64(sec), nsec).UTC()
	case 12:
		nsec := binary.BigEndian.Uint32(b)
		sec := binary.BigEndian.Uint64(b[4:])
		t.Time = time.Unix(int64(sec), int64(nsec)).UTC()
	default:
		return fmt.Errorf("invalid length %d decoding time", len(b))
	}
	return nil
}
