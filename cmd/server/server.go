package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/joho/godotenv"
	localcontext "github.com/rahul4469/github-analyzer/context"
	"github.com/rahul4469/github-analyzer/internal/controllers"
	"github.com/rahul4469/github-analyzer/internal/models"
	"github.com/rahul4469/github-analyzer/internal/services"
	"github.com/rahul4469/github-analyzer/internal/views"
	"github.com/rahul4469/github-analyzer/migrations"
	"github.com/rahul4469/github-analyzer/templates"
)

type config struct {
	PSQL   models.PostgresConfig
	Server struct {
		Address      string
		ReadTimeout  time.Duration
		WriteTimeout time.Duration
		IdleTimeout  time.Duration
	}
	CSRF struct {
		Key            string
		Secure         bool
		SameSite       string
		TrustedOrigins []string
	}
	PerplexityAPIKey string
}

func loadEnvConfig() (config, error) {
	var cfg config

	// load .env file
	err := godotenv.Load()
	if err != nil {
		panic(err)
	}

	// Databse config
	cfg.PSQL = models.PostgresConfig{
		Host:     getEnvOrDefault("PSQL_HOST", "localhost"),
		Port:     getEnvOrDefault("PSQL_PORT", "5432"),
		User:     getEnvOrDefault("PSQL_USER", "postgres"),
		Password: getEnvOrDefault("PSQL_PASSWORD", ""),
		Database: getEnvOrDefault("PSQL_DATABASE", "github_analyzer"),
		SSLMode:  getEnvOrDefault("PSQL_SSLMODE", "disable"),
	}
	if cfg.PSQL.Host == "" && cfg.PSQL.Database == "" {
		return cfg, fmt.Errorf("no psql config provided")
	}

	// CSRF config
	cfg.CSRF.Key = getEnvOrRequired("CSRF_KEY", "CSRF_KEY environment variable is required")
	if len(cfg.CSRF.Key) < 32 {
		return cfg, fmt.Errorf("CSRF_KEY must be at least 32 characters long")
	}

	cfg.CSRF.Secure = getEnvOrDefault("CSRF_SECURE", "false") == "true"
	cfg.CSRF.SameSite = getEnvOrDefault("CSRF_SAMESITE", "Lax")
	cfg.CSRF.TrustedOrigins = strings.Fields(getEnvOrDefault("CSRF_TRUSTED_ORIGINS", ""))

	// Server config
	cfg.Server.Address = getEnvOrDefault("SERVER_ADDRESS", ":8080")
	cfg.Server.ReadTimeout = getDurationOrDefault("SERVER_READ_TIMEOUT", 15*time.Second)
	cfg.Server.WriteTimeout = getDurationOrDefault("SERVER_WRITE_TIMEOUT", 15*time.Second)
	cfg.Server.IdleTimeout = getDurationOrDefault("SERVER_IDLE_TIMEOUT", 60*time.Second)

	// PPLX api
	cfg.PerplexityAPIKey = getEnvOrRequired("PERPLEXITY_API_KEY", "PERPLEXITY_API_KEY environment variable is required")

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
	log.Println("Connecting to database...")
	db, err := models.Open(cfg.PSQL)
	if err != nil {
		return err
	}
	defer db.Close()
	// Test connection
	err = db.Ping()
	if err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	log.Println("Database connected successfully")

	// run migrations
	err = models.MigrateFS(db, migrations.FS, ".")
	if err != nil {
		return err
	}

	// Setup Services ---------------
	userService := models.NewUserService(db)
	repositoryService := models.NewRepositoryService(db)
	analysisService := models.NewAnalysisService(db)
	sessionService := models.NewSessionService(db)
	githubFetcher := services.NewGitHubFetcher("")
	aiAnalyzer := services.NewAIAnalyzer(cfg.PerplexityAPIKey)
	dataFormatter := services.NewDataFormatter()

	// Initialize template renderer (controller-level renderer)
	templateRenderer := controllers.NewTemplateRenderer(
		templates.FS,
		"",
		true, // Enable caching in production
	)

	//CSRF middleware
	csrfMw := csrf.Protect([]byte(cfg.CSRF.Key), csrf.Secure(cfg.CSRF.Secure), csrf.Path("/"), csrf.TrustedOrigins(cfg.CSRF.TrustedOrigins))
	umw := controllers.UserMiddleware{
		SessionService: sessionService,
	}

	// Setup Controllers ---------------
	// Parse signup/signin templates using the `views` helper and pass to auth controller
	authTpl, err := views.ParseFS(templates.FS, "signup.gohtml", "tailwind.gohtml")
	if err != nil {
		panic(err)
	}

	authCtrl := controllers.NewAuthController(
		userService,
		sessionService,
		authTpl,
	)

	analyzeCtrl := controllers.NewAnalyzeController(
		userService,
		repositoryService,
		analysisService,
		githubFetcher,
		aiAnalyzer,
		dataFormatter,
		templateRenderer,
	)

	// dashboardController := controllers.NewDashboardController(
	// 	userService,
	// 	repositoryService,
	// 	analysisService,
	// 	templateRenderer,
	// )
	// userC.Template.New, err = views.ParseFS(templates.FS, "signup.gohtml", "tailwind.gohtml")
	// if err != nil {
	// 	panic(err)
	// }
	// userC.Template.Signin, err = views.ParseFS(templates.FS, "signin.gohtml", "tailwind.gohtml")
	// if err != nil {
	// 	panic(err)
	// }

	// Setup router and routes
	r := chi.NewRouter()
	r.Use(csrfMw)
	r.Use(umw.SetUser)

	// tpl, err := views.ParseFS(templates.FS, "home.gohtml", "base.gohtml")
	// if err != nil {
	// 	panic(err)
	// }
	// r.Get("/", controllers.StaticHandler(tpl))

	// ---- Public Routes ----
	r.Group(func(r chi.Router) {
		// Home page
		r.Get("/", handleHome)

		// Authentication routes
		r.Get("/signup", authCtrl.GetSignUp)
		r.Post("/signup", authCtrl.PostSignUp)

		r.Get("/signin", authCtrl.GetSignIn)
		r.Post("/signin", authCtrl.PostSignIn)
	})
	// ---- Protected Routes ----
	r.Group(func(r chi.Router) {
		// Require authentication for these routes
		r.Use(umw.RequireUser)

		// Dashboard
		// r.Get("/dashboard", dashboardCtrl.GetDashboard)

		// Analysis
		r.Get("/analyze", analyzeCtrl.GetAnalyzeForm)
		r.Post("/analyze", analyzeCtrl.PostAnalyze)
		// r.Get("/analysis/{id}", analyzeCtrl.GetAnalysisResults)

		// User
		r.Get("/profile", handleProfile)
		r.Post("/logout", authCtrl.PostLogout)
	})

	// Protected user routes can be added here (requires a UserController).

	// Start the Server
	fmt.Printf("Starting server at port %s...\n", cfg.Server.Address)
	return http.ListenAndServe(cfg.Server.Address, r)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	user := localcontext.UserFromContext(r.Context())
	w.Header().Set("Content-Type", "text/html")

	if user != nil {
		// Logged in, show dashboard redirect
		fmt.Fprintf(w, `<html><body>Welcome %s! <a href="/dashboard">Go to Dashboard</a></body></html>`, user.Email)
	} else {
		// Not logged in, show login/signup links
		fmt.Fprint(w, `<html><body>
			<h1>Welcome to GitHub Analyzer</h1>
			<p><a href="/signin">Sign In</a> | <a href="/signup">Sign Up</a></p>
		</body></html>`)
	}
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	user := localcontext.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/signin", http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
		"id": %d,
		"email": "%s",
		"username": "%s"
	}`, user.ID, user.Email, user.Username)
}

// HELPER FUNCTIONS
// ============================================

// initializeTemplates loads all HTML templates
func initializeTemplates() (views.Template, error) {
	// Load base template first
	tpl, err := views.ParseFS(
		os.DirFS("templates"),
		"base.gohtml",
		"signin.gohtml",
		"signup.gohtml",
		"analyze.gohtml",
		"dashboard.gohtml",
		"repositories.gohtml",
		"code-structure.gohtml",
		"issues.gohtml",
	)
	if err != nil {
		return views.Template{}, fmt.Errorf("failed to parse templates: %w", err)
	}
	return tpl, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrRequired(key, errorMsg string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	panic(errorMsg)
}

func getDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			log.Printf("Warning: Invalid duration for %s: %v, using default", key, err)
			return defaultValue
		}
		return duration
	}
	return defaultValue
}
