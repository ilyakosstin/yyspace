package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "focus_session"
const sessionLifetime = 7 * 24 * time.Hour

type authentication struct {
	sessions     *mongo.Collection
	username     string
	passwordHash []byte
	secure       bool
}

func openAuthentication(ctx context.Context, db *mongo.Database) (*authentication, error) {
	secure, err := strconv.ParseBool(env("APP_SESSION_SECURE", "true"))
	if err != nil {
		return nil, errors.New("APP_SESSION_SECURE must be true or false")
	}
	hash := []byte(env("APP_PASSWORD_HASH", ""))
	if len(hash) > 0 {
		if _, err := bcrypt.Cost(hash); err != nil {
			return nil, errors.New("APP_PASSWORD_HASH must be a bcrypt hash")
		}
	}
	sessions := db.Collection("sessions")
	_, err = sessions.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0),
	})
	return &authentication{sessions, env("APP_USERNAME", "focus"), hash, secure}, err
}

func (a *authentication) sessionFilter(r *http.Request) (bson.M, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || len(cookie.Value) != base64.RawURLEncoding.EncodedLen(32) {
		return nil, false
	}
	digest := sha256.Sum256([]byte(cookie.Value))
	return bson.M{"_id": digest[:]}, true
}

func (a *authentication) signedIn(r *http.Request) (bool, error) {
	if len(a.passwordHash) == 0 {
		return false, nil
	}
	filter, ok := a.sessionFilter(r)
	if !ok {
		return false, nil
	}
	filter["expires_at"] = bson.M{"$gt": time.Now()}
	filter["credentials"] = a.credentialVersion()
	err := a.sessions.FindOne(r.Context(), filter, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

func (a *authentication) credentialVersion() []byte {
	digest := sha256.Sum256(append([]byte(a.username+"\x00"), a.passwordHash...))
	return digest[:]
}

func (a *authentication) cookie(w http.ResponseWriter, value string, age int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", MaxAge: age,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode,
	})
}

func (a *authentication) session(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		signedIn, err := a.signedIn(r)
		if err != nil {
			writeError(w, err)
			return
		}
		var user *string
		if signedIn {
			user = &a.username
		}
		writeJSON(w, http.StatusOK, map[string]*string{"username": user})
	case http.MethodPost:
		a.login(w, r)
	case http.MethodDelete:
		if filter, ok := a.sessionFilter(r); ok {
			if _, err := a.sessions.DeleteOne(r.Context(), filter); err != nil {
				writeError(w, err)
				return
			}
		}
		a.cookie(w, "", -1)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *authentication) login(w http.ResponseWriter, r *http.Request) {
	if len(a.passwordHash) == 0 {
		writeError(w, failure{http.StatusServiceUnavailable, "Sign-in is unavailable until an account is configured"})
		return
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/json" {
		writeError(w, failure{http.StatusUnsupportedMediaType, "Sign-in expects JSON"})
		return
	}
	body, err := decode[struct{ Username, Password string }](w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(body.Password) > 72 || bcrypt.CompareHashAndPassword(a.passwordHash, []byte(body.Password)) != nil ||
		subtle.ConstantTimeCompare([]byte(body.Username), []byte(a.username)) != 1 {
		writeError(w, failure{http.StatusUnauthorized, "Incorrect username or password"})
		return
	}

	token := make([]byte, 32)
	rand.Read(token)
	value := base64.RawURLEncoding.EncodeToString(token)
	digest := sha256.Sum256([]byte(value))
	_, err = a.sessions.InsertOne(r.Context(), bson.M{
		"_id": digest[:], "credentials": a.credentialVersion(), "expires_at": time.Now().Add(sessionLifetime),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	a.cookie(w, value, int(sessionLifetime/time.Second))
	writeJSON(w, http.StatusOK, map[string]string{"username": a.username})
}

func (a *authentication) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/api/session" {
			a.session(w, r)
			return
		}
		signedIn, err := a.signedIn(r)
		switch {
		case err != nil:
			writeError(w, err)
		case !signedIn:
			writeError(w, failure{http.StatusUnauthorized, "Sign in to continue"})
		default:
			next.ServeHTTP(w, r)
		}
	})
}
