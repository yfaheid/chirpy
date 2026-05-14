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
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/yfaheid/chirpy/internal/auth"
	"github.com/yfaheid/chirpy/internal/database"
)

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	secret := os.Getenv("SECRET")
	dbConn, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(dbConn)
	mux := http.NewServeMux()
	var apiCfg apiConfig
	apiCfg.db = dbQueries
	apiCfg.platform = platform
	apiCfg.secret = secret
	mux.Handle("/app/", http.StripPrefix("/app", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))
	mux.HandleFunc("GET /api/healthz", handlerReadiness)
	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	mux.HandleFunc("POST /api/chirps", apiCfg.handlerChirps)
	mux.HandleFunc("POST /api/users", apiCfg.handlerUsers)
	mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetChirp)
	mux.HandleFunc("POST /api/login", apiCfg.handlerLogin)
	mux.HandleFunc("POST /api/refresh", apiCfg.handlerRefresh)
	mux.HandleFunc("POST /api/revoke", apiCfg.handlerRevoke)
	mux.HandleFunc("PUT /api/users", apiCfg.handlerUsersUpdate)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.handlerChirpsDelete)
	server := http.Server{Handler: mux, Addr: ":8080"}
	server.ListenAndServe()
}

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	secret         string
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)

	count := fmt.Sprintf(`
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())
	w.Write([]byte(count))
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(403)
		return
	}
	cfg.fileserverHits.Store(0)
	err := cfg.db.DeleteUsers(r.Context())
	if err != nil {
		log.Printf("Error deleting users: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) handlerChirps(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token: %s", err)
		w.WriteHeader(500)
		return
	}
	id, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		log.Printf("Invalid token: %s", err)
		w.WriteHeader(401)
		return
	}

	type errorResponse struct {
		Error string `json:"error"`
	}

	errorRespBody := errorResponse{}
	if len(params.Body) > 140 {
		errorRespBody.Error = "Chirp is too long"
		dat, err := json.Marshal(errorRespBody)
		if err != nil {
			log.Printf("Error marshalling JSON: %s", err)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write(dat)
	} else {
		words := strings.Split(params.Body, " ")
		for i, word := range words {
			if strings.ToLower(word) == "kerfuffle" || strings.ToLower(word) == "sharbert" || strings.ToLower(word) == "fornax" {
				words[i] = "****"
			}
		}
		cleanedBody := strings.Join(words, " ")
		chirp, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{Body: cleanedBody, UserID: id})
		if err != nil {
			log.Printf("Error marshalling JSON: %s", err)
			w.WriteHeader(500)
			return
		}
		newChirp := Chirp{ID: chirp.ID, CreatedAt: chirp.CreatedAt, UpdatedAt: chirp.UpdatedAt, Body: chirp.Body, UserID: chirp.UserID}
		dat, err := json.Marshal(newChirp)
		if err != nil {
			log.Printf("Error marshalling JSON: %s", err)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write(dat)
	}
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
}

func (cfg *apiConfig) handlerUsers(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}
	hash, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		w.WriteHeader(500)
		return
	}
	user, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{Email: params.Email, HashedPassword: hash})
	if err != nil {
		log.Printf("Error creating user: %s", err)
		w.WriteHeader(500)
		return
	}
	newUser := User{ID: user.ID, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt, Email: user.Email}

	dat, err := json.Marshal(newUser)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(dat)
}

func (cfg *apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	chirps, err := cfg.db.GetChirps(r.Context())
	if err != nil {
		log.Printf("Error getting chirps: %s", err)
		w.WriteHeader(500)
		return
	}
	var allChirps []Chirp
	for _, chirp := range chirps {
		allChirps = append(allChirps, Chirp{ID: chirp.ID, CreatedAt: chirp.CreatedAt, UpdatedAt: chirp.UpdatedAt, Body: chirp.Body, UserID: chirp.UserID})
	}
	dat, err := json.Marshal(allChirps)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerGetChirp(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		log.Printf("Error parsing ID to UUID: %s", err)
		w.WriteHeader(500)
		return
	}
	chirp, err := cfg.db.GetChirp(r.Context(), id)
	if err != nil {
		log.Printf("Error getting chirp: %s", err)
		w.WriteHeader(404)
		return
	}
	newChirp := Chirp{ID: chirp.ID, CreatedAt: chirp.CreatedAt, UpdatedAt: chirp.UpdatedAt, Body: chirp.Body, UserID: chirp.UserID}
	dat, err := json.Marshal(newChirp)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}
	refreshToken := auth.MakeRefreshToken()

	user, err := cfg.db.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		log.Printf("Error getting user: %s", err)
		w.WriteHeader(401)
		return
	}
	dbRefreshToken, err := cfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{Token: refreshToken, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour * 24 * 60)})
	if err != nil {
		log.Printf("Error making refresh token: %s", err)
		w.WriteHeader(500)
		return
	}
	token, err := auth.MakeJWT(user.ID, cfg.secret, time.Hour)
	if err != nil {
		log.Printf("Error making token: %s", err)
		w.WriteHeader(500)
		return
	}
	check, err := auth.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil {
		log.Printf("Error getting user: %s", err)
		w.WriteHeader(401)
		return
	}
	if !check {
		log.Printf("Error getting user: %s", err)
		w.WriteHeader(401)
		return
	}
	newUser := User{ID: user.ID, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt, Email: user.Email, Token: token, RefreshToken: dbRefreshToken.Token}

	dat, err := json.Marshal(newUser)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	type JWT struct {
		Token string `json:"token"`
	}
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token: %s", err)
		w.WriteHeader(400)
		return
	}
	userID, err := cfg.db.GetUserFromRefreshToken(r.Context(), token)
	if err != nil {
		log.Printf("Error getting token: %s", err)
		w.WriteHeader(401)
		return
	}
	jwt, err := auth.MakeJWT(userID, cfg.secret, time.Hour)
	if err != nil {
		log.Printf("Error making JWT: %s", err)
		w.WriteHeader(500)
		return
	}
	newJWT := JWT{Token: jwt}
	dat, err := json.Marshal(newJWT)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token: %s", err)
		w.WriteHeader(400)
		return
	}
	err = cfg.db.RevokeRefreshToken(r.Context(), token)
	if err != nil {
		log.Printf("Error revoking token: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(204)
}

func (cfg *apiConfig) handlerUsersUpdate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token: %s", err)
		w.WriteHeader(401)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		log.Printf("Error validating token: %s", err)
		w.WriteHeader(401)
		return
	}
	hash, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		w.WriteHeader(500)
		return
	}
	user, err := cfg.db.UpdateUser(r.Context(), database.UpdateUserParams{Email: params.Email, HashedPassword: hash, ID: userID})
	if err != nil {
		log.Printf("Error updating user: %s", err)
		w.WriteHeader(500)
		return
	}
	resp := User{ID: user.ID, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt, Email: user.Email}
	dat, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerChirpsDelete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		log.Printf("Error parsing ID to UUID: %s", err)
		w.WriteHeader(500)
		return
	}
	chirp, err := cfg.db.GetChirp(r.Context(), id)
	if err != nil {
		log.Printf("Error getting chirp: %s", err)
		w.WriteHeader(404)
		return
	}
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token: %s", err)
		w.WriteHeader(401)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		log.Printf("Error validating token: %s", err)
		w.WriteHeader(401)
		return
	}
	if userID != chirp.UserID {
		log.Printf("Incorrect user ID: %s", err)
		w.WriteHeader(403)
		return
	}
	err = cfg.db.DeleteChirp(r.Context(), chirp.ID)
	if err != nil {
		log.Printf("Error deleting chirp: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(204)
}
