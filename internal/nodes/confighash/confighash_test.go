package confighash

import "testing"

func TestCompute_OrderInsensitive(t *testing.T) {
	a := Compute(
		[]UserEntry{
			{Email: "alice@vless1", UUID: "u1", InboundTag: "vless1", Enabled: true},
			{Email: "bob@trojan1", UUID: "p1", InboundTag: "trojan1", Enabled: true},
		},
		[]InboundEntry{
			{Tag: "vless1", TrafficRate: 1.0},
			{Tag: "trojan1", TrafficRate: 2.0},
		},
	)
	b := Compute(
		[]UserEntry{
			{Email: "bob@trojan1", UUID: "p1", InboundTag: "trojan1", Enabled: true},
			{Email: "alice@vless1", UUID: "u1", InboundTag: "vless1", Enabled: true},
		},
		[]InboundEntry{
			{Tag: "trojan1", TrafficRate: 2.0},
			{Tag: "vless1", TrafficRate: 1.0},
		},
	)
	if a != b {
		t.Fatalf("hash should be order-insensitive; %s != %s", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("expected 64-hex sha256, got %d chars: %q", len(a), a)
	}
}

func TestCompute_SensitiveToFields(t *testing.T) {
	base := Compute(
		[]UserEntry{{Email: "u@vless1", UUID: "u1", InboundTag: "vless1", Enabled: true}},
		[]InboundEntry{{Tag: "vless1", TrafficRate: 1.0}},
	)
	cases := map[string]string{
		"uuid changed": Compute(
			[]UserEntry{{Email: "u@vless1", UUID: "u2", InboundTag: "vless1", Enabled: true}},
			[]InboundEntry{{Tag: "vless1", TrafficRate: 1.0}},
		),
		"email changed": Compute(
			[]UserEntry{{Email: "v@vless1", UUID: "u1", InboundTag: "vless1", Enabled: true}},
			[]InboundEntry{{Tag: "vless1", TrafficRate: 1.0}},
		),
		"enabled changed": Compute(
			[]UserEntry{{Email: "u@vless1", UUID: "u1", InboundTag: "vless1", Enabled: false}},
			[]InboundEntry{{Tag: "vless1", TrafficRate: 1.0}},
		),
		"tag changed": Compute(
			[]UserEntry{{Email: "u@vless1", UUID: "u1", InboundTag: "vless1", Enabled: true}},
			[]InboundEntry{{Tag: "vless2", TrafficRate: 1.0}},
		),
		"rate changed": Compute(
			[]UserEntry{{Email: "u@vless1", UUID: "u1", InboundTag: "vless1", Enabled: true}},
			[]InboundEntry{{Tag: "vless1", TrafficRate: 2.0}},
		),
		"user added": Compute(
			[]UserEntry{
				{Email: "u@vless1", UUID: "u1", InboundTag: "vless1", Enabled: true},
				{Email: "u2@vless1", UUID: "u2", InboundTag: "vless1", Enabled: true},
			},
			[]InboundEntry{{Tag: "vless1", TrafficRate: 1.0}},
		),
	}
	for name, h := range cases {
		if h == base {
			t.Errorf("%s: hash should differ from base, both = %s", name, h)
		}
	}
}

func TestCompute_EmptyStable(t *testing.T) {
	a := Compute(nil, nil)
	b := Compute([]UserEntry{}, []InboundEntry{})
	if a != b {
		t.Fatalf("nil and empty slices should hash identically; %s vs %s", a, b)
	}
}

func TestHashFromXrayJSON_MatchesCompute(t *testing.T) {
	cfg := `{
		"inbounds": [
			{"tag":"vless1","settings":{"clients":[
				{"id":"u1","email":"alice@vless1"},
				{"id":"u2","email":"bob@vless1"}
			]}},
			{"tag":"trojan1","settings":{"clients":[
				{"password":"p1","email":"alice@trojan1"}
			]}}
		]
	}`
	got := HashFromXrayJSON(cfg)
	want := Compute(
		[]UserEntry{
			{Email: "alice@vless1", UUID: "u1", InboundTag: "vless1", Enabled: true},
			{Email: "bob@vless1", UUID: "u2", InboundTag: "vless1", Enabled: true},
			{Email: "alice@trojan1", UUID: "p1", InboundTag: "trojan1", Enabled: true},
		},
		[]InboundEntry{
			{Tag: "vless1", TrafficRate: 0},
			{Tag: "trojan1", TrafficRate: 0},
		},
	)
	if got != want {
		t.Fatalf("HashFromXrayJSON mismatch:\n  got  %s\n  want %s", got, want)
	}
}

func TestHashFromXrayJSON_Empty(t *testing.T) {
	if HashFromXrayJSON("") != "" {
		t.Fatal("empty input should produce empty hash")
	}
}

func TestHashFromXrayJSON_InvalidJSONStable(t *testing.T) {
	a := HashFromXrayJSON("not json {{{")
	b := HashFromXrayJSON("not json {{{")
	if a == "" || a != b {
		t.Fatalf("invalid JSON should produce stable non-empty hash; %s vs %s", a, b)
	}
}
