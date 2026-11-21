package hub

import (
	"fmt"
	"testing"
)

type presenceLog []string

func (p *presenceLog) Change(user string, online, away int) {
	*p = append(*p, fmt.Sprintf("%s %d/%d", user, online, away))
}

func TestPresenceHearsEachPersonsSocketCounts(t *testing.T) {
	log := &presenceLog{}
	a := NewApp(*testApp("a", nil).Config(), log)
	phone, _ := connect(t, a, "u1", "s1")
	connect(t, a, "u1", "s2")
	connect(t, a, "", "anon")
	a.SetAway(phone, true)
	a.Unregister(phone)
	a.SetAway(phone, false)
	want := "[u1 1/0 u1 2/0 u1 1/1 u1 1/0]"
	if fmt.Sprint(*log) != want {
		t.Fatalf("got %v, want %s", *log, want)
	}
}
