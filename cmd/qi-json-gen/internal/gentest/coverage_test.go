package gentest

import (
	"testing"
	"time"

	"github.com/qi-lore/qi-json"
	"github.com/qi-lore/qi-json/cmd/qi-json-gen/internal/gentest/xtypes"
)

// TestGeneratedDecodeBranches drives every inline field decoder in the generated
// schemas through its null branch (which succeeds) and its mismatched-type and
// truncated branches (which fail), so the per-field error paths are exercised.
func TestGeneratedDecodeBranches(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		// Config inline numeric closures.
		{"config.level null", `{"level":null}`, false},
		{"config.level wrong", `{"level":"x"}`, true},
		{"config.level truncated", `{"level":`, true},
		{"config.count null", `{"count":null}`, false},
		{"config.count wrong", `{"count":"x"}`, true},
		{"config.count truncated", `{"count":`, true},
		{"config.ratio null", `{"ratio":null}`, false},
		{"config.ratio wrong", `{"ratio":"x"}`, true},
		{"config.ratio truncated", `{"ratio":`, true},
		{"config.size null", `{"size":null}`, false},
		{"config.size wrong", `{"size":"x"}`, true},
		{"config.size truncated", `{"size":`, true},
		// Config inline pointer closures.
		{"config.limit null", `{"limit":null}`, false},
		{"config.limit wrong", `{"limit":"x"}`, true},
		{"config.limit truncated", `{"limit":`, true},
		{"config.deadline null", `{"deadline":null}`, false},
		{"config.deadline wrong", `{"deadline":"x"}`, true},
		{"config.deadline truncated", `{"deadline":`, true},
		{"config.prev null", `{"prev":null}`, false},
		{"config.prev wrong", `{"prev":1}`, true},
		{"config.prev truncated", `{"prev":`, true},
		// Config inline named-scalar closures.
		{"config.timeout null", `{"timeout":null}`, false},
		{"config.timeout wrong", `{"timeout":"x"}`, true},
		{"config.timeout truncated", `{"timeout":`, true},
		{"config.label null", `{"label":null}`, false},
		{"config.label wrong", `{"label":1}`, true},
		{"config.label truncated", `{"label":`, true},
		{"config.flag null", `{"flag":null}`, false},
		{"config.flag wrong", `{"flag":1}`, true},
		{"config.flag truncated", `{"flag":`, true},
		{"config.status null", `{"status":null}`, false},
		{"config.status wrong", `{"status":1}`, true},
		{"config.status truncated", `{"status":`, true},
		{"config.mode null", `{"mode":null}`, false},
		{"config.mode wrong", `{"mode":1}`, true},
		{"config.mode truncated", `{"mode":`, true},
		// Config's synthesized-pointer foreign field (schemaPtrCodec).
		{"config.maybe null", `{"maybe":null}`, false},
		{"config.maybe wrong", `{"maybe":1}`, true},
		{"config.maybe truncated", `{"maybe":`, true},

		// Point closures, reached through a nested object.
		{"point.x null", `{"origin":{"x":null}}`, false},
		{"point.x wrong", `{"origin":{"x":"s"}}`, true},
		{"point.x truncated", `{"origin":{"x":`, true},
		{"point.y null", `{"origin":{"y":null}}`, false},
		{"point.y wrong", `{"origin":{"y":"s"}}`, true},
		{"point.y truncated", `{"origin":{"y":`, true},

		// Foreign Thing closures, reached through the object field.
		{"thing.count null", `{"thing":{"count":null}}`, false},
		{"thing.count wrong", `{"thing":{"count":"s"}}`, true},
		{"thing.count truncated", `{"thing":{"count":`, true},
		{"thing.tiny null", `{"thing":{"tiny":null}}`, false},
		{"thing.tiny wrong", `{"thing":{"tiny":"s"}}`, true},
		{"thing.tiny truncated", `{"thing":{"tiny":`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Config
			err := qijson.UnmarshalInto[Config]([]byte(tt.doc), &c)
			if tt.wantErr && err == nil {
				t.Errorf("UnmarshalInto(%s) = nil, want an error", tt.doc)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("UnmarshalInto(%s) = %v, want nil", tt.doc, err)
			}
		})
	}
}

