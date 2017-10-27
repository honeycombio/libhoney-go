package libhoney

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gopkg.in/alexcesaro/statsd.v2"
)

// because package level vars get initialized on package inclusion, subsequent
// tests interact with the same variables in a way that is not like how it
// would be used. This function resets things to a blank state.
func resetPackageVars() {
	defaultBuilder = &Builder{}
	sd, _ = statsd.New(statsd.Mute(true))
}

func TestLibhoney(t *testing.T) {
	resetPackageVars()
	conf := Config{
		WriteKey:   "aoeu",
		Dataset:    "oeui",
		SampleRate: 1,
		APIHost:    "http://localhost:8081/",
	}
	err := Init(conf)
	testOK(t, err)
	testEquals(t, cap(responses), 2*DefaultPendingWorkCapacity)
}

func TestNewEvent(t *testing.T) {
	resetPackageVars()
	conf := Config{
		WriteKey:   "aoeu",
		Dataset:    "oeui",
		SampleRate: 1,
		APIHost:    "http://localhost:8081/",
	}
	Init(conf)
	ev := NewEvent()
	testEquals(t, ev.WriteKey, "aoeu")
	testEquals(t, ev.Dataset, "oeui")
	testEquals(t, ev.SampleRate, uint(1))
	testEquals(t, ev.APIHost, "http://localhost:8081/")
}

func TestAddField(t *testing.T) {
	resetPackageVars()
	conf := Config{
		WriteKey:   "aoeu",
		Dataset:    "oeui",
		SampleRate: 1,
		APIHost:    "http://localhost:8081/",
	}
	Init(conf)
	ev := NewEvent()
	ev.AddField("strVal", "bar")
	ev.AddField("intVal", 5)
	ev.AddField("floatVal", 3.123)
	ev.AddField("uintVal", uint(4))
	ev.AddField("boolVal", true)
	testEquals(t, ev.data["strVal"], "bar")
	testEquals(t, ev.data["intVal"], 5)
	testEquals(t, ev.data["floatVal"], 3.123)
	testEquals(t, ev.data["uintVal"], uint(4))
	testEquals(t, ev.data["boolVal"], true)
}

type Aich struct {
	F1 string
	F2 int
	F3 int `json:"effthree"`
	F4 int `json:"-"`
	F5 int `json:"f5,omitempty"`
	h1 int
	h2 []string
	P1 *int
	P2 *int
	P3 []int
	P4 map[string]int
}

func TestAddStruct(t *testing.T) {
	intPtr := new(int)
	conf := Config{}
	Init(conf)
	ev := NewEvent()
	r := Aich{
		F1: "snth",
		F2: 5,
		F3: 6,
		F4: 7,
		F5: 8,
		h1: 9,
		h2: []string{"a", "b"},
		P2: intPtr,
	}
	ev.Add(r)
	marshalled, err := json.Marshal(ev.data)
	assert.Nil(t, err)
	assert.JSONEq(t,
		`{
			"F1": "snth",
			"F2": 5,
			"P2": 0,
			"effthree": 6,
			"f5": 8
		}`,
		string(marshalled))
}

func TestAddStructPtr(t *testing.T) {
	resetPackageVars()
	intPtr := new(int)
	conf := Config{}
	Init(conf)
	ev := NewEvent()
	r := Aich{
		F1: "snth",
		F2: 5,
		F3: 6,
		F4: 7,
		F5: 8,
		h1: 9,
		h2: []string{"a", "b"},
		P2: intPtr,
	}
	ev.Add(&r)

	marshalled, err := json.Marshal(ev.data)
	assert.Nil(t, err)
	assert.JSONEq(t,
		`{
			"F1": "snth",
			"F2": 5,
			"P2": 0,
			"effthree": 6,
			"f5": 8
		}`,
		string(marshalled))
}

type Jay struct {
	F1 string
	F2 Aich
	F3 struct{ A []int }
	F4 []string
}

func TestAddDeepStruct(t *testing.T) {
	resetPackageVars()
	conf := Config{}
	Init(conf)
	ev := NewEvent()
	r := Aich{
		F1: "snth",
		F2: 5,
		F3: 6,
	}
	j := Jay{
		F1: "ntdh",
		F2: r,
		F3: struct{ A []int }{[]int{2, 3}},
		F4: []string{"eoeoe", "ththt"},
	}
	err := ev.Add(j)
	testOK(t, err)
	testEquals(t, ev.data["F1"], j.F1)
	testEquals(t, ev.data["F2"], r)
	testEquals(t, ev.data["F3"], struct{ A []int }{[]int{2, 3}})
	testEquals(t, ev.data["F4"], []string{"eoeoe", "ththt"})
}

