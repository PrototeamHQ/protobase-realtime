package hub

import "testing"

func TestJoinOnlyListedChannels(t *testing.T) {
	a := testApp("a", false)
	c, _ := connect(t, a, "u1", "s1", "room")
	if err := a.Join(c, "other", nil, nil); err != ErrNotMember {
		t.Fatalf("got %v", err)
	}
	join(t, a, c, "room")
	if !a.IsJoined(c, "room") {
		t.Fatal("not joined")
	}
}

func TestAddLetsOpenSocketsJoin(t *testing.T) {
	a := testApp("a", false)
	c, _ := connect(t, a, "u1", "s1")
	anon, _ := connect(t, a, "", "anon-1")
	if n := a.AddToChannel("room", Target{Users: []string{"u1", "nobody"}, Sessions: []string{"anon-1"}}); n != 2 {
		t.Fatalf("reached %d", n)
	}
	a.AddToChannel("room", Target{Users: []string{"u1"}})
	join(t, a, c, "room")
	join(t, a, anon, "room")
}

func TestRemoveClosesTheJoinedChannel(t *testing.T) {
	a := testApp("a", false)
	c, sink := connect(t, a, "u1", "s1", "room")
	ref := "7"
	if err := a.Join(c, "room", &ref, nil); err != nil {
		t.Fatal(err)
	}
	a.RemoveFromChannel("room", Target{Users: []string{"u1"}})
	a.RemoveFromChannel("room", Target{Users: []string{"u1"}})
	if len(sink.frames) != 1 || sink.frames[0] != `["7","7","room","phx_close",{}]` {
		t.Fatalf("got %v", sink.frames)
	}
	a.Publish(Publish{Channel: "room", Event: "new"})
	if len(sink.frames) != 1 {
		t.Fatal("removed socket still receives")
	}
	if err := a.Join(c, "room", &ref, nil); err != ErrNotMember {
		t.Fatalf("rejoin got %v", err)
	}
}

func TestValidInterest(t *testing.T) {
	long := make([]byte, MaxInterestKeyLen+1)
	for i := range long {
		long[i] = 'x'
	}
	if !ValidInterest([]string{"a"}) || ValidInterest([]string{""}) || ValidInterest([]string{string(long)}) {
		t.Fatal("wrong verdict")
	}
	if ValidInterest(make([]string, MaxInterestKeys+1)) {
		t.Fatal("too many keys accepted")
	}
}
