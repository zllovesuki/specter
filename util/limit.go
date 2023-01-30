package util

import "net/http"

func LimitBody(size int64) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, size)
			h.ServeHTTP(w, r)
		})
	}
}