func TestAddSlice(t *testing.T) {
	resetPackageVars()
	conf := Config{}
	Init(conf)
	ev := NewEvent()
	sl := []string{"a", "b", "c"}
	err := ev.Add(sl)
	testErr(t, err)
}

func TestAddMap(t *testing.T) {
	resetPackageVars()
	conf := Config{}
	Init(conf)
	r := Aich{
		F1: "snth",
		F2: 5,
		F3: 6,
	}
	mStr := map[string]interface{}{
		"a": "valA",
		"b": 2,
		"c": 5.123,
		"d": []string{"d_a", "d_b"},
		"e": r,
	}
	ev := NewEvent()
	err := ev.Add(mStr)
	testOK(t, err)
	testEquals(t, ev.data["a"], mStr["a"].(string))
	testEquals(t, ev.data["b"], int(mStr["b"].(int)))
	testEquals(t, ev.data["c"], float64(mStr["c"].(float64)))
	testEquals(t, ev.data["d"], mStr["d"])
	testEquals(t, ev.data["e"], r)

	mInt := map[uint8]interface{}{
		1: "valA",
		2: 2,
		3: 5.123,
		4: []string{"d_a", "d_b"},
		6: r,
	}
	ev = NewEvent()
	err = ev.Add(mInt)
	t.Logf("ev.data is %+v", ev.data)
	testOK(t, err)
	testEquals(t, ev.data["1"], mInt[1].(string))
	testEquals(t, ev.data["2"], mInt[2].(int))
	testEquals(t, ev.data["3"], float64(mInt[3].(float64)))
	testEquals(t, ev.data["4"], mInt[4])
	testEquals(t, ev.data["6"], mInt[6])

	ev = NewEvent()
	mStrStr := map[string]string{
		"1": "2",
	}

	err = ev.Add(mStrStr)
	testOK(t, err)
	testEquals(t, ev.data["1"], "2")
}

func TestAddMapPtr(t *testing.T) {
	resetPackageVars()
	conf := Config{}
	Init(conf)
	r := Aich{
		F1: "snth",
		F2: 5,
		F3: 6,
	}
	mStr := map[string]interface{}{
		"a": "valA",
		"b": 2,
		"c": 5.123,
		"d": []string{"d_a", "d_b"},
		"e": r,
	}
	ev := NewEvent()
	err := ev.Add(&mStr)
	t.Logf("ev.data is %+v", ev.data)
	testOK(t, err)
	testEquals(t, ev.data["a"], mStr["a"].(string))
	testEquals(t, ev.data["b"], int(mStr["b"].(int)))
	testEquals(t, ev.data["c"], float64(mStr["c"].(float64)))
	testEquals(t, ev.data["d"], mStr["d"])
	testEquals(t, ev.data["e"], r)

}

func TestAddFunc(t *testing.T) {
	resetPackageVars()
	conf := Config{}
	Init(conf)
	keys := []string{
		"aoeu",
		"oeui",
		"euid",
	}
	vals := []interface{}{
		"str",
		5,
		[]string{"d_a", "d_b"},
	}
	i := 0
	myFn := func() (string, interface{}, error) {
		if i >= 3 {
			return "", nil, errors.New("all done")
		}
		str := keys[i]
		val := vals[i]
		i++
		return str, val, nil
	}

	ev := NewEvent()
	ev.AddFunc(myFn)
	t.Logf("data has %+v", ev.data)
	testEquals(t, ev.data["aoeu"], vals[0].(string))
	testEquals(t, ev.data["oeui"], int(vals[1].(int)))
	testEquals(t, ev.data["euid"], vals[2])
	testEquals(t, len(ev.data), 3)
}

func TestAddFuncUsingAdd(t *testing.T) {
	resetPackageVars()
	conf := Config{}
	Init(conf)
	myFn := func() (string, interface{}, error) {
		return "", "", nil
	}
	ev := NewEvent()
	err := ev.Add(myFn)
	testErr(t, err)
}

func TestAddDynamicField(t *testing.T) {
	resetPackageVars()
	Init(Config{})
	i := 0
	myFn := func() interface{} {
		v := i
		i++
		return v
	}
	AddDynamicField("incrementingInt", myFn)
	ev1 := NewEvent()
	testEquals(t, ev1.data["incrementingInt"], 0)
	ev2 := NewEvent()
	testEquals(t, ev2.data["incrementingInt"], 1)
}

