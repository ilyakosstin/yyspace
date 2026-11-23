package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	binLimit, err := strconv.ParseInt(env("BIN_LIMIT", "200"), 10, 32)
	if err != nil || binLimit < 1 {
		return errors.New("BIN_LIMIT must be a positive integer")
	}
	var origins []string
	if err := json.Unmarshal([]byte(env("CORS_ORIGINS", `["http://localhost:5173"]`)), &origins); err != nil {
		return err
	}
	client, err := mongo.Connect(options.Client().ApplyURI(env("MONGODB_URI", "mongodb://localhost:27017")).
		SetServerSelectionTimeout(5 * time.Second))
	if err != nil {
		return err
	}
	defer func() {
		disconnect, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(disconnect)
	}()

	init, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	db := client.Database(env("MONGODB_DATABASE", "focus_helper"))
	s, err := openStore(init, db, binLimit)
	if err != nil {
		return err
	}
	if len(os.Args) == 2 && os.Args[1] == "import" {
		return s.importLegacy(ctx, os.Stdin)
	}
	auth, err := openAuthentication(init, db)
	if err != nil {
		return err
	}
	v, err := openVault(init, db)
	if err != nil {
		return err
	}
	cancel()
	routes := s.routes()
	v.routes(routes)

	server := &http.Server{
		Addr: env("HTTP_ADDR", ":8000"), Handler: cors(auth.protect(routes), origins),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second,
		WriteTimeout: 25 * time.Second, IdleTimeout: 60 * time.Second,
	}
	result := make(chan error, 1)
	go func() {
		log.Printf("HTTP listening on %s", server.Addr)
		result <- server.ListenAndServe()
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func cors(next http.Handler, origins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Add("Vary", "Origin")
			if slices.Contains(origins, origin) || slices.Contains(origins, "*") {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(r.Header.Values("Access-Control-Request-Headers"), ", "))
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
