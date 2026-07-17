package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

func (a *App) googleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if currentAuth(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	state, err := randomToken()
	if err != nil {
		a.serverError(w, "oauth state", err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode})
	query := url.Values{
		"client_id":     {a.googleClientID},
		"redirect_uri":  {a.googleRedirectURL},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"prompt":        {"select_account"},
	}
	http.Redirect(w, r, a.oauthAuthorization+"?"+query.Encode(), http.StatusFound)
}

func (a *App) googleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	state, err := r.Cookie(oauthStateCookie)
	queryState := r.URL.Query().Get("state")
	if err != nil || state.Value == "" || queryState == "" || subtle.ConstantTimeCompare([]byte(state.Value), []byte(queryState)) != 1 {
		a.authFailure(w, r, "Não foi possível validar o início do login. Tente novamente.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode})
	if problem := r.URL.Query().Get("error"); problem != "" {
		a.authFailure(w, r, "O login com Google foi cancelado ou recusado.")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		a.authFailure(w, r, "O Google não retornou um código de autenticação válido.")
		return
	}
	accessToken, err := a.exchangeCode(r.Context(), code)
	if err != nil {
		a.logAuthError("oauth token", err)
		a.authFailure(w, r, "Não foi possível concluir o login com Google.")
		return
	}
	profile, err := a.googleProfile(r.Context(), accessToken)
	if err != nil {
		a.logAuthError("oauth userinfo", err)
		a.authFailure(w, r, "Não foi possível validar sua conta Google.")
		return
	}
	if !a.emailAllowed(profile.Email) {
		a.authFailure(w, r, "Esta conta não possui autorização para acessar o sistema.")
		return
	}
	user, err := a.upsertUser(r.Context(), profile)
	if err != nil {
		a.serverError(w, "save user", err)
		return
	}
	token, err := a.createSession(r.Context(), user.Email)
	if err != nil {
		a.serverError(w, "create session", err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int(sessionDuration.Seconds()), HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {a.googleClientID},
		"client_secret": {a.googleClientSecret},
		"redirect_uri":  {a.googleRedirectURL},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.oauthToken, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.client().Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return "", fmt.Errorf("token endpoint: %s: %s", response.Status, body)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return "", err
	}
	if token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") {
		return "", fmt.Errorf("token response inválida")
	}
	return token.AccessToken, nil
}

type googleUser struct {
	Email         string `json:"email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	EmailVerified bool   `json:"email_verified"`
}

func (a *App) googleProfile(ctx context.Context, accessToken string) (googleUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.oauthUserInfo, nil)
	if err != nil {
		return googleUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := a.client().Do(req)
	if err != nil {
		return googleUser{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return googleUser{}, fmt.Errorf("userinfo endpoint: %s", response.Status)
	}
	var profile googleUser
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile); err != nil {
		return googleUser{}, err
	}
	profile.Email = strings.ToLower(strings.TrimSpace(profile.Email))
	profile.Name = strings.TrimSpace(profile.Name)
	address, err := mail.ParseAddress(profile.Email)
	if err != nil || !strings.EqualFold(address.Address, profile.Email) || !profile.EmailVerified || profile.Name == "" {
		return googleUser{}, fmt.Errorf("perfil Google inválido")
	}
	return profile, nil
}

func (a *App) emailAllowed(email string) bool {
	if a.allowedEmailDomain == "" {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	return at >= 0 && strings.EqualFold(email[at+1:], a.allowedEmailDomain)
}

func (a *App) upsertUser(ctx context.Context, profile googleUser) (User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	role := roleUser
	bootstrapAdmin := a.initialAdminUsers[profile.Email]
	if bootstrapAdmin {
		role = roleAdmin
	}
	_, err := a.db.ExecContext(ctx, `INSERT INTO users(email,name,avatar_url,role,created_at,updated_at,last_login) VALUES(?,?,?,?,?,?,?)
ON CONFLICT(email) DO UPDATE SET name=excluded.name,avatar_url=excluded.avatar_url,updated_at=excluded.updated_at,last_login=excluded.last_login,
role=CASE WHEN ? THEN 'admin' ELSE users.role END`, profile.Email, profile.Name, profile.Picture, role, now, now, now, bootstrapAdmin)
	if err != nil {
		return User{}, err
	}
	var user User
	err = a.db.QueryRowContext(ctx, "SELECT email,name,avatar_url,role,created_at,updated_at,last_login FROM users WHERE email=?", profile.Email).Scan(
		&user.Email, &user.Name, &user.AvatarURL, &user.Role, &user.CreatedAt, &user.UpdatedAt, &user.LastLogin)
	return user, err
}
