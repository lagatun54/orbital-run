package api

import "net/http"

func allowRPCFrontend(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		allowed := origin != "" && r.Header.Get("Origin") == origin
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "Grpc-Status, Grpc-Message, Grpc-Status-Details-Bin")
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method == http.MethodOptions {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			if !allowed || r.Header.Get("Access-Control-Request-Method") != http.MethodPost {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", http.MethodPost)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Grpc-Web, X-User-Agent, Grpc-Timeout")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
