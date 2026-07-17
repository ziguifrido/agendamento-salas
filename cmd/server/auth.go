package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

const (
	roleAdmin        = "admin"
	roleUser         = "user"
	sessionCookie    = "session"
	oauthStateCookie = "oauth_state"
	sessionDuration  = 7 * 24 * time.Hour
)

type User struct {
	Email, Name, AvatarURL, Role, CreatedAt, UpdatedAt, LastLogin string
}

type authState struct {
	User User
	CSRF string
}

type authKey struct{}

func emailSet(value string) map[string]bool {
	out := map[string]bool{}
	for _, email := range strings.Split(value, ",") {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			out[email] = true
		}
	}
	return out
}

func currentAuth(r *http.Request) *authState {
	auth, _ := r.Context().Value(authKey{}).(*authState)
	return auth
}

func (a *App) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		public := r.URL.Path == "/" || r.URL.Path == "/auth/google" || r.URL.Path == "/auth/google/callback"
		if cookie, err := r.Cookie(sessionCookie); err == nil {
			if auth, err := a.session(cookie.Value); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), authKey{}, auth))
			} else {
				a.clearSessionCookie(w, r)
			}
		}
		if currentAuth(r) == nil && !public {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := currentAuth(r)
		if r.Method == http.MethodPost && auth != nil && subtle.ConstantTimeCompare([]byte(auth.CSRF), []byte(r.FormValue("_csrf"))) != 1 {
			http.Error(w, "token CSRF inválido", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) session(token string) (*authState, error) {
	var auth authState
	err := a.db.QueryRow(`SELECT u.email,u.name,u.avatar_url,u.role,u.created_at,u.updated_at,u.last_login,s.csrf_token
FROM sessions s JOIN users u ON u.email=s.user_email
WHERE s.token_hash=? AND s.expires_at>?`, tokenHash(token), time.Now().UTC().Format(time.RFC3339)).Scan(
		&auth.User.Email, &auth.User.Name, &auth.User.AvatarURL, &auth.User.Role, &auth.User.CreatedAt, &auth.User.UpdatedAt, &auth.User.LastLogin, &auth.CSRF)
	return &auth, err
}

func (a *App) createSession(ctx context.Context, email string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	csrfToken, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if _, err := a.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=?", now.Format(time.RFC3339)); err != nil {
		return "", err
	}
	_, err = a.db.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_email,csrf_token,expires_at,created_at) VALUES(?,?,?,?,?)", tokenHash(token), email, csrfToken, now.Add(sessionDuration).Format(time.RFC3339), now.Format(time.RFC3339))
	return token, err
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if _, err := a.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=?", tokenHash(cookie.Value)); err != nil {
			a.clearSessionCookie(w, r)
			a.serverError(w, "delete session", err)
			return
		}
	}
	a.clearSessionCookie(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) userRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	auth, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	role := r.FormValue("role")
	if role != roleAdmin && role != roleUser {
		http.Error(w, "papel inválido", http.StatusBadRequest)
		return
	}
	result, err := a.db.ExecContext(r.Context(), `UPDATE users SET role=?,updated_at=? WHERE email=? AND NOT (
email=? AND role='admin' AND ?='user' AND (SELECT COUNT(*) FROM users WHERE role='admin')<=1)`,
		role, time.Now().UTC().Format(time.RFC3339), email, auth.User.Email, role)
	if err != nil {
		a.serverError(w, "update user role", err)
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		message := "Usuário não encontrado."
		if email == auth.User.Email && role == roleUser {
			message = "O sistema precisa manter pelo menos um administrador."
		}
		setFlash(w, flash{ActionError: message})
	} else {
		a.notify()
		setFlash(w, flash{Message: "Permissão do usuário atualizada."})
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) userList() ([]User, error) {
	rows, err := a.db.Query("SELECT email,name,avatar_url,role,created_at,updated_at,last_login FROM users ORDER BY name COLLATE NOCASE,email COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.Email, &user.Name, &user.AvatarURL, &user.Role, &user.CreatedAt, &user.UpdatedAt, &user.LastLogin); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func requireAdmin(w http.ResponseWriter, r *http.Request) (*authState, bool) {
	auth := currentAuth(r)
	if auth == nil || auth.User.Role != roleAdmin {
		http.Error(w, "acesso restrito a administradores", http.StatusForbidden)
		return nil, false
	}
	return auth, true
}

func (a *App) authFailure(w http.ResponseWriter, r *http.Request, message string) {
	setFlash(w, flash{AuthError: message})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode})
}

func (a *App) client() *http.Client {
	if a.httpClient != nil {
		return a.httpClient
	}
	return http.DefaultClient
}

func (a *App) logAuthError(message string, err error) {
	if a.log != nil {
		a.log.Error(message, "error", err)
	}
}

func secureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
