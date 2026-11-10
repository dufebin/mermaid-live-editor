package main

import (
	"log"
	"net/http"
	"os"
)

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	host, port, dataDir := envOr("HOST", "127.0.0.1"), envOr("PORT", "18790"), envOr("DATA_DIR", "server-data")
	s, err := openDB(dataDir)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	log.Printf("storage API listening on http://%s:%s", host, port)
	log.Fatal(http.ListenAndServe(host+":"+port, recoverWrap(newHandler(s))))
}

// recoverWrap mirrors node's try/catch -> 400 with the error message.
func recoverWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if err, ok := rec.(error); ok {
					writeError(w, 400, err.Error())
					return
				}
				writeError(w, 400, "Bad request")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
