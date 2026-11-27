package socket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// sendTo posts a body to a connection of the handler with the secret, and returns the status.
func sendTo(h *Handler, id, secret, body string) int {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+SendPattern, h.ServeSend)
	r := httptest.NewRequest(http.MethodPost, SendPath(id), strings.NewReader(body))
	r.Header.Set(SecretHeader, secret)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w.Code
}

func openStream(t *testing.T) (*Handler, *stream) {
	t.Helper()
	h := &Handler{}
	return h, h.streams.open(newSession(t, "u1", "room"))
}

func TestSendHandsFramesToTheSessionInOrder(t *testing.T) {
	h, st := openStream(t)
	if code := sendTo(h, st.id, st.secret, `[null,"1","phoenix","heartbeat",{}]`); code != http.StatusNoContent {
		t.Fatalf("one frame: %d", code)
	}
	batch := `[["2","2","room","phx_join",{}],["2","3","room","phx_leave",{}]]`
	if code := sendTo(h, st.id, st.secret, batch); code != http.StatusNoContent {
		t.Fatalf("batch: %d", code)
	}
	for _, ref := range []string{`"1"`, `"2"`, `"3"`} {
		reply := string(<-st.session.out.queue)
		if !strings.Contains(reply, ","+ref+",") || !strings.Contains(reply, `"status":"ok"`) {
			t.Fatalf("reply to %s: %s", ref, reply)
		}
	}
}

func TestSendRefusesAWrongSecretAndAnUnknownConnection(t *testing.T) {
	h, st := openStream(t)
	heartbeat := `[null,"1","phoenix","heartbeat",{}]`
	if code := sendTo(h, st.id, st.secret+"x", heartbeat); code != http.StatusNotFound {
		t.Fatalf("wrong secret: %d", code)
	}
	if code := sendTo(h, "unknown", st.secret, heartbeat); code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", code)
	}
	h.streams.forget(st)
	if code := sendTo(h, st.id, st.secret, heartbeat); code != http.StatusNotFound {
		t.Fatalf("forgotten id: %d", code)
	}
	if len(st.session.out.queue) != 0 {
		t.Fatal("a refused frame reached the session")
	}
}

func TestSendRefusesABodyOverTheReadLimit(t *testing.T) {
	h, st := openStream(t)
	payload := strings.Repeat("x", int(readLimit(st.session.app)))
	if code := sendTo(h, st.id, st.secret, `["1","1","room","typing",{"x":"`+payload+`"}]`); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d", code)
	}
}

func TestSendEndsTheConnectionOnAMalformedFrame(t *testing.T) {
	h, st := openStream(t)
	if code := sendTo(h, st.id, st.secret, `[["1","1","room"]]`); code != http.StatusBadRequest {
		t.Fatalf("got %d", code)
	}
	if st.session.out.code != websocket.StatusInvalidFramePayloadData {
		t.Fatalf("code %v", st.session.out.code)
	}
}

func TestDecodeFramesTellsAFrameFromABatch(t *testing.T) {
	one, err := decodeFrames([]byte(`[null,"1","phoenix","heartbeat",{}]`))
	if err != nil || len(one) != 1 {
		t.Fatalf("one: %v %v", one, err)
	}
	batch, err := decodeFrames([]byte(` [ [null,"1","phoenix","heartbeat",{}], [null,"2","phoenix","heartbeat",{}] ]`))
	if err != nil || len(batch) != 2 || *batch[1].Ref != "2" {
		t.Fatalf("batch: %v %v", batch, err)
	}
	for _, raw := range []string{`[]`, `{}`, `[[]]`, `"x"`} {
		if _, err := decodeFrames([]byte(raw)); err == nil {
			t.Fatalf("%s decoded", raw)
		}
	}
}
