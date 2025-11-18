package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/joho/godotenv"
	"github.com/rahul4469/github-analyzer/controllers"
	"github.com/rahul4469/github-analyzer/migrations"
	"github.com/rahul4469/github-analyzer/models"
	"github.com/rahul4469/github-analyzer/templates"
	"github.com/rahul4469/github-analyzer/views"
)

type config struct {
	PSQL   models.PostgresConfig
	Server struct {
		Address string
	}
	CSRF struct {
		Key            string
		Secure         bool
		TrustedOrigins []string
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

	cfg.CSRF.Key = os.Getenv("CSRF_KEY")
	cfg.CSRF.Secure = os.Getenv("CSRF_SECURE") == "true"
	cfg.CSRF.TrustedOrigins = strings.Fields(os.Getenv("CSRF_TRUSTED_ORIGINS"))

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

	// Setup Services ---------------
	userService := &models.UserService{
		DB: db,
	}
	sessionService := &models.SessionService{
		DB: db,
	}

	//CSRF middleware
	csrfMw := csrf.Protect([]byte(cfg.CSRF.Key), csrf.Secure(cfg.CSRF.Secure), csrf.Path("/"), csrf.TrustedOrigins(cfg.CSRF.TrustedOrigins))
	umw := controllers.UserMiddleware{
		SessionService: sessionService,
	}

	// Setup Contollers ---------------
	userC := controllers.Users{
		UserService:    userService,
		SessionService: sessionService,
	}
	userC.Template.New, err = views.ParseFS(templates.FS, "signup.gohtml", "tailwind.gohtml")
	if err != nil {
		panic(err)
	}
	userC.Template.Signin, err = views.ParseFS(templates.FS, "signin.gohtml", "tailwind.gohtml")
	if err != nil {
		panic(err)
	}

	// Setup router and routes
	r := chi.NewRouter()
	r.Use(csrfMw)
	r.Use(umw.SetUser)

	tpl, err := views.ParseFS(templates.FS, "home.gohtml", "tailwind.gohtml")
	if err != nil {
		panic(err)
	}
	r.Get("/", controllers.StaticHandler(tpl))

	r.Get("/signup", userC.New)
	r.Post("/users", userC.Create)
	r.Get("/signin", userC.Signin)
	r.Post("/signin", userC.ProcessSignIn)
	r.Post("/signout", userC.ProcessSignOut)

	r.Route("/users/me", func(r chi.Router) {
		r.Use(umw.RequireUser)
		r.Get("/", userC.CurrentUser)

	})

	// Start the Server
	fmt.Printf("Starting server at port %s...\n", cfg.Server.Address)
	return http.ListenAndServe(cfg.Server.Address, r)
}
