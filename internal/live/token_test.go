package live

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// sealed holds tokens the app's issuer sealed with the JavaScript jose library and WebCrypto's HKDF, as it does for a
// read: a person's and a visitor's, issued by "chat" at 1795190000 and expiring at 1795193600.
type sealed struct {
	TicketKey string `json:"ticket_key"`
	Key       string `json:"key"`
	Person    string `json:"person"`
	Visitor   string `json:"visitor"`
}

func readSealed(t *testing.T) sealed {
	t.Helper()
	raw, err := os.ReadFile("testdata/sealed.json")
	if err != nil {
		t.Fatal(err)
	}
	var s sealed
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

var (
	issuedAt  = time.Unix(1795190000, 0)
	expiresAt = time.Unix(1795193600, 0)
	person    = Socket{App: "chat", User: "u1", Session: "s1"}
	visitor   = Socket{App: "chat", Anonymous: true, Session: "anon-1"}
	otherKey  = "ticket-key-two-0123456789abcdefghij"
)

func TestKeyMatchesTheIssuersDerivation(t *testing.T) {
	s := readSealed(t)
	if got := hex.EncodeToString(Key(s.TicketKey)); got != s.Key {
		t.Fatalf("key %s, want %s", got, s.Key)
	}
}

func TestOpensTheIssuersTokens(t *testing.T) {
	s := readSealed(t)
	if err := Open(s.Person, []string{s.TicketKey}, person, issuedAt); err != nil {
		t.Errorf("person: %v", err)
	}
	if err := Open(s.Visitor, []string{s.TicketKey}, visitor, issuedAt); err != nil {
		t.Errorf("visitor: %v", err)
	}
	if err := Open(s.Person, []string{otherKey, s.TicketKey}, person, issuedAt); err != nil {
		t.Errorf("person under the second key: %v", err)
	}
}

func TestRefusesTokensNotForTheSocket(t *testing.T) {
	s := readSealed(t)
	keys := []string{s.TicketKey}
	cases := []struct {
		name   string
		token  string
		keys   []string
		socket Socket
		now    time.Time
	}{
		{"another key", s.Person, []string{otherKey}, person, issuedAt},
		{"another app", s.Person, keys, Socket{App: "shop", User: "u1", Session: "s1"}, issuedAt},
		{"another person", s.Person, keys, Socket{App: "chat", User: "u2", Session: "s1"}, issuedAt},
		{"another session", s.Person, keys, Socket{App: "chat", User: "u1", Session: "s2"}, issuedAt},
		{"a person's token on an anonymous socket", s.Person, keys, visitor, issuedAt},
		{"a visitor's token on a person's socket", s.Visitor, keys, person, issuedAt},
		{"at its expiry", s.Person, keys, person, expiresAt},
		{"after its expiry", s.Person, keys, person, expiresAt.Add(time.Second)},
		{"not a token", "not-a-token", keys, person, issuedAt},
	}
	for _, c := range cases {
		if err := Open(c.token, c.keys, c.socket, c.now); err != ErrInvalid {
			t.Errorf("%s: %v, want ErrInvalid", c.name, err)
		}
	}
}

// seal makes a token with claims of its own under key, for shapes the issuer never sends.
func seal(t *testing.T, key []byte, enc jose.ContentEncryption, claims map[string]any) string {
	t.Helper()
	encrypter, err := jose.NewEncrypter(enc, jose.Recipient{Algorithm: jose.DIRECT, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Encrypted(encrypter).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestRefusesMalformedClaims(t *testing.T) {
	key := Key(otherKey)
	exp := expiresAt.Unix()
	whole := seal(t, key, jose.A256GCM, map[string]any{"iss": "chat", "sub": "u1", "sid": "s1", "exp": exp})
	if err := Open(whole, []string{otherKey}, person, issuedAt); err != nil {
		t.Fatalf("a whole token: %v", err)
	}
	cases := []struct {
		name   string
		claims map[string]any
		socket Socket
	}{
		{"no expiry", map[string]any{"iss": "chat", "sub": "u1", "sid": "s1"}, person},
		{"no session", map[string]any{"iss": "chat", "sub": "u1", "exp": exp}, Socket{App: "chat", User: "u1"}},
		{"no person", map[string]any{"iss": "chat", "sid": "s1", "exp": exp}, Socket{App: "chat", Session: "s1"}},
		{"anonymous with a person", map[string]any{"iss": "chat", "sub": "u1", "anonymous": true, "exp": exp}, visitor},
		{"anonymous with a session", map[string]any{"iss": "chat", "sid": "anon-1", "anonymous": true, "exp": exp}, visitor},
	}
	for _, c := range cases {
		token := seal(t, key, jose.A256GCM, c.claims)
		if err := Open(token, []string{otherKey}, c.socket, issuedAt); err != ErrInvalid {
			t.Errorf("%s: %v, want ErrInvalid", c.name, err)
		}
	}
}

func TestRefusesAnotherContentEncryption(t *testing.T) {
	key := Key(otherKey)[:16]
	token := seal(t, key, jose.A128GCM, map[string]any{"iss": "chat", "sub": "u1", "sid": "s1", "exp": expiresAt.Unix()})
	if err := Open(token, []string{otherKey}, person, issuedAt); err != ErrInvalid {
		t.Fatalf("%v, want ErrInvalid", err)
	}
}
