package main

import (
	"database/sql"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Room struct {
	ID, Capacity                           int
	Name, Description, Location, Resources string
}

type Booking struct {
	ID, RoomID                                                                 int
	Room, Owner, Title, Description, Day, Starts, Ends, RequesterEmail, Status string
	CanCancel, CanReview, CanEdit                                              bool
}

type AgendaDay struct {
	Day      string
	Bookings []Booking
}

type App struct {
	db                 *sql.DB
	templates          *template.Template
	log                *slog.Logger
	mu                 sync.Mutex
	subscribers        map[chan struct{}]struct{}
	googleClientID     string
	googleClientSecret string
	googleRedirectURL  string
	allowedEmailDomain string
	initialAdminUsers  map[string]bool
	oauthAuthorization string
	oauthToken         string
	oauthUserInfo      string
	httpClient         *http.Client
}

type flash struct {
	Message, Error, ActionError, RoomError, RoomDialog, AuthError string
	Form                                                          Booking
}

const (
	maxFormBytes   = 64 << 10
	maxFlashBytes  = 2800
	maxNameBytes   = 100
	maxTitleBytes  = 150
	maxDetailBytes = 200
	maxTextBytes   = 1000
)

func main() {
	if err := loadEnvFile(".env"); err != nil {
		panic(err)
	}
	path := os.Getenv("DATABASE_PATH")
	if path == "" {
		path = "data/reservas.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		panic(err)
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := migrate(db); err != nil {
		panic(err)
	}
	t := template.Must(template.New("").Funcs(template.FuncMap{"now": func() string { return time.Now().Format("2006-01-02") }, "dateBR": dateBR, "weekdayBR": weekdayBR}).ParseGlob("web/templates/*.html"))
	a := &App{
		db:                 db,
		templates:          t,
		log:                slog.New(slog.NewJSONHandler(os.Stdout, nil)),
		googleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		googleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		googleRedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		allowedEmailDomain: strings.ToLower(strings.TrimSpace(os.Getenv("ALLOWED_EMAIL_DOMAIN"))),
		initialAdminUsers:  emailSet(os.Getenv("INITIAL_ADMIN_USERS")),
		oauthAuthorization: "https://accounts.google.com/o/oauth2/v2/auth",
		oauthToken:         "https://oauth2.googleapis.com/token",
		oauthUserInfo:      "https://openidconnect.googleapis.com/v1/userinfo",
		httpClient:         &http.Client{Timeout: 10 * time.Second},
	}
	if a.googleClientID == "" || a.googleClientSecret == "" || a.googleRedirectURL == "" {
		panic("GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET e GOOGLE_REDIRECT_URL são obrigatórios")
	}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/templates/static"))))
	mux.HandleFunc("/auth/google", a.googleLogin)
	mux.HandleFunc("/auth/google/callback", a.googleCallback)
	mux.HandleFunc("/logout", a.logout)
	mux.HandleFunc("/users/role", a.userRole)
	mux.HandleFunc("/", a.dashboard)
	mux.HandleFunc("/rooms", a.rooms)
	mux.HandleFunc("/rooms/", a.roomAction)
	mux.HandleFunc("/agenda/", a.navigateAgenda)
	mux.HandleFunc("/bookings", a.bookings)
	mux.HandleFunc("/bookings/", a.bookingAction)
	mux.HandleFunc("/events", a.events)
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	a.log.Info("server started", "address", addr)
	server := &http.Server{Addr: addr, Handler: security(a.authenticate(a.csrf(mux))), ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		a.log.Error("server stopped", "error", err)
	}
}

func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none';")
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if secureRequest(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if r.Method == http.MethodPost && r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host && r.Header.Get("Origin") != "https://"+r.Host {
			http.Error(w, "origem inválida", 403)
			return
		}
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
			if err := r.ParseForm(); err != nil {
				http.Error(w, "formulário inválido ou muito grande", http.StatusRequestEntityTooLarge)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) serverError(w http.ResponseWriter, message string, err error) {
	if a.log != nil {
		a.log.Error(message, "error", err)
	}
	http.Error(w, "erro interno", http.StatusInternalServerError)
}