func TestNewBuilder(t *testing.T) {
	resetPackageVars()
	conf := Config{
		WriteKey:   "aoeu",
		Dataset:    "oeui",
		SampleRate: 1,
		APIHost:    "http://localhost:8081/",
	}
	Init(conf)
	b := NewBuilder()
	testEquals(t, b.WriteKey, "aoeu")
	testEquals(t, b.Dataset, "oeui")
	testEquals(t, b.SampleRate, uint(1))
	testEquals(t, b.APIHost, "http://localhost:8081/")
}

func TestCloneBuilder(t *testing.T) {
	resetPackageVars()
	conf := Config{
		WriteKey:   "aoeu",
		Dataset:    "oeui",
		SampleRate: 1,
		APIHost:    "http://localhost:8081/",
	}
	Init(conf)
	b := NewBuilder()
	b2 := b.Clone()
	b2.WriteKey = "newAAAA"
	b2.Dataset = "newoooo"
	b2.SampleRate = 2
	b2.APIHost = "differentAPIHost"
	// old builder didn't change
	testEquals(t, b.WriteKey, "aoeu")
	testEquals(t, b.Dataset, "oeui")
	testEquals(t, b.SampleRate, uint(1))
	testEquals(t, b.APIHost, "http://localhost:8081/")
	// cloned builder has new values
	testEquals(t, b2.WriteKey, "newAAAA")
	testEquals(t, b2.Dataset, "newoooo")
	testEquals(t, b2.SampleRate, uint(2))
	testEquals(t, b2.APIHost, "differentAPIHost")
}

func TestBuilderDynFields(t *testing.T) {
	resetPackageVars()
	var i int
	myIntFn := func() interface{} {
		v := i
		i++
		return v
	}
	strs := []string{
		"aoeu",
		"oeui",
		"euid",
	}
	var j int
	myStrFn := func() interface{} {
		v := j
		j++
		return strs[v]
	}
	f := 1.0
	myFloatFn := func() interface{} {
		v := f
		f += 1.2
		return v
	}
	AddDynamicField("ints", myIntFn)
	b := NewBuilder()
	b.AddDynamicField("strs", myStrFn)
	testEquals(t, len(defaultBuilder.dynFields), 1)
	testEquals(t, len(b.dynFields), 2)

	ev1 := NewEvent()
	testEquals(t, ev1.data["ints"], 0)
	ev2 := b.NewEvent()
	testEquals(t, ev2.data["ints"], 1)
	testEquals(t, ev2.data["strs"], "aoeu")

	b2 := b.Clone()
	b2.AddDynamicField("floats", myFloatFn)
	ev3 := NewEvent()
	testEquals(t, ev3.data["ints"], 2)
	testEquals(t, ev3.data["strs"], nil)
	ev4 := b.NewEvent()
	testEquals(t, ev4.data["ints"], 3)
	testEquals(t, ev4.data["strs"], "oeui")
	ev5 := b2.NewEvent()
	testEquals(t, ev5.data["ints"], 4)
	testEquals(t, ev5.data["strs"], "euid")
	testEquals(t, ev5.data["floats"], 1.0)
}

func TestBuilderStaticFields(t *testing.T) {
	resetPackageVars()
	// test you can add fields to a builder and events get them
	b := NewBuilder()
	b.AddField("intF", 1)
	b.AddField("strF", "aoeu")
	ev := b.NewEvent()
	testEquals(t, ev.data["intF"], 1)
	testEquals(t, ev.data["strF"], "aoeu")
	// test you can clone a builder and events get the cloned data
	b2 := b.Clone()
	ev2 := b2.NewEvent()
	testEquals(t, ev2.data["intF"], 1)
	testEquals(t, ev2.data["strF"], "aoeu")
	// test that you can add new fields to the cloned builder
	// and events get them
	b2.AddField("floatF", 1.234)
	ev3 := b2.NewEvent()
	testEquals(t, ev3.data["intF"], 1)
	testEquals(t, ev3.data["strF"], "aoeu")
	testEquals(t, ev3.data["floatF"], 1.234)
	// test that the old builder didn't get the metrics added to
	// the new builder
	ev4 := b.NewEvent()
	testEquals(t, ev4.data["intF"], 1)
	testEquals(t, ev4.data["strF"], "aoeu")
	testEquals(t, ev4.data["floatF"], nil)
}

