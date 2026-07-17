package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func flashFromResponse(t *testing.T, w *httptest.ResponseRecorder) flash {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "flash" {
			r.AddCookie(cookie)
			return readFlash(httptest.NewRecorder(), r)
		}
	}
	t.Fatal("flash cookie missing")
	return flash{}
}

func requestAs(r *http.Request, role string) *http.Request {
	auth := &authState{User: User{Email: role + "@example.com", Name: "Teste", Role: role}, CSRF: "csrf"}
	return r.WithContext(context.WithValue(r.Context(), authKey{}, auth))
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "test.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}
