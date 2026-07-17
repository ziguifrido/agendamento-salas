package main

import (
	"database/sql"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	(&App{db: db}).roomAction(w, requestAs(httptest.NewRequest(http.MethodPost, "/rooms/1/delete", nil), roleAdmin))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("got status %d", w.Code)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&count); err != nil || count != 0 {
		t.Fatalf("room was not deleted: %d, %v", count, err)
	}
}

func TestRoomCreationRejectsInvalidCapacity(t *testing.T) {
	db := testDB(t)
	r := requestAs(httptest.NewRequest(http.MethodPost, "/rooms", strings.NewReader("name=Sala&capacity=dez")), roleAdmin)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	(&App{db: db}).rooms(w, r)
	if response := flashFromResponse(t, w); response.RoomError != "Preencha nome e capacidade" {
		t.Fatalf("invalid capacity accepted: %+v", response)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&count); err != nil || count != 0 {
		t.Fatalf("unexpected rooms: %d, %v", count, err)
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

func TestTemplatesParse(t *testing.T) {
	templates, err := template.New("").Funcs(template.FuncMap{"now": func() string { return "2999-01-01" }, "dateBR": dateBR, "weekdayBR": weekdayBR}).ParseGlob("../../web/templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	if err := templates.ExecuteTemplate(io.Discard, "login.html", map[string]any{}); err != nil {
		t.Fatalf("render login: %v", err)
	}
	for _, admin := range []bool{false, true} {
		data := map[string]any{"CurrentUser": User{Name: "Teste"}, "CSRF": "csrf", "IsAdmin": admin, "View": "day", "Form": Booking{}, "Day": "2999-01-01", "Requests": []Booking{}, "Rooms": []Room{}, "Bookings": []Booking{}, "Users": []User{}}
		if err := templates.ExecuteTemplate(io.Discard, "dashboard.html", data); err != nil {
			t.Fatalf("render admin=%v: %v", admin, err)
		}
	}
}

func TestMigrateExistingDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "legacy.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE rooms (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '', capacity INTEGER NOT NULL, location TEXT NOT NULL DEFAULT '', resources TEXT NOT NULL DEFAULT '');
CREATE TABLE bookings (id INTEGER PRIMARY KEY, room_id INTEGER NOT NULL REFERENCES rooms(id), owner TEXT NOT NULL, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', day TEXT NOT NULL, starts TEXT NOT NULL, ends TEXT NOT NULL);
INSERT INTO rooms(name,capacity) VALUES('Legado',10);
INSERT INTO bookings(room_id,owner,title,day,starts,ends) VALUES(1,'Ana','Legado','2999-01-01','09:00','10:00');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("migration is not idempotent: %v", err)
	}
	var status string
	if err := db.QueryRow("SELECT status FROM bookings WHERE id=1").Scan(&status); err != nil || status != "approved" {
		t.Fatalf("legacy booking not approved: %q, %v", status, err)
	}
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

func TestAgendaFilterPersistsCookiesAndRejectsLongQuery(t *testing.T) {
	form := "day=2026-07-16&view=week&room_id=2&q=planejamento"
	r := httptest.NewRequest(http.MethodPost, "/agenda/filter", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	(&App{}).navigateAgenda(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("got status %d", w.Code)
	}
	cookies := w.Result().Cookies()
	request := httptest.NewRequest(http.MethodGet, "/?day=2999-01-01&view=day&room_id=99&q=url", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	if agendaDay(request) != "2026-07-16" || agendaView(request) != "week" || agendaRoom(request) != 2 || agendaQuery(request) != "planejamento" {
		t.Fatal("agenda filter was not persisted in session cookies")
	}

	longQuery := strings.Repeat("á", maxTitleBytes+1)
	r = httptest.NewRequest(http.MethodPost, "/agenda/filter", strings.NewReader("q="+longQuery))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	(&App{}).navigateAgenda(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("long query got status %d", w.Code)
	}
}

func TestHealth(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		health(w, httptest.NewRequest(method, "/healthz", nil))
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s got status %d", method, w.Code)
		}
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