func TestSendTime(t *testing.T) {
	resetPackageVars()
	testTx := &MockOutput{}
	Init(Config{
		WriteKey: "foo",
		Dataset:  "bar",
		Output:   testTx,
	})

	now := time.Now().Truncate(time.Millisecond)
	expected := map[string]interface{}{"event_time": now}

	tsts := []struct {
		key string
		val interface{}
	}{
		{"event_time", now},
		{"", map[string]interface{}{"event_time": now}},
		{"", struct {
			Time time.Time `json:"event_time"`
		}{now}},
	}

	for i, tt := range tsts {
		ev := NewEvent()
		if tt.key != "" {
			ev.AddField(tt.key, tt.val)
		} else {
			ev.Add(tt.val)
		}
		err := ev.Send()
		testOK(t, err)
		testEquals(t, len(testTx.Events()), i+1)
		testEquals(t, testTx.Events()[i].Fields(), expected)
	}
}

func TestSendPresampledErrors(t *testing.T) {
	resetPackageVars()
	testTx := &MockOutput{}
	Init(Config{Output: testTx})

	tsts := []struct {
		ev     *Event
		expErr error
	}{
		{
			ev:     &Event{},
			expErr: errors.New("No metrics added to event. Won't send empty event."),
		},
		{
			ev: &Event{
				fieldHolder: fieldHolder{
					data: map[string]interface{}{"a": 1},
				},
			},
			expErr: errors.New("No APIHost for Honeycomb. Can't send to the Great Unknown."),
		},
		{
			ev: &Event{
				fieldHolder: fieldHolder{
					data: map[string]interface{}{"a": 1},
				},
				APIHost: "foo",
			},
			expErr: errors.New("No WriteKey specified. Can't send event."),
		},
		{
			ev: &Event{
				fieldHolder: fieldHolder{
					data: map[string]interface{}{"a": 1},
				},
				APIHost:  "foo",
				WriteKey: "bar",
			},
			expErr: errors.New("No Dataset for Honeycomb. Can't send datasetless."),
		},
		{
			ev: &Event{
				fieldHolder: fieldHolder{
					data: map[string]interface{}{"a": 1},
				},
				APIHost:  "foo",
				WriteKey: "bar",
				Dataset:  "baz",
			},
			expErr: nil,
		},
	}
	for i, tst := range tsts {
		err := tst.ev.SendPresampled()
		testEquals(t, err, tst.expErr, fmt.Sprintf("testing expected error from test object %d", i))
	}
}

// TestPresampledSendSamplerate verifies that SendPresampled does no sampling
func TestPresampledSendSamplerate(t *testing.T) {
	resetPackageVars()
	Init(Config{})
	testTx := &MockOutput{}
	testTx.Start()

	tx = testTx

	ev := &Event{
		fieldHolder: fieldHolder{
			data: map[string]interface{}{"a": 1},
		},
		APIHost:    "foo",
		WriteKey:   "bar",
		Dataset:    "baz",
		SampleRate: 5,
	}

	for i := 0; i < 5; i++ {
		err := ev.SendPresampled()
		testOK(t, err)

		testEquals(t, len(testTx.Events()), i+1)
		testEquals(t, testTx.Events()[i].SampleRate, uint(5))
	}
}

// TestSendSamplerate verifies that Send samples
func TestSendSamplerate(t *testing.T) {
	resetPackageVars()
	Init(Config{})
	testTx := &MockOutput{}
	testTx.Start()
	rand.Seed(1)

	tx = testTx

	ev := &Event{
		fieldHolder: fieldHolder{
			data: map[string]interface{}{"a": 1},
		},
		APIHost:    "foo",
		WriteKey:   "bar",
		Dataset:    "baz",
		SampleRate: 2,
	}
	for i := 0; i < 10; i++ {
		err := ev.Send()
		testOK(t, err)
	}
	testEquals(t, len(testTx.Events()), 4, "expected testTx num events incorrect")
	for _, ev := range testTx.Events() {
		testEquals(t, ev.SampleRate, uint(2))
	}
}

type testTransport struct {
	invoked bool
}

func (tr *testTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	tr.invoked = true
	return &http.Response{Body: ioutil.NopCloser(bytes.NewReader(nil))}, nil
}

func TestSendTestTransport(t *testing.T) {
	tr := &testTransport{}
	Init(Config{
		WriteKey:  "foo",
		Dataset:   "bar",
		Transport: tr,
	})

	err := SendNow(map[string]interface{}{"foo": 3})
	tx.Stop()  // flush unsent events
	tx.Start() // reopen tx.muster channel
	testOK(t, err)
	testEquals(t, tr.invoked, true)
}

