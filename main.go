package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dealhouse/http-server/internal/database"
	// "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/dealhouse/http-server/internal/auth"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	jwtSecret      string
	polkaKey       string
}

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
func handleValidateChirp(body string) (string, error) {

	if len(body) > 140 {
		return "", fmt.Errorf("chirp is too long")
	}
	bodyStr := strings.Split(body, " ")
	words := []string{"kerfuffle", "sharbert", "fornax"}
	for _, word := range bodyStr {
		for _, badWord := range words {
			if badWord == strings.ToLower(word) {
				body = strings.ReplaceAll(body, word, "****")
			}
		}
	}
	return body, nil

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
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		eresp := fmt.Sprintf("Error decoding parameters: %s", err)
		respondWithError(w, 500, eresp)
		return
	}

	hash, err := auth.HashPassword(params.Password)
	if err != nil {
		eresp := fmt.Sprintf("Error hashing password: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	params.Password = hash
	user, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{
		HashedPassword: params.Password,
		Email:          params.Email,
	})
	if err != nil {
		eresp := fmt.Sprintf("Error creating user: %s", err)
		respondWithError(w, 500, eresp)
	}

	type respBody struct {
		Id          string `json:"id"`
		Created_at  string `json:"created_at"`
		Updated_at  string `json:"updated_at"`
		Email       string `json:"email"`
		IsChirpyRed bool   `json:"is_chirpy_red"`
	}

	resp := respBody{
		Id:          user.ID.String(),
		Created_at:  user.CreatedAt.String(),
		Updated_at:  user.UpdatedAt.String(),
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed,
	}
	respondWithJSON(w, 201, resp)

}
func (cfg *apiConfig) handleCreateChirp(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		eresp := fmt.Sprintf("Error getting bearer token: %s", err)
		respondWithError(w, 401, eresp)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		eresp := fmt.Sprintf("Error validating token: %s", err)
		respondWithError(w, 401, eresp)
		return
	}

	type parameters struct {
		Body string `json:"body"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		eresp := fmt.Sprintf("Error decoding parameters: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	cleaned, err := handleValidateChirp(params.Body)
	if err != nil {
		eresp := fmt.Sprintf("Error validating chirp: %s", err)
		respondWithError(w, 400, eresp)
		return
	}
	params.Body = cleaned

	chirp, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   params.Body,
		UserID: userID,
	})
	if err != nil {
		eresp := fmt.Sprintf("Error creating chirp: %s", err)
		respondWithError(w, 500, eresp)
		return
	}

	type respBody struct {
		Id         string `json:"id"`
		Created_at string `json:"created_at"`
		Updated_at string `json:"updated_at"`
		Body       string `json:"body"`
		User_id    string `json:"user_id"`
	}
	resp := respBody{
		Id:         chirp.ID.String(),
		Created_at: chirp.CreatedAt.String(),
		Updated_at: chirp.UpdatedAt.String(),
		Body:       chirp.Body,
		User_id:    chirp.UserID.String(),
	}
	respondWithJSON(w, 201, resp)
}

func (cfg *apiConfig) handleGetChirps(w http.ResponseWriter, r *http.Request) {
	var data []database.Chirp
	var err error
	author, err := uuid.Parse(r.URL.Query().Get("author_id"))
	if err != nil {
		author = uuid.Nil
	}
	if author != uuid.Nil {
		data, err = cfg.db.GetChirpsByUserID(r.Context(), author)
		if err != nil {
			eresp := fmt.Sprintf("Error getting chirps: %s", err)
			respondWithError(w, 500, eresp)
			return
		}
	} else {
		data, err = cfg.db.GetChirps(r.Context())
		if err != nil {
			eresp := fmt.Sprintf("Error getting chirps: %s", err)
			respondWithError(w, 500, eresp)
			return
		}
	}
	sorting := r.URL.Query().Get("sort")
	if sorting == "desc" {
		sort.Slice(data, func(i, j int) bool {
			return data[i].CreatedAt.After(data[j].CreatedAt)
		})
	}

	type chirpBody struct {
		Id         string `json:"id"`
		Created_at string `json:"created_at"`
		Updated_at string `json:"updated_at"`
		Body       string `json:"body"`
		User_id    string `json:"user_id"`
	}
	items := []chirpBody{}
	for _, chirp := range data {
		currentChirp := chirpBody{
			Id:         chirp.ID.String(),
			Created_at: chirp.CreatedAt.String(),
			Updated_at: chirp.UpdatedAt.String(),
			Body:       chirp.Body,
			User_id:    chirp.UserID.String(),
		}
		items = append(items, currentChirp)
	}
	respondWithJSON(w, 200, items)

}

func (cfg *apiConfig) handleGetChirpByID(w http.ResponseWriter, r *http.Request) {
	urlID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		eresp := fmt.Sprintf("Error parsing chirp id: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	data, err := cfg.db.GetChirpByID(r.Context(), urlID)
	if err == sql.ErrNoRows {
		eresp := fmt.Sprintf("Chirp with id %s not found", urlID)
		respondWithError(w, 404, eresp)
		return
	}
	if err != nil {
		eresp := fmt.Sprintf("Error getting chirp: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	type chirpBody struct {
		Id         string `json:"id"`
		Created_at string `json:"created_at"`
		Updated_at string `json:"updated_at"`
		Body       string `json:"body"`
		User_id    string `json:"user_id"`
	}

	currentChirp := chirpBody{
		Id:         data.ID.String(),
		Created_at: data.CreatedAt.String(),
		Updated_at: data.UpdatedAt.String(),
		Body:       data.Body,
		User_id:    data.UserID.String(),
	}
	respondWithJSON(w, 200, currentChirp)
}

func (cfg *apiConfig) handleLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		eresp := fmt.Sprintf("Error decoding parameters: %s", err)
		respondWithError(w, 500, eresp)
		return
	}

	user, err := cfg.db.GetUserByEmail(r.Context(), params.Email)
	if err == sql.ErrNoRows {
		eresp := fmt.Sprintf("User with email %s not found", params.Email)
		respondWithError(w, 404, eresp)
		return
	}
	if err != nil {
		eresp := fmt.Sprintf("Error getting user: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	match, err := auth.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil {
		eresp := fmt.Sprintf("Error checking password: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	if !match {
		eresp := "Incorrect password"
		respondWithError(w, 401, eresp)
		return
	}

	accessToken, err := auth.MakeJWT(user.ID, cfg.jwtSecret, time.Hour)
	if err != nil {
		eresp := fmt.Sprintf("Error making JWT: %s", err)
		respondWithError(w, 500, eresp)
		return
	}

	refreshToken := auth.MakeRefreshToken()
	expiresAt := time.Now().Add(time.Hour * 24 * 60)
	err = cfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		eresp := fmt.Sprintf("Error creating refresh token: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	type respBody struct {
		Id            string `json:"id"`
		Created_at    string `json:"created_at"`
		Updated_at    string `json:"updated_at"`
		Email         string `json:"email"`
		Token         string `json:"token"`
		Refresh_token string `json:"refresh_token"`
		IsChirpyRed   bool   `json:"is_chirpy_red"`
	}

	resp := respBody{
		Id:            user.ID.String(),
		Created_at:    user.CreatedAt.String(),
		Updated_at:    user.UpdatedAt.String(),
		Email:         user.Email,
		Token:         accessToken,
		Refresh_token: refreshToken,
		IsChirpyRed:   user.IsChirpyRed,
	}
	respondWithJSON(w, 200, resp)

}

func (cfg *apiConfig) handleRefresh(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		eresp := fmt.Sprintf("Invalid refresh token: %s", err)
		respondWithError(w, 401, eresp)
		return
	}

	userID, err := cfg.db.GetUserFromRefreshToken(r.Context(), token)
	if err != nil {
		eresp := fmt.Sprintf("Error finding user: %s", err)
		respondWithError(w, 401, eresp)
		return
	}

	accessToken, err := auth.MakeJWT(userID, cfg.jwtSecret, time.Hour)
	if err != nil {
		eresp := fmt.Sprintf("Error creating access token %s", err)
		respondWithError(w, 500, eresp)
		return
	}

	type respBody struct {
		Token string `json:"token"`
	}

	resp := respBody{
		Token: accessToken,
	}

	respondWithJSON(w, 200, resp)
}

func (cfg *apiConfig) handleRevoke(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		eresp := fmt.Sprintf("Invalid refresh token: %s", err)
		respondWithError(w, 401, eresp)
		return
	}
	err = cfg.db.RevokeRefreshToken(r.Context(), token)
	if err != nil {
		eresp := fmt.Sprintf("Error revoking token: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handleUpdateEmailAndPassword(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		eresp := fmt.Sprintf("Invalid access token: %s", err)
		respondWithError(w, 401, eresp)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		eresp := fmt.Sprintf("Error validating token: %s", err)
		respondWithError(w, 401, eresp)
		return
	}
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		eresp := fmt.Sprintf("Error decoding parameters: %s", err)
		respondWithError(w, 500, eresp)
		return
	}

	newHashPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		eresp := fmt.Sprintf("Error hashing password: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	user, err := cfg.db.UpdateEmailAndPassword(r.Context(), database.UpdateEmailAndPasswordParams{
		Email:          params.Email,
		HashedPassword: newHashPassword,
		ID:             userID,
	})
	if err != nil {
		eresp := fmt.Sprintf("Error updating email and/or password: %s", err)
		respondWithError(w, 500, eresp)
		return
	}

	type respBody struct {
		Id          string `json:"id"`
		Created_at  string `json:"created_at"`
		Updated_at  string `json:"updated_at"`
		Email       string `json:"email"`
		IsChirpyRed bool   `json:"is_chirpy_red"`
	}
	resp := respBody{
		Id:          user.ID.String(),
		Created_at:  user.CreatedAt.String(),
		Updated_at:  user.UpdatedAt.String(),
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed,
	}
	respondWithJSON(w, 200, resp)
}

func (cfg *apiConfig) handleDeleteChirp(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		eresp := fmt.Sprintf("Error parsing chirp id: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		eresp := fmt.Sprintf("Invalid access token: %s", err)
		respondWithError(w, 401, eresp)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		eresp := fmt.Sprintf("Error validating token: %s", err)
		respondWithError(w, 401, eresp)
		return
	}
	chirp, err := cfg.db.GetChirpByID(r.Context(), chirpID)
	if err == sql.ErrNoRows {
		eresp := fmt.Sprintf("Chirp with id %s not found", chirpID)
		respondWithError(w, 404, eresp)
		return
	}

	if err != nil {
		eresp := fmt.Sprintf("Error getting chirp: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	if chirp.UserID != userID {
		eresp := "Unauthorized"
		respondWithError(w, 403, eresp)
		return
	}
	err = cfg.db.DeleteChirp(r.Context(), chirpID)
	if err != nil {
		eresp := fmt.Sprintf("Error deleting chirp: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handleSubscriptionUpgrade(w http.ResponseWriter, r *http.Request) {
	key, err := auth.GetAPIKey(r.Header)
	if err != nil {
		eresp := fmt.Sprintf("Error getting API key: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	loggingKey := fmt.Sprintf("API key: %s", key)
	loggingConfig := fmt.Sprintf("Polka key: %s", cfg.polkaKey)
	fmt.Println(loggingKey, loggingConfig)
	if key != cfg.polkaKey {
		eresp := "Unauthorized"
		respondWithError(w, 401, eresp)
		return
	}
	type parameters struct {
		Event string `json:"event"`
		Data  struct {
			UserId uuid.UUID `json:"user_id"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		eresp := fmt.Sprintf("Error decoding parameters: %s", err)
		respondWithError(w, 500, eresp)
		return
	}
	if params.Event != "user.upgraded" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	err = cfg.db.UpgradeRedSubscription(r.Context(), params.Data.UserId)
	if err != nil {
		eresp := fmt.Sprintf("User not found: %s", err)
		respondWithError(w, 404, eresp)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	apiCfg.jwtSecret = os.Getenv("JWT_SECRET")
	apiCfg.polkaKey = os.Getenv("POLKA_KEY")
	sm.Handle("/app/", apiCfg.middlewareMetrics(fileHandler))

	sm.HandleFunc("GET /api/healthz", handlerReadiness)

	sm.HandleFunc("GET /api/chirps", apiCfg.handleGetChirps)
	sm.HandleFunc("GET /api/chirps/{id}", apiCfg.handleGetChirpByID)
	sm.HandleFunc("POST /api/chirps", apiCfg.handleCreateChirp)
	sm.HandleFunc("DELETE /api/chirps/{id}", apiCfg.handleDeleteChirp)

	sm.HandleFunc("POST /api/users", apiCfg.handleCreateUser)
	sm.HandleFunc("PUT /api/users", apiCfg.handleUpdateEmailAndPassword)

	sm.HandleFunc("POST /api/login", apiCfg.handleLogin)

	sm.HandleFunc("POST /api/refresh", apiCfg.handleRefresh)
	sm.HandleFunc("POST /api/revoke", apiCfg.handleRevoke)

	sm.HandleFunc("POST /api/polka/webhooks", apiCfg.handleSubscriptionUpgrade)

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
