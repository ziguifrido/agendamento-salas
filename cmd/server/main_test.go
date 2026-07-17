package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRoomActionDeletesRoom(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO rooms(name,capacity) VALUES('Sala 1',10)"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	(&App{db: db}).roomAction(w, httptest.NewRequest(http.MethodPost, "/rooms/1/delete", nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("got status %d", w.Code)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&count); err != nil || count != 0 {
		t.Fatalf("room was not deleted: %d, %v", count, err)
	}
}

func TestNotifySignalsSubscribers(t *testing.T) {
	a := &App{}
	ch := a.subscribe()
	a.notify()
	select {
	case <-ch:
	default:
		t.Fatal("subscriber was not notified")
	}
	a.unsubscribe(ch)
	a.notify()
	select {
	case <-ch:
		t.Fatal("unsubscribed channel was notified")
	default:
	}
}

func TestValidBooking(t *testing.T) {
	if !validBooking("2999-01-01", "09:00", "10:00") {
		t.Fatal("valid booking rejected")
	}
	for _, test := range [][3]string{{"2999-01-01", "10:00", "10:00"}, {"2999-01-01", "aa:aa", "zz:zz"}, {"2999-01-01", "09:60", "10:00"}, {"2000-01-01", "09:00", "10:00"}, {"bad", "09:00", "10:00"}} {
		if validBooking(test[0], test[1], test[2]) {
			t.Fatalf("invalid booking accepted: %v", test)
		}
	}
}

func TestSQLiteDSNEnablesForeignKeysOnEveryConnection(t *testing.T) {
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "test.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	first, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for _, conn := range []*sql.Conn{first, second} {
		var enabled int
		if err := conn.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&enabled); err != nil || enabled != 1 {
			t.Fatalf("foreign keys disabled: %d, %v", enabled, err)
		}
	}
}

func TestBookingConflictIsRejected(t *testing.T) {
	dsn := sqliteDSN(filepath.Join(t.TempDir(), "test.db"))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO rooms(name,capacity) VALUES('Sala 1',10)"); err != nil {
		t.Fatal(err)
	}
	otherDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()

	start := make(chan struct{})
	responses := make(chan flash, 2)
	var wg sync.WaitGroup
	for _, app := range []*App{{db: db}, {db: otherDB}} {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			form := url.Values{"room_id": {"1"}, "owner": {"Ana"}, "title": {"Planejamento"}, "day": {"2999-01-01"}, "starts": {"09:00"}, "ends": {"10:00"}}
			r := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(form.Encode())).WithContext(ctx)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			app.bookings(w, r)
			responses <- flashFromResponse(t, w)
		}()
	}
	close(start)
	wg.Wait()
	close(responses)

	var successes, conflicts int
	for response := range responses {
		if response.Message == "Reserva criada" {
			successes++
		}
		if response.Error == "Esta sala já está reservada nesse horário" {
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one conflict: success=%d conflict=%d", successes, conflicts)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM bookings").Scan(&count); err != nil || count != 1 {
		t.Fatalf("expected one booking: %d, %v", count, err)
	}
}

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

func TestDateBR(t *testing.T) {
	if got := dateBR("2026-07-14"); got != "14/07/2026" {
		t.Fatalf("got %q", got)
	}
}

func TestDateISO(t *testing.T) {
	if got := dateISO("14/07/2026"); got != "2026-07-14" {
		t.Fatalf("got %q", got)
	}
}

func TestWeekStart(t *testing.T) {
	if got := weekStart("2026-07-15").Format("2006-01-02"); got != "2026-07-13" {
		t.Fatalf("got %q", got)
	}
}

func TestWeekdayBR(t *testing.T) {
	if got := weekdayBR("2026-07-15"); got != "Quarta-feira" {
		t.Fatalf("got %q", got)
	}
}

func TestNavigateAgenda(t *testing.T) {
	r := httptest.NewRequest("POST", "/agenda/today", strings.NewReader("day=2026-07-16"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "agenda_day", Value: "2026-07-14"})
	w := httptest.NewRecorder()
	(&App{}).navigateAgenda(w, r)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Set-Cookie"), "2026-07-16") {
		t.Fatalf("unexpected navigation: %d %s", w.Code, w.Header().Get("Set-Cookie"))
	}
}

func TestRedirectKeepsBookingFieldsAfterError(t *testing.T) {
	form := url.Values{"room_id": {"2"}, "owner": {"Ana"}, "title": {"Planejamento"}, "description": {"Q3"}, "day": {"2999-01-01"}, "starts": {"10:00"}, "ends": {"09:00"}}
	r := httptest.NewRequest("POST", "/bookings", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	(&App{}).redirect(w, r, "", "Data ou horário inválido")
	if location := w.Result().Header.Get("Location"); location != "/" {
		t.Fatalf("unexpected redirect: %s", location)
	}
	if !strings.Contains(w.Result().Header.Get("Set-Cookie"), "flash=") {
		t.Fatal("flash cookie missing")
	}
}

func TestSecurityRejectsLargeForm(t *testing.T) {
	called := false
	handler := security(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	r := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader("title="+strings.Repeat("a", maxFormBytes)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if called || w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large form accepted: called=%v status=%d", called, w.Code)
	}
}

func TestFlashCookieIsBounded(t *testing.T) {
	w := httptest.NewRecorder()
	setFlash(w, flash{Error: "erro", Form: Booking{Description: strings.Repeat("a", maxFormBytes)}})
	cookie := w.Header().Get("Set-Cookie")
	if len(cookie) >= 4096 {
		t.Fatalf("oversized cookie: %d bytes", len(cookie))
	}
}
