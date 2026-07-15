package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	(&App{db: db}).roomAction(w, httptest.NewRequest(http.MethodPost, "/rooms/1/delete", nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("got status %d", w.Code)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&count); err != nil || count != 0 {
		t.Fatalf("room was not deleted: %d, %v", count, err)
	}
}

func TestValidBooking(t *testing.T) {
	if !validBooking("2999-01-01", "09:00", "10:00") {
		t.Fatal("valid booking rejected")
	}
	for _, test := range [][3]string{{"2999-01-01", "10:00", "10:00"}, {"2000-01-01", "09:00", "10:00"}, {"bad", "09:00", "10:00"}} {
		if validBooking(test[0], test[1], test[2]) {
			t.Fatalf("invalid booking accepted: %v", test)
		}
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
	r := httptest.NewRequest("POST", "/agenda/next", nil)
	r.AddCookie(&http.Cookie{Name: "agenda_day", Value: "2026-07-14"})
	w := httptest.NewRecorder()
	(&App{}).navigateAgenda(w, r)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Set-Cookie"), "2026-07-15") {
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
