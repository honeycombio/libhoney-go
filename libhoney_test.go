package libhoney

import (
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

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
	testEquals(t, cap(responses), 2*defaultpendingWorkCapacity)
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
	testEquals(t, ev.data["F1"], "snth")
	testEquals(t, ev.data["F2"], 5)
	testEquals(t, ev.data["effthree"], 6)
	testEquals(t, ev.data["f5"], 8)
	testEquals(t, ev.data["P2"], intPtr)
	_, ok := ev.data["F4"]
	testEquals(t, ok, false)
	_, ok = ev.data["h1"]
	testEquals(t, ok, false)
	_, ok = ev.data["h3"]
	testEquals(t, ok, false)
	_, ok = ev.data["h4"]
	testEquals(t, ok, false)
	_, ok = ev.data["P1"]
	testEquals(t, ok, false)
	_, ok = ev.data["P3"]
	testEquals(t, ok, false)
	_, ok = ev.data["P4"]
	testEquals(t, ok, false)
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
	testEquals(t, ev.data["F1"], "snth")
	testEquals(t, ev.data["F2"], 5)
	testEquals(t, ev.data["effthree"], 6)
	testEquals(t, ev.data["f5"], 8)
	testEquals(t, ev.data["P2"], intPtr)
	_, ok := ev.data["F4"]
	testEquals(t, ok, false)
	_, ok = ev.data["h3"]
	testEquals(t, ok, false)
	_, ok = ev.data["h4"]
	testEquals(t, ok, false)
	_, ok = ev.data["P1"]
	testEquals(t, ok, false)
	_, ok = ev.data["P3"]
	testEquals(t, ok, false)
	_, ok = ev.data["P4"]
	testEquals(t, ok, false)
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
		i += 1
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
		i += 1
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
	Init(Config{})
	var i int
	myIntFn := func() interface{} {
		v := i
		i += 1
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
		j += 1
		return strs[v]
	}
	var f float64 = 1.0
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
	Init(Config{})
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
	Init(Config{
		WriteKey: "foo",
		Dataset:  "bar",
	})
	testTx := &txTestClient{}
	testTx.Start()

	tx = testTx

	now := time.Now().Truncate(time.Millisecond)
	expected := fmt.Sprintf(`{"event_time":"%s"}`, now.Format(time.RFC3339Nano))

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
		testEquals(t, len(testTx.datas), i+1)
		testEquals(t, string(testTx.datas[i]), expected)
	}
}

func TestChannelMembers(t *testing.T) {
	resetPackageVars()
	Init(Config{})

	// adding channels directly using .AddField
	ev := NewEvent()
	ev.AddField("intChan", make(chan int))
	testEquals(t, ev.data["intChan"], nil)

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

	testEquals(t, ev2.data["A"], 1)
	testEquals(t, ev2.data["B"], "hello")
	testEquals(t, ev2.data["C"], nil)

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
