package main

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGoogleCallbackCreatesAdminAndSession(t *testing.T) {
	db := testDB(t)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/token":
			if r.Method != http.MethodPost || r.FormValue("code") != "valid-code" {
				t.Fatal("invalid token request")
			}
			body = `{"access_token":"access-token","token_type":"Bearer"}`
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer access-token" {
				t.Fatal("missing bearer token")
			}
			body = `{"email":"admin@empresa.com.br","name":"Admin","picture":"https://example.com/avatar","email_verified":true}`
		default:
			t.Fatalf("unexpected provider request: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	a := &App{
		db: db, googleClientID: "client", googleClientSecret: "secret", googleRedirectURL: "http://app.test/auth/google/callback",
		allowedEmailDomain: "empresa.com.br", initialAdminUsers: emailSet("admin@empresa.com.br"),
		oauthToken: "https://provider.test/token", oauthUserInfo: "https://provider.test/userinfo", httpClient: client,
	}
	r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=valid-code&state=state", nil)
	r.TLS = &tls.ConnectionState{}
	r.AddCookie(&http.Cookie{Name: oauthStateCookie, Value: "state"})
	w := httptest.NewRecorder()
	a.googleCallback(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("got status %d", w.Code)
	}
	var role string
	if err := db.QueryRow("SELECT role FROM users WHERE email='admin@empresa.com.br'").Scan(&role); err != nil || role != roleAdmin {
		t.Fatalf("admin not created: %q, %v", role, err)
	}
	var sessions int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("session not created: %d, %v", sessions, err)
	}
	var sessionToken string
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == sessionCookie && cookie.HttpOnly && cookie.Secure && cookie.SameSite == http.SameSiteLaxMode {
			sessionToken = cookie.Value
		}
	}
	if sessionToken == "" {
		t.Fatal("secure session cookie missing")
	}
	if auth, err := a.session(sessionToken); err != nil || auth.User.Role != roleAdmin || auth.CSRF == "" {
		t.Fatalf("session validation failed: %+v, %v", auth, err)
	}
}

func TestAllowedEmailDomain(t *testing.T) {
	a := &App{allowedEmailDomain: "empresa.com.br"}
	if !a.emailAllowed("user@empresa.com.br") || a.emailAllowed("user@outraempresa.com.br") || a.emailAllowed("user@empresa.com.br.example") {
		t.Fatal("domain restriction failed")
	}
	a.allowedEmailDomain = ""
	if !a.emailAllowed("user@example.com") {
		t.Fatal("empty domain should allow verified emails")
	}
	a.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"email":"user@gmail.com","name":"User","email_verified":false}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	a.oauthUserInfo = "https://provider.test/userinfo"
	if _, err := a.googleProfile(t.Context(), "token"); err == nil {
		t.Fatal("unverified Google email accepted")
	}
}

func TestUserListIsSortedByName(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, user := range []struct{ email, name string }{{"bia@example.com", "bia"}, {"ana@example.com", "Ana"}, {"carla@example.com", "Carla"}} {
		if _, err := db.Exec("INSERT INTO users(email,name,role,created_at,updated_at,last_login) VALUES(?,?,'user',?,?,?)", user.email, user.name, now, now, now); err != nil {
			t.Fatal(err)
		}
	}
	users, err := (&App{db: db}).userList()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 || users[0].Name != "Ana" || users[1].Name != "bia" || users[2].Name != "Carla" {
		t.Fatalf("users are not sorted by name: %+v", users)
	}
}

func TestCSRFRejectsInvalidToken(t *testing.T) {
	called := false
	handler := (&App{}).csrf(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	r := requestAs(httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader("_csrf=wrong")), roleUser)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if called || w.Code != http.StatusForbidden {
		t.Fatalf("invalid CSRF accepted: called=%v status=%d", called, w.Code)
	}
}

func TestUserCannotDeleteRoom(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec("INSERT INTO rooms(name,capacity) VALUES('Sala 1',10)"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	(&App{db: db}).roomAction(w, requestAs(httptest.NewRequest(http.MethodPost, "/rooms/1/delete", nil), roleUser))
	if w.Code != http.StatusForbidden {
		t.Fatalf("got status %d", w.Code)
	}
}

func TestLastAdminCannotDemoteSelf(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec("INSERT INTO users(email,name,role,created_at,updated_at,last_login) VALUES('admin@example.com','Admin','admin',?,?,?)", now, now, now); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"email": {"admin@example.com"}, "role": {roleUser}}
	r := requestAs(httptest.NewRequest(http.MethodPost, "/users/role", strings.NewReader(form.Encode())), roleAdmin)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	(&App{db: db}).userRole(w, r)
	var role string
	if err := db.QueryRow("SELECT role FROM users WHERE email='admin@example.com'").Scan(&role); err != nil || role != roleAdmin {
		t.Fatalf("last admin demoted: %q, %v", role, err)
	}
	if _, err := db.Exec("INSERT INTO users(email,name,role,created_at,updated_at,last_login) VALUES('second@example.com','Second','admin',?,?,?)", now, now, now); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, email := range []string{"admin@example.com", "second@example.com"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			values := url.Values{"email": {email}, "role": {roleUser}}
			request := httptest.NewRequest(http.MethodPost, "/users/role", strings.NewReader(values.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			auth := &authState{User: User{Email: email, Role: roleAdmin}, CSRF: "csrf"}
			request = request.WithContext(context.WithValue(request.Context(), authKey{}, auth))
			(&App{db: db}).userRole(httptest.NewRecorder(), request)
		}()
	}
	close(start)
	wg.Wait()
	var admins int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role='admin'").Scan(&admins); err != nil || admins != 1 {
		t.Fatalf("concurrent demotion left %d admins: %v", admins, err)
	}
}
