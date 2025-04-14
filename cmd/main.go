package main

import (
	"avito_intr/internal/auth/jwt_auth"
	"avito_intr/internal/http_api"
	"avito_intr/internal/storage/pg_storage"
	"net/http"
	"os"
)

func main() {
	pgConn, ok := os.LookupEnv("PG_CONN")
	if !ok {
		panic("PG_CONN environment variable not set")
	}
	jwtKey, ok := os.LookupEnv("JWT_SECRET_KEY")
	if !ok {
		jwtKey = "secret_key"
	}
	port, ok := os.LookupEnv("PORT")
	if !ok {
		port = "8080"
	}
	pg, err := pg_storage.NewPgStorage(pgConn)
	if err != nil {
		panic(err)
	}
	err = pg.Migrate()
	if err != nil {
		panic(err)
	}

	auth := jwt_auth.NewJwtAuth(jwtKey)
	h := http_api.NewServer(pg, auth)

	err = http.ListenAndServe(":"+port, h)
	if err != nil {
		panic(err)
	}
}