func TestChannelMembers(t *testing.T) {
	resetPackageVars()
	Init(Config{})

	// adding channels directly using .AddField
	ev := NewEvent()
	ev.AddField("intChan", make(chan int))
	marshalled, err := json.Marshal(ev.data)
	assert.Nil(t, err)
	assert.JSONEq(t, "{}", string(marshalled))

	// adding a struct with a channel in it to an event
	type StructWithChan struct {
		A int
		B string
		C chan int
	}
	structWithChan := &StructWithChan{
		A: 1,
		B: "hello",
		C: make(chan int),
	}

	ev2 := NewEvent()
	ev2.Add(structWithChan)

	marshalled2, err := json.Marshal(ev2.data)
	assert.JSONEq(t, `{"A": 1, "B": "hello"}`, string(marshalled2))

	// adding a struct with a struct-valued field containing a channel
	type ChanInField struct {
		CStruct *StructWithChan
		D       int
	}

	chanInField := &ChanInField{}
	chanInField.CStruct = &StructWithChan{
		A: 1,
		B: "hello",
		C: make(chan int),
	}
	chanInField.D = 2

	ev3 := NewEvent()
	ev3.Add(chanInField)

	testEquals(t, ev3.data["A"], nil)
	testEquals(t, ev3.data["B"], nil)
	testEquals(t, ev3.data["C"], nil)
	testEquals(t, ev3.data["D"], 2)

	// adding a struct containing an embedded struct containing a channel
	type ChanInEmbedded struct {
		StructWithChan
		D int
	}

	chanInEmbedded := &ChanInEmbedded{}
	chanInEmbedded.A = 1
	chanInEmbedded.B = "hello"
	chanInEmbedded.C = make(chan int)
	chanInEmbedded.D = 2

	ev4 := NewEvent()
	ev4.Add(chanInField)

	testEquals(t, ev4.data["A"], nil)
	testEquals(t, ev4.data["B"], nil)
	testEquals(t, ev4.data["C"], nil)
	testEquals(t, ev4.data["D"], 2)
}

func TestEventMarshal(t *testing.T) {
	e := &Event{SampleRate: 1}
	e.data = map[string]interface{}{"a": 1}
	b, err := json.Marshal(e)
	testOK(t, err)
	testEquals(t, string(b), `{"data":{"a":1}}`)

	e.Timestamp = time.Unix(1476309645, 0).UTC()
	e.SampleRate = 5
	b, err = json.Marshal(e)
	testOK(t, err)
	testEquals(t, string(b), `{"data":{"a":1},"samplerate":5,"time":"2016-10-12T22:00:45Z"}`)
}

func TestDataRace1(t *testing.T) {
	e := &Event{SampleRate: 1}
	e.data = map[string]interface{}{"a": 1}

	var err error
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		_, err = json.Marshal(e)
		wg.Done()
	}()

	go func() {
		mStr := map[string]interface{}{
			"a": "valA",
			"b": 2,
			"c": 5.123,
			"d": []string{"d_a", "d_b"},
		}
		e.AddField("b", 2)
		e.Add(mStr)
		wg.Done()
	}()

	wg.Wait()
	testOK(t, err)
}

func TestDataRace2(t *testing.T) {
	b := &Builder{}
	b.data = map[string]interface{}{"a": 1}

	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		_ = b.NewEvent()
		wg.Done()
	}()

	go func() {
		b.AddField("b", 2)
		wg.Done()
	}()

	wg.Wait()
}

func TestDataRace3(t *testing.T) {
	resetPackageVars()
	testTx := &MockOutput{}
	Init(Config{
		Output: testTx,
	})
	testTx.Start()

	tx = testTx

	ev := &Event{
		fieldHolder: fieldHolder{
			data: map[string]interface{}{"a": 1},
		},
		APIHost:    "foo",
		WriteKey:   "bar",
		Dataset:    "baz",
		SampleRate: 1,
	}

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		err := ev.Send()
		testOK(t, err)
		wg.Done()
	}()

	go func() {
		ev.AddField("newField", 1)
		wg.Done()
	}()

	wg.Wait()

	testEquals(t, len(testTx.Events()), 1, "expected testTx num datas incorrect")
}

//
//  Examples
//

func Example() {
	// call Init before using libhoney
	Init(Config{
		WriteKey:   "abcabc123123defdef456456",
		Dataset:    "Example Service",
		SampleRate: 1,
	})
	// when all done, call close
	defer Close()

	// create an event, add fields
	ev := NewEvent()
	ev.AddField("duration_ms", 153.12)
	ev.AddField("method", "get")
	// send the event
	ev.Send()
}

func ExampleAddDynamicField() {
	// adds the number of goroutines running at event
	// creation time to every event sent to Honeycomb.
	AddDynamicField("num_goroutines",
		func() interface{} { return runtime.NumGoroutine() })
}
