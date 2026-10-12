package transmission

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"
)

// appendJSON appends the event's wire JSON to b. It decodes the same as
// encoding/json's output, but handles the common field types directly: going
// through json.Marshal costs reflection per field and re-validates the output
// of every MarshalJSON method it calls, which dominates batch encoding time.
func (e *Event) appendJSON(b []byte) ([]byte, error) {
	b = append(b, `{"data":`...)
	b = appendData(b, e.Data)
	// don't include sample rate if it's 1; this is the default
	if e.SampleRate > 1 {
		b = append(b, `,"samplerate":`...)
		b = strconv.AppendUint(b, uint64(e.SampleRate), 10)
	}
	if !e.Timestamp.IsZero() {
		t, err := e.Timestamp.MarshalJSON()
		if err != nil {
			return nil, err
		}
		b = append(b, `,"time":`...)
		b = append(b, t...)
	}
	return append(b, '}'), nil
}

// appendData writes fields in sorted key order, omitting any that are nil or
// can't be marshalled rather than failing the whole event.
func appendData(b []byte, data map[string]any) []byte {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	b = append(b, '{')
	first := true
	for _, k := range keys {
		start := len(b)
		if !first {
			b = append(b, ',')
		}
		b = appendString(b, k)
		b = append(b, ':')
		var ok bool
		if b, ok = appendField(b, data[k]); ok {
			first = false
		} else {
			b = b[:start]
		}
	}
	return append(b, '}')
}

var ptrKinds = []reflect.Kind{reflect.Ptr, reflect.Slice, reflect.Map}

func appendField(b []byte, v any) ([]byte, bool) {
	if v == nil {
		return b, false
	}
	if b, ok := appendFast(b, v); ok {
		return b, true
	}
	val := reflect.ValueOf(v)
	if slices.Contains(ptrKinds, val.Kind()) && val.IsNil() {
		return b, false
	}
	return appendMarshalled(b, v)
}

func appendMarshalled(b []byte, v any) ([]byte, bool) {
	enc, err := json.Marshal(v)
	if err != nil {
		return b, false
	}
	return append(b, enc...), true
}

// appendFast encodes the exact types that make up most event fields. Named
// types are left to encoding/json, as they may implement json.Marshaler.
func appendFast(b []byte, v any) ([]byte, bool) {
	switch x := v.(type) {
	case string:
		return appendString(b, x), true
	case bool:
		return strconv.AppendBool(b, x), true
	case int:
		return strconv.AppendInt(b, int64(x), 10), true
	case int8:
		return strconv.AppendInt(b, int64(x), 10), true
	case int16:
		return strconv.AppendInt(b, int64(x), 10), true
	case int32:
		return strconv.AppendInt(b, int64(x), 10), true
	case int64:
		return strconv.AppendInt(b, x, 10), true
	case uint:
		return strconv.AppendUint(b, uint64(x), 10), true
	case uint8:
		return strconv.AppendUint(b, uint64(x), 10), true
	case uint16:
		return strconv.AppendUint(b, uint64(x), 10), true
	case uint32:
		return strconv.AppendUint(b, uint64(x), 10), true
	case uint64:
		return strconv.AppendUint(b, x, 10), true
	case time.Duration:
		return strconv.AppendInt(b, int64(x), 10), true
	case []string:
		if x == nil {
			return b, false
		}
		b = append(b, '[')
		for i, s := range x {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendString(b, s)
		}
		return append(b, ']'), true
	case []any:
		if x == nil {
			return b, false
		}
		return appendSlice(b, x)
	}
	return b, false
}

func appendSlice(b []byte, s []any) ([]byte, bool) {
	start := len(b)
	b = append(b, '[')
	for i, e := range s {
		if i > 0 {
			b = append(b, ',')
		}
		var ok bool
		if e == nil {
			b, ok = append(b, "null"...), true
		} else if b, ok = appendFast(b, e); !ok {
			b, ok = appendMarshalled(b, e)
		}
		if !ok {
			// encoding/json fails the whole slice if any element fails
			return b[:start], false
		}
	}
	return append(b, ']'), true
}

// appendString skips encoding/json's HTML escaping, as events aren't embedded
// in HTML, and leaves control characters and invalid UTF-8 to encoding/json.
func appendString(b []byte, s string) []byte {
	if !utf8.ValidString(s) {
		return appendQuoted(b, s)
	}
	start := len(b)
	b = append(b, '"')
	last := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c < ' ':
			return appendQuoted(b[:start], s)
		case c == '"' || c == '\\':
			b = append(b, s[last:i]...)
			b = append(b, '\\', c)
			last = i + 1
		}
	}
	b = append(b, s[last:]...)
	return append(b, '"')
}

func appendQuoted(b []byte, s string) []byte {
	enc, _ := json.Marshal(s)
	return append(b, enc...)
}
