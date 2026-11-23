package hub

// Conn is one socket. User is empty for an anonymous session. ID is minted by the relay for the socket, so it
// identifies the sender to others without exposing the sign-in session.
type Conn struct {
	ID        string
	User      string
	Anonymous bool
	Session   string
	sink      Sink

	// Guarded by the app's mutex.
	allowed set[string]
	joined  map[string]*joined
	// away is set while the client says it is suspended.
	away bool
}

type joined struct {
	ref *string
	// interest is nil while the socket has not said what it views: then nothing is filtered.
	interest set[string]
}

// sees reports whether a message about a key reaches this joined socket. An empty key reaches everyone.
func (j *joined) sees(about string) bool {
	if about == "" || j.interest == nil {
		return true
	}
	_, ok := j.interest[about]
	return ok
}
