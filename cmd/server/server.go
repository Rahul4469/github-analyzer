package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/csrf"

	"github.com/rahul4469/github-analyzer/internal/config"
	"github.com/rahul4469/github-analyzer/internal/controllers"
	"github.com/rahul4469/github-analyzer/internal/crypto"
	"github.com/rahul4469/github-analyzer/internal/middleware"
	"github.com/rahul4469/github-analyzer/internal/models"
	"github.com/rahul4469/github-analyzer/internal/services"
	"github.com/rahul4469/github-analyzer/internal/views"
)

func main() {
	// Load Configs
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	log.Printf("Starting GitHub Analyzer in %s mode", cfg.Server.Environment)

	// initialize encryptor for token storage
	encryptor, err := crypto.NewEncryptorFromString(cfg.Security.EncryptionKey)
	if err != nil {
		log.Fatalf("Failed to create encryptor: %v", err)
	}

	// DATABASE
	ctx := context.Background()
	db, err := models.NewDatabase(ctx, models.DefaultDatabaseConfig(cfg.Database.URL))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Connected to database")

	// Run migrations automatically on startup
	// log.Println("Running database migrations...")
	// if err := models.Migrate(db.DB, "./migrations"); err != nil {
	// 	log.Fatalf("Failed to run migrations: %v", err)
	// }

	// Initialize Template filesystem (OS filesystem for development)
	views.TemplateFS = os.DirFS(".").(fs.ReadDirFS)

	// Parse templates
	templates := parseTemplates()

	// SERVICES
	userService := models.NewUserService(db.Pool, cfg.Security.BcryptCost)
	sessionService := models.NewSessionService(db.Pool, cfg.Security.SessionDuration)
	repositoryService := models.NewRepositoryService(db.Pool)
	analysisService := models.NewAnalysisService(db.Pool)

	githubService := services.NewGitHubService(cfg.APIs.GitHubAPIBaseURL)
	perplexityService := services.NewPerplexityService(cfg.APIs.PerplexityAPIKey, cfg.APIs.PerplexityModel)

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(sessionService, cfg.Security.SessionCookieName)

	// CONTROLLERS
	staticController := controllers.NewStaticController(controllers.StaticTemplates{
		Home: templates.home,
	})

	authController := controllers.NewAuthController(
		userService,
		sessionService,
		controllers.AuthTemplates{
			SignUp: templates.signUp,
			SignIn: templates.signIn,
		},
		cfg.Security.SessionCookieName,
		cfg.Security.SecureCookies,
		cfg.Security.SessionDuration,
		cfg.Limits.DefaultUserQuota,
	)

	dashboardController := controllers.NewDashboardController(
		analysisService,
		repositoryService,
		templates.dashboard,
	)

	analyzeController := controllers.NewAnalyzeController(
		analysisService,
		repositoryService,
		userService,
		githubService,
		perplexityService,
		encryptor,
		controllers.AnalyzeTemplates{
			Form:   templates.analyze,
			Result: templates.result,
		},
	)

	oauthController := controllers.NewOAuthController(
		userService,
		sessionService,
		encryptor,
		controllers.OAuthConfig{
			ClientID:     cfg.GitHubOAuth.ClientID,
			ClientSecret: cfg.GitHubOAuth.ClientSecret,
			RedirectURL:  cfg.GitHubOAuth.RedirectURL,
			Scopes:       cfg.GitHubOAuth.Scopes,
		},
		cfg.Security.SessionCookieName,
		cfg.Security.SecureCookies,
		cfg.Security.SessionDuration,
	)

	// Setup Router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Timeout(60 * time.Second))

	// CSRF protection
	csrfMiddleware := csrf.Protect(
		[]byte(cfg.Security.CSRFSecret),
		csrf.Secure(cfg.Security.SecureCookies),
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.TrustedOrigins([]string{"localhost:3000", "127.0.0.1:3000"}),
	)
	r.Use(csrfMiddleware)

	// Auth middleware (loads user from session)
	r.Use(authMiddleware.SetUser)

	// Static files (serve from fs)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Health check(no auth required)
	r.Get("/health", controllers.HealthCheck)

	// Public routes
	r.Get("/", staticController.GetHome)

	// OAuth routes (public - GitHub redirects here)
	r.Get("/auth/github/login", oauthController.GitHubLogin)
	r.Get("/auth/github/callback", oauthController.GitHubCallback)

	// Auth routes (accessible only when "not" logged in)
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware.RequireNoUser)
		r.Get("/signup", authController.GetSignUp)
		r.Post("/signup", authController.PostSignUp)
		r.Get("/signin", authController.GetSignIn)
		r.Post("/signin", authController.PostSignIn)
	})

	// Logout (requires being logged in)
	r.Post("/logout", authController.PostLogout)

	// Protected routes (require authentication)
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware.RequireUser)

		r.Get("/dashboard", dashboardController.GetDashboard)

		// GitHub connection management
		r.Get("/auth/github/connect", oauthController.GitHubConnect)
		r.Get("/auth/github/disconnect", oauthController.GitHubDisconnect)

		r.Get("/analyze", analyzeController.GetAnalyze)
		r.Post("/analyze", analyzeController.PostAnalyze)
		r.Get("/analyze/{id}", analyzeController.GetResult)
		r.Post("/analyze/{id}/delete", analyzeController.DeleteAnalysis)
	})

	// Start session cleanup routine
	stopCleanup := sessionService.StartCleanupRoutine(1 * time.Hour)
	defer close(stopCleanup)

	// Create Server
	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on http://localhost:%s", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server stopped gracefully")
}

// Templates related code --------------------------------------------------

// templates holds all parsed templates.
type appTemplates struct {
	home      *views.Template
	signUp    *views.Template
	signIn    *views.Template
	dashboard *views.Template
	analyze   *views.Template
	result    *views.Template
}

func parseTemplates() *appTemplates {
	mustParse := func(path string) *views.Template {
		tmpl, err := views.ParseFS(path)
		if err != nil {
			panic(fmt.Sprintf("Failed to parse template %s: %v", path, err))
		}
		return tmpl
	}
	return &appTemplates{
		home:      mustParse("pages/home.gohtml"),
		signUp:    mustParse("pages/signup.gohtml"),
		signIn:    mustParse("pages/signin.gohtml"),
		dashboard: mustParse("pages/dashboard.gohtml"),
		analyze:   mustParse("pages/analyze.gohtml"),
		result:    mustParse("pages/result.gohtml"),
	}
}
