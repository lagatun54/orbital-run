package api

import "net/http"

// Allow the configured frontend to read API responses.
func allowFrontend(origin string, next http.Handler) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Add("Vary", "Origin")
		next.ServeHTTP(w, r)
	})
}

func localResultsPreflight(origin string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if origin == "" || r.Header.Get("Origin") != origin ||
			r.Header.Get("Access-Control-Request-Method") != http.MethodPost {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", http.MethodPost)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Add("Vary", "Origin")
		w.WriteHeader(http.StatusNoContent)
	}
}
