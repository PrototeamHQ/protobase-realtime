package ticket

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	keyA = "app-a-ticket-key-0123456789abcdef"
	keyB = "app-b-ticket-key-0123456789abcdef"
)

var now = time.Unix(1_800_000_000, 0)

func keys(app string) ([]string, bool) {
	switch app {
	case "a":
		return []string{"old-key-for-a-0123456789abcdefgh", keyA}, true
	case "b":
		return []string{keyB}, true
	}
	return nil, false
}

func sign(t *testing.T, key string, c jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func base(jti string) jwt.MapClaims {
	return jwt.MapClaims{"iss": "a", "sub": "u1", "sid": "s1", "jti": jti, "exp": now.Add(time.Minute).Unix(),
		"channels": []string{"room:1"}}
}

func verifier() *Verifier { return NewVerifier(keys, func() time.Time { return now }) }

func TestVerifyAcceptsAValidTicket(t *testing.T) {
	got, err := verifier().Verify(sign(t, keyA, base("t1")))
	if err != nil {
		t.Fatal(err)
	}
	if got.App != "a" || got.User != "u1" || got.Session != "s1" || got.Anonymous || len(got.Channels) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestVerifyReadsTheRules(t *testing.T) {
	c := base("t1")
	c["blocked"] = []string{"u2"}
	c["friends_only"] = true
	c["friends"] = []string{"u3", "u4"}
	got, err := verifier().Verify(sign(t, keyA, c))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules.Blocked) != 1 || !got.Rules.FriendsOnly || len(got.Rules.Friends) != 2 {
		t.Fatalf("got %+v", got.Rules)
	}
}

func TestVerifyAcceptsAnAnonymousTicket(t *testing.T) {
	c := base("t1")
	delete(c, "sub")
	c["anonymous"] = true
	got, err := verifier().Verify(sign(t, keyA, c))
	if err != nil || !got.Anonymous || got.User != "" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestVerifyRefusesASecondUse(t *testing.T) {
	v := verifier()
	raw := sign(t, keyA, base("t1"))
	if _, err := v.Verify(raw); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(raw); err == nil {
		t.Fatal("second use accepted")
	}
}

func TestVerifyKeepsTicketIdsPerApp(t *testing.T) {
	v := verifier()
	other := base("t1")
	other["iss"] = "b"
	if _, err := v.Verify(sign(t, keyA, base("t1"))); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(sign(t, keyB, other)); err != nil {
		t.Fatalf("same id in another app refused: %v", err)
	}
}

func TestVerifyRefuses(t *testing.T) {
	cases := map[string]func(jwt.MapClaims) (string, jwt.MapClaims){
		"another app's key": func(c jwt.MapClaims) (string, jwt.MapClaims) { return keyB, c },
		"unknown app":       func(c jwt.MapClaims) (string, jwt.MapClaims) { c["iss"] = "z"; return keyA, c },
		"expired": func(c jwt.MapClaims) (string, jwt.MapClaims) {
			c["exp"] = now.Add(-time.Second).Unix()
			return keyA, c
		},
		"no expiry":      func(c jwt.MapClaims) (string, jwt.MapClaims) { delete(c, "exp"); return keyA, c },
		"too long-lived": func(c jwt.MapClaims) (string, jwt.MapClaims) { c["exp"] = now.Add(time.Hour).Unix(); return keyA, c },
		"no session":     func(c jwt.MapClaims) (string, jwt.MapClaims) { delete(c, "sid"); return keyA, c },
		"no ticket id":   func(c jwt.MapClaims) (string, jwt.MapClaims) { delete(c, "jti"); return keyA, c },
		"no person":      func(c jwt.MapClaims) (string, jwt.MapClaims) { delete(c, "sub"); return keyA, c },
		"person and anonymous": func(c jwt.MapClaims) (string, jwt.MapClaims) {
			c["anonymous"] = true
			return keyA, c
		},
		"anonymous with rules": func(c jwt.MapClaims) (string, jwt.MapClaims) {
			delete(c, "sub")
			c["anonymous"] = true
			c["blocked"] = []string{"u2"}
			return keyA, c
		},
		"too many blocked": func(c jwt.MapClaims) (string, jwt.MapClaims) {
			c["blocked"] = make([]string, MaxRuleList+1)
			return keyA, c
		},
	}
	for name, change := range cases {
		key, c := change(base("t-" + name))
		if _, err := verifier().Verify(sign(t, key, c)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestVerifyRefusesAnotherAlgorithm(t *testing.T) {
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS512, base("t1")).SignedString([]byte(keyA))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier().Verify(raw); err == nil {
		t.Fatal("HS512 accepted")
	}
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, base("t2")).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := verifier().Verify(none); err == nil {
		t.Fatal("alg none accepted")
	}
}
