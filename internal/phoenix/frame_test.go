package phoenix

import (
	"encoding/json"
	"testing"
)

func TestDecodeReadsAJoin(t *testing.T) {
	f, err := Decode([]byte(`["1","1","room:1","phx_join",{"viewing":["a"]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if *f.JoinRef != "1" || *f.Ref != "1" || f.Topic != "room:1" || f.Event != Join || string(f.Payload) != `{"viewing":["a"]}` {
		t.Fatalf("got %+v", f)
	}
}

func TestDecodeReadsNullRefs(t *testing.T) {
	f, err := Decode([]byte(`[null,"3","phoenix","heartbeat",{}]`))
	if err != nil || f.JoinRef != nil || *f.Ref != "3" {
		t.Fatalf("got %+v, %v", f, err)
	}
}

func TestDecodeRefusesMalformedFrames(t *testing.T) {
	for _, raw := range []string{
		`{"topic":"a"}`, `[null,null,"a","b"]`, `[null,null,"","b",{}]`, `[null,null,"a","b",[1]]`,
		`[null,null,"a","b",null]`, `[1,null,"a","b",{}]`, `not json`,
	} {
		if _, err := Decode([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", raw)
		}
	}
}

func TestReplyEchoesRefs(t *testing.T) {
	joinRef, ref := "1", "2"
	got := Refuse(Frame{JoinRef: &joinRef, Ref: &ref, Topic: "room:1", Event: Join}, "unauthorized")
	want := `["1","2","room:1","phx_reply",{"response":{"reason":"unauthorized"},"status":"error"}]`
	if string(got) != want {
		t.Fatalf("got %s", got)
	}
}

func TestEncodeSendsEmptyPayloadAsObject(t *testing.T) {
	got := Encode(Frame{Topic: "t", Event: "e"})
	var parts []json.RawMessage
	if err := json.Unmarshal(got, &parts); err != nil || string(parts[4]) != `{}` || string(parts[0]) != "null" {
		t.Fatalf("got %s", got)
	}
}