// TestGeneratedHelperErrorBranches drives the read-error branch of every
// generated element helper by feeding a mismatched element type.
func TestGeneratedHelperErrorBranches(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"matrix qiDecodeInt", `{"matrix":["s","s"]}`},
		{"matrix qiDecodeInt truncated", `{"matrix":[`},
		{"scores qiDecodeInt32", `{"scores":{"k":"s"}}`},
		{"points qiDecodePoint", `{"points":[1]}`},
		{"ports qiDecodePoint", `{"ports":{"k":1}}`},
		{"durations qiDecodeTimeDuration", `{"durations":["s"]}`},
		{"labels qiDecodeXtypesLabel", `{"labels":{"k":1}}`},
		{"statuses qiDecodeStatus", `{"statuses":[1]}`},
		{"modes qiDecodeMode", `{"modes":{"k":1}}`},
		{"things qiDecodeXtypesThing", `{"things":[1]}`},
		{"twin.b qiDecodeXtypesInner", `{"twin":{"b":[1]}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Config
			if err := qijson.UnmarshalInto[Config]([]byte(tt.doc), &c); err == nil {
				t.Errorf("UnmarshalInto(%s) = nil, want an error", tt.doc)
			}
		})
	}
}

// TestGeneratedNumsHelperErrors drives the Nums element helpers' read-error and
// range-error branches.
func TestGeneratedNumsHelperErrors(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"ints wrong", `{"ints":["s"]}`},
		{"ints overflow", `{"ints":[200]}`},
		{"uints wrong", `{"uints":["s"]}`},
		{"uints overflow", `{"uints":[4294967296]}`},
		{"float wrong", `{"float":["s"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var n Nums
			if err := qijson.UnmarshalInto[Nums]([]byte(tt.doc), &n); err == nil {
				t.Errorf("UnmarshalInto(%s) = nil, want an error", tt.doc)
			}
		})
	}
}

// TestGeneratedHooksDecodeBranches drives the omit-hook fields' decoders through
// their null and read-error branches.
func TestGeneratedHooksDecodeBranches(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		{"stamp null", `{"stamp":null}`, false},
		{"stamp wrong", `{"stamp":"s"}`, true},
		{"stamp truncated", `{"stamp":`, true},
		{"tag null", `{"tag":null}`, false},
		{"tag wrong", `{"tag":1}`, true},
		{"tag truncated", `{"tag":`, true},
		{"keep null", `{"keep":null}`, false},
		{"keep wrong", `{"keep":"s"}`, true},
		{"keep truncated", `{"keep":`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Hooks
			err := qijson.UnmarshalInto[Hooks]([]byte(tt.doc), &h)
			if tt.wantErr && err == nil {
				t.Errorf("UnmarshalInto(%s) = nil, want an error", tt.doc)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("UnmarshalInto(%s) = %v, want nil", tt.doc, err)
			}
		})
	}
}

// TestGeneratedStrictEncode checks the -strict schema's EncodeJSON path.
func TestGeneratedStrictEncode(t *testing.T) {
	data, err := qijson.Marshal[Strict](Strict{A: 1, B: "x"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(data), `{"a":1,"b":"x"}`; got != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}
}

// TestGeneratedEncodePaths drives the generated encode helpers and the
// non-nil pointer encode branches by marshalling populated values.
func TestGeneratedEncodePaths(t *testing.T) {
	if _, err := qijson.Marshal[Nums](Nums{
		Ints:  []int8{1, -2},
		Uints: []uint32{3, 4},
		Float: []float32{1.5, 2},
	}); err != nil {
		t.Fatalf("Marshal[Nums]: %v", err)
	}

	if _, err := qijson.Marshal[Point](Point{X: 1, Y: 2}); err != nil {
		t.Fatalf("Marshal[Point]: %v", err)
	}
	var p Point
	if err := qijson.UnmarshalInto[Point]([]byte(`{"x":1,"y":2}`), &p); err != nil {
		t.Fatalf("UnmarshalInto[Point]: %v", err)
	}

	limit := int32(1)
	deadline := 5 * time.Second
	prev := Status("idle")
	if _, err := qijson.Marshal[Config](Config{
		Limit:     &limit,
		Deadline:  &deadline,
		Prev:      &prev,
		Points:    []Point{{X: 1, Y: 2}},
		Ports:     map[string]Point{"p": {X: 1, Y: 2}},
		Durations: []time.Duration{1},
		Labels:    map[string]xtypes.Label{"k": "v"},
		Statuses:  []Status{"s"},
		Modes:     map[string]Mode{"m": true},
	}); err != nil {
		t.Fatalf("Marshal[Config]: %v", err)
	}
}

// TestGeneratedDecodeSuccessBranches drives the value-assignment branches of the
// omit-hook field decoders, which only the error branches reached before.
func TestGeneratedDecodeSuccessBranches(t *testing.T) {
	var h Hooks
	if err := qijson.UnmarshalInto[Hooks]([]byte(`{"stamp":5,"tag":"x","keep":7}`), &h); err != nil {
		t.Fatalf("UnmarshalInto[Hooks]: %v", err)
	}
	if h.Stamp != 5 || h.Tag != "x" || h.Keep != 7 {
		t.Errorf("Hooks = %+v, want {5 x 0 7}", h)
	}
}

// TestGeneratedHelperRangeErrors drives the range branches of the element
// helpers not already exercised by the field range tests.
func TestGeneratedHelperRangeErrors(t *testing.T) {
	var c Config
	if err := qijson.UnmarshalInto[Config]([]byte(`{"scores":{"k":2147483648}}`), &c); err == nil {
		t.Error("UnmarshalInto(scores int32 overflow) = nil, want a range error")
	}
	var n Nums
	if err := qijson.UnmarshalInto[Nums]([]byte(`{"float":[1e40]}`), &n); err == nil {
		t.Error("UnmarshalInto(float32 overflow) = nil, want a range error")
	}
}
