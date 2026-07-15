package main

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

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
