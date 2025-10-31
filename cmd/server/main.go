package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/!rahul4469/lenslocked/views"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"github.com/rahul4469/github-analyzer/controllers"
	"github.com/rahul4469/github-analyzer/migrations"
	"github.com/rahul4469/github-analyzer/models"
	"github.com/rahul4469/github-analyzer/templates"
)

type config struct {
	PSQL   models.PostgresConfig
	Server struct {
		Address string
	}
}

func loadEnvConfig() (config, error) {
	var cfg config

	err := godotenv.Load()
	if err != nil {
		panic(err)
	}

	cfg.PSQL = models.PostgresConfig{
		Host:     os.Getenv("PSQL_HOST"),
		Port:     os.Getenv("PSQL_PORT"),
		User:     os.Getenv("PSQL_USER"),
		Password: os.Getenv("PSQL_PASSWORD"),
		Database: os.Getenv("PSQL_DATABASE"),
		SSLMode:  os.Getenv("PSQL_SSLMODE"),
	}
	if cfg.PSQL.Host == "" && cfg.PSQL.Port == "" {
		return cfg, fmt.Errorf("no psql config provided")
	}

	cfg.Server.Address = os.Getenv("SERVER_ADDRESS")

	return cfg, err
}

func main() {
	cfg, err := loadEnvConfig()
	if err != nil {
		panic(err)
	}
	err = run(cfg)
	if err != nil {
		panic(err)
	}
}

func run(cfg config) error {
	// Setup the Database ---------------
	db, err := models.Open(cfg.PSQL)
	if err != nil {
		return err
	}
	defer db.Close()

	err = models.MigrateFS(db, migrations.FS, ".")
	if err != nil {
		return err
	}

	// Setup router
	r := chi.NewRouter()

	tpl, err := views.ParseFS(templates.FS, "home.gohtml", "tailwind.gohtml")
	if err != nil {
		panic(err)
	}
	r.Get("/", controllers.StaticHandler(tpl))

	return http.ListenAndServe(cfg.Server.Address, r)
}
