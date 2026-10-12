package transmission

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestEventJSONIsEquivalentToEncodingJSON(t *testing.T) {
	var nilMap map[string]int
	var nilPtr *int
	events := map[string]*Event{
		"span": realisticSpan(),
		"empty": {
			Data: map[string]any{},
		},
		"edge cases": {
			SampleRate: 7,
			Timestamp:  time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("x", 3600)),
			Data: map[string]any{
				"html":      `<a href="x">&amp;</a>`,
				"control":   "tab\tnl\nnul\x00bell\x07\b\f\r\x1f",
				"unicode":   "héllo ✓     😀",
				"invalid":   "bad\xffutf8\xc3",
				"quote":     `"back\slash"`,
				"empty":     "",
				"tiny":      1e-7,
				"huge":      1e21,
				"negative":  -0.000001,
				"zero":      0.0,
				"f32":       float32(3.14159),
				"f32 tiny":  float32(1e-9),
				"nan":       math.NaN(),
				"inf":       math.Inf(-1),
				"ints":      []any{int8(-1), int16(2), int32(3), int64(4), uint8(5), uint16(6), uint32(7), uint64(8), uint(9)},
				"nil":       nil,
				"nil map":   nilMap,
				"nil ptr":   nilPtr,
				"nil slice": []string(nil),
				"mixed":     []any{"a", 1, nil, 2.5, true, map[string]int{"x": 1}},
				"bad slice": []any{"a", math.NaN()},
				"duration":  3 * time.Second,
				"time":      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				"struct":    struct{ A []string }{A: []string{"<x>"}},
				"named":     json.Number("12.5"),
				"raw":       json.RawMessage(`{ "spaced" : true }`),
			},
		},
	}

	for name, ev := range events {
		t.Run(name, func(t *testing.T) {
			want, wantErr := referenceEventJSON(ev)
			got, err := ev.MarshalJSON()
			testEquals(t, err, wantErr)
			if !reflect.DeepEqual(decodeJSON(t, got), decodeJSON(t, want)) {
				t.Errorf("got  %s\nwant %s", got, want)
			}
		})
	}
}

func TestEventJSONEscapesKeys(t *testing.T) {
	ev := &Event{Data: map[string]any{"quote\"key\\": 1}}
	got, err := ev.MarshalJSON()
	testOK(t, err)
	testEquals(t, string(got), `{"data":{"quote\"key\\":1}}`)
}

func TestEventJSONTimestampOutOfRange(t *testing.T) {
	ev := &Event{Timestamp: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}
	_, wantErr := referenceEventJSON(ev)
	_, err := ev.MarshalJSON()
	if err == nil || wantErr == nil {
		t.Fatalf("expected errors, got %v and reference %v", err, wantErr)
	}
}

func BenchmarkEventJSON(b *testing.B) {
	ev := realisticSpan()
	b.Run("reference", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			referenceEventJSON(ev)
		}
	})
	b.Run("appendJSON", func(b *testing.B) {
		b.ReportAllocs()
		var buf []byte
		for b.Loop() {
			buf, _ = ev.appendJSON(buf[:0])
		}
	})
}

func BenchmarkEncodeBatchJSON(b *testing.B) {
	events := make([]*Event, 100)
	for i := range events {
		events[i] = realisticSpan()
	}
	b.Run("reference", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			// json.Marshal on each event, as the batch encoder used to
			buf := []byte{'['}
			for i, ev := range events {
				if i > 0 {
					buf = append(buf, ',')
				}
				enc, _ := json.Marshal(referenceEvent{ev})
				buf = append(buf, enc...)
			}
			_ = append(buf, ']')
		}
	})
	b.Run("encodeBatchJSON", func(b *testing.B) {
		b.ReportAllocs()
		agg := &batchAgg{}
		var buf bytes.Buffer
		for b.Loop() {
			buf.Reset()
			agg.encodeBatchJSON(events, &buf)
		}
	})
}

func decodeJSON(t *testing.T, b []byte) any {
	t.Helper()
	if b == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("invalid JSON %s: %v", b, err)
	}
	return v
}

// realisticSpan resembles a tracing span for a database query, which is
// dominated by strings, numbers and long SQL text.
func realisticSpan() *Event {
	return &Event{
		SampleRate: 1,
		Timestamp:  time.Date(2026, 10, 9, 13, 56, 45, 769844785, time.UTC),
		Data: map[string]any{
			"name":               "QueryContext",
			"service.name":       "example",
			"trace.trace_id":     "0005238bd84585506c156404969fa9fb",
			"trace.span_id":      "2b09026a93e8c11f",
			"trace.parent_id":    "1db584c991d61a2e",
			"duration_ms":        1.03,
			"meta.type":          "db",
			"meta.beeline_ver":   "1.18.0",
			"meta.local_host":    "app-production-i-0123456789abcdef0",
			"aws.ecs.task.arn":   "arn:aws:ecs:us-east-1:123456789012:task/example-cluster/0123456789abcdef0123456789abcdef",
			"aws.instance_id":    "i-0123456789abcdef0",
			"aws.region":         "us-east-1",
			"db.call":            "QueryContext",
			"db.conns_idle":      48,
			"db.conns_in_use":    2,
			"db.open_conns":      50,
			"db.wait_count":      int64(1341607),
			"db.wait_duration":   time.Duration(60221627891380),
			"db.query":           `SELECT "id", "name", "account_id", "status", "currency", "total_cents", "shipping_address", "created_at", "updated_at", "notes", "customer_id", "owner_id" FROM "orders" WHERE (id = $1) LIMIT 1;`,
			"db.query_args":      []any{"order_01J0Z3K9Q4M5N6P7R8S9T0V1W2"},
			"process.gc.cycles":  uint32(41275),
			"process.mem_in_use": uint64(111999968),
			"process.uptime_s":   9.86,
		},
	}
}

// referenceEventJSON is the encoding/json implementation that appendJSON
// replaced, kept as the definition of the expected output.
func referenceEventJSON(e *Event) ([]byte, error) {
	tPointer := &(e.Timestamp)
	if e.Timestamp.IsZero() {
		tPointer = nil
	}
	sampleRate := e.SampleRate
	if sampleRate == 1 {
		sampleRate = 0
	}
	return json.Marshal(struct {
		Data       referenceMap `json:"data"`
		SampleRate uint         `json:"samplerate,omitempty"`
		Timestamp  *time.Time   `json:"time,omitempty"`
	}{e.Data, sampleRate, tPointer})
}

type referenceEvent struct{ *Event }

func (e referenceEvent) MarshalJSON() ([]byte, error) {
	return referenceEventJSON(e.Event)
}

type referenceMap map[string]any

func (m referenceMap) MarshalJSON() ([]byte, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := bytes.NewBufferString("{")
	first := true
	for _, k := range keys {
		v := m[k]
		if v == nil {
			continue
		}
		val := reflect.ValueOf(v)
		if k := val.Kind(); (k == reflect.Ptr || k == reflect.Slice || k == reflect.Map) && val.IsNil() {
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		if !first {
			out.WriteByte(',')
		}
		first = false
		out.WriteString(`"` + k + `":`)
		out.Write(b)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}
