package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) hitsWriter(w http.ResponseWriter, r *http.Request) {
	hitRes := cfg.fileserverHits.Load()
	hitString := fmt.Sprintf("Hits: %d", hitRes)
	w.Write([]byte(hitString))
}

func (cfg *apiConfig) hitsResetter(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
}
func main() {
	sm := http.NewServeMux()
	fileHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	apiCfg := apiConfig{}
	apiCfg.fileserverHits.Store(0)
	sm.Handle("/app/", apiCfg.middlewareMetrics(fileHandler))
	hFunc := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}
	sm.HandleFunc("/healthz", hFunc)
	server := &http.Server{
		Addr:    ":8080",
		Handler: sm,
	}

	sm.HandleFunc("/metrics", apiCfg.hitsWriter)
	sm.HandleFunc("/reset", apiCfg.hitsResetter)
	// refactored from
	/*
		err := server.ListenAndServe()
		if err != nil {
			fmt.Println("listen and serve error")
			return
		} else {
		f	mt.Println("Listening")
		}
	*/
	log.Printf("Listening on %s...", server.Addr)
	log.Fatal(server.ListenAndServe())
}
