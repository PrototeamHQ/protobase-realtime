package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

type problem struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) // the client is gone when this fails
}

func refuse(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, problem{Error: code, Message: message})
}

// decode reads a JSON body of at most limit bytes, refusing unknown fields.
func decode(w http.ResponseWriter, r *http.Request, limit int, into any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(limit)))
	dec.DisallowUnknownFields()
	err := dec.Decode(into)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		refuse(w, http.StatusRequestEntityTooLarge, "too_large", "the body is over the app's request_bytes limit")
		return false
	}
	if err != nil {
		refuse(w, http.StatusBadRequest, "invalid_json", "the body is not the expected JSON object")
		return false
	}
	return true
}

// sockets reports how many open sockets a command reached.
type sockets struct {
	Sockets   int  `json:"sockets"`
	Duplicate bool `json:"duplicate,omitempty"`
}
