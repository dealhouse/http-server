package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/dealhouse/http-server/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorResponse struct {
		Error string `json:"error"`
	}
	respondWithJSON(w, code, errorResponse{Error: msg})
}
func handleValidateChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		eresp := fmt.Sprintf("Error decoding parameters: %s", err)
		respondWithError(w, 500, eresp)
	}
	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}
	bodyStr := strings.Split(params.Body, " ")
	words := []string{"kerfuffle", "sharbert", "fornax"}
	for _, word := range bodyStr {
		for _, badWord := range words {
			if badWord == strings.ToLower(word) {
				params.Body = strings.ReplaceAll(params.Body, word, "****")
			}
		}
	}
	type cleaned struct {
		Cleaned_body string `json:"cleaned_body"`
	}

	respondWithJSON(w, 200, cleaned{Cleaned_body: params.Body})

}

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
}

func (cfg *apiConfig) middlewareMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) hitsWriter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	hitRes := cfg.fileserverHits.Load()
	hitString := fmt.Sprintf(
		`<html>
	<body>
		<h1>Welcome, Chirpy Admin</h1>
		<p>Chirpy has been visited %d times!</p>
	</body>
	</html>`,
		hitRes)
	w.Write([]byte(hitString))
}

func (cfg *apiConfig) hitsResetter(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		respondWithError(w, 403, "Forbidden")
		return
	}
	cfg.fileserverHits.Store(0)
	cfg.db.DeleteUsers(r.Context())
}

func (cfg *apiConfig) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		eresp := fmt.Sprintf("Error decoding parameters: %s", err)
		respondWithError(w, 500, eresp)
		return
	}

	user, err := cfg.db.CreateUser(r.Context(), params.Email)
	if err != nil {
		eresp := fmt.Sprintf("Error creating user: %s", err)
		respondWithError(w, 500, eresp)
	}

	type respBody struct {
		Id         string `json:"id"`
		Created_at string `json:"created_at"`
		Updated_at string `json:"updated_at"`
		Email      string `json:"email"`
	}

	resp := respBody{
		Id:         user.ID.String(),
		Created_at: user.CreatedAt.String(),
		Updated_at: user.UpdatedAt.String(),
		Email:      user.Email,
	}
	respondWithJSON(w, 201, resp)

}
func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)
	sm := http.NewServeMux()
	server := &http.Server{
		Addr:    ":8080",
		Handler: sm,
	}

	fileHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	apiCfg := apiConfig{}
	apiCfg.fileserverHits.Store(0)
	apiCfg.db = dbQueries
	apiCfg.platform = os.Getenv("PLATFORM")
	sm.Handle("/app/", apiCfg.middlewareMetrics(fileHandler))

	sm.HandleFunc("GET /api/healthz", handlerReadiness)

	sm.HandleFunc("POST /api/validate_chirp", handleValidateChirp)

	sm.HandleFunc("POST /api/users", apiCfg.handleCreateUser)

	sm.HandleFunc("GET /admin/metrics", apiCfg.hitsWriter)
	sm.HandleFunc("POST /admin/reset", apiCfg.hitsResetter)

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
