package middleware

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"regexp"
)

// HeaderRequestID is the correlation header propagated through all services.
const HeaderRequestID = "X-Request-Id"

// validRequestID accepts only safe characters (same rule as the Java services), which prevents
// log and header injection through a client-supplied value.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// RequestID reuses a valid incoming X-Request-Id or generates a new one, stores it in the request
// context (for logs and downstream calls) and echoes it in the response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), id)))
	})
}

// newRequestID returns a random UUID (version 4).
func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never fails since Go 1.24
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
