package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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

func TestValidFieldsCountsUnicodeCharacters(t *testing.T) {
	if !validFields(field{strings.Repeat("á", maxTitleBytes), maxTitleBytes}) {
		t.Fatal("valid Unicode field rejected")
	}
	if validFields(field{strings.Repeat("á", maxTitleBytes+1), maxTitleBytes}) {
		t.Fatal("oversized Unicode field accepted")
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
			r := requestAs(httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(form.Encode())).WithContext(ctx), roleAdmin)
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

func TestPendingBookingDoesNotBlockAndConflictingApprovalFails(t *testing.T) {
	db := testDB(t)
	db.Exec("INSERT INTO rooms(name,capacity) VALUES('Sala 1',10)")
	a := &App{db: db}
	form := url.Values{"room_id": {"1"}, "owner": {"Usuário"}, "title": {"Planejamento"}, "day": {"2999-01-01"}, "starts": {"09:00"}, "ends": {"10:00"}}
	request := func(role string) *httptest.ResponseRecorder {
		r := requestAs(httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(form.Encode())), role)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		a.bookings(w, r)
		return w
	}
	if response := flashFromResponse(t, request(roleUser)); response.Message != "Solicitação de reserva enviada para aprovação." {
		t.Fatalf("unexpected user response: %+v", response)
	}
	if response := flashFromResponse(t, request(roleAdmin)); response.Message != "Reserva criada" {
		t.Fatalf("pending booking blocked admin: %+v", response)
	}
	w := httptest.NewRecorder()
	a.bookingAction(w, requestAs(httptest.NewRequest(http.MethodPost, "/bookings/1/approve", nil), roleAdmin))
	if response := flashFromResponse(t, w); response.ActionError == "" {
		t.Fatal("conflicting approval accepted")
	}
	var pending, approved int
	if err := db.QueryRow("SELECT COUNT(*) FILTER (WHERE status='pending'),COUNT(*) FILTER (WHERE status='approved') FROM bookings").Scan(&pending, &approved); err != nil || pending != 1 || approved != 1 {
		t.Fatalf("unexpected booking states: pending=%d approved=%d err=%v", pending, approved, err)
	}
	form.Set("starts", "10:00")
	form.Set("ends", "11:00")
	flashFromResponse(t, request(roleUser))
	w = httptest.NewRecorder()
	a.bookingAction(w, requestAs(httptest.NewRequest(http.MethodPost, "/bookings/3/approve", nil), roleAdmin))
	if response := flashFromResponse(t, w); response.Message != "Reserva aprovada." {
		t.Fatalf("non-conflicting approval failed: %+v", response)
	}
	form.Set("starts", "11:00")
	form.Set("ends", "12:00")
	flashFromResponse(t, request(roleUser))
	w = httptest.NewRecorder()
	a.bookingAction(w, requestAs(httptest.NewRequest(http.MethodPost, "/bookings/4/reject", nil), roleAdmin))
	if response := flashFromResponse(t, w); response.Message != "Solicitação rejeitada." {
		t.Fatalf("rejection failed: %+v", response)
	}
	form.Set("title", "Alteração indevida")
	r := requestAs(httptest.NewRequest(http.MethodPost, "/bookings/4/edit", strings.NewReader(form.Encode())), roleAdmin)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	a.bookingAction(w, r)
	if response := flashFromResponse(t, w); response.ActionError == "" {
		t.Fatal("rejected booking was edited")
	}
	var title string
	if err := db.QueryRow("SELECT title FROM bookings WHERE id=4").Scan(&title); err != nil || title == "Alteração indevida" {
		t.Fatalf("rejected booking changed: title=%q err=%v", title, err)
	}
}

func TestConcurrentApprovalsKeepSingleBooking(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec(`INSERT INTO rooms(name,capacity) VALUES('Sala 1',10);
INSERT INTO bookings(room_id,owner,title,day,starts,ends,requester_email,status) VALUES
(1,'Ana','Primeira','2999-01-01','09:00','10:00','ana@example.com','pending'),
(1,'Bia','Segunda','2999-01-01','09:00','10:00','bia@example.com','pending')`); err != nil {
		t.Fatal(err)
	}
	a := &App{db: db}
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	for id := 1; id <= 2; id++ {
		go func() {
			<-start
			w := httptest.NewRecorder()
			a.bookingAction(w, requestAs(httptest.NewRequest(http.MethodPost, fmt.Sprintf("/bookings/%d/approve", id), nil), roleAdmin))
			responses <- w
		}()
	}
	close(start)
	var succeeded, conflicted int
	for range 2 {
		response := flashFromResponse(t, <-responses)
		if response.Message != "" {
			succeeded++
		}
		if response.ActionError != "" {
			conflicted++
		}
	}
	var approved, pending int
	if err := db.QueryRow("SELECT COUNT(*) FILTER (WHERE status='approved'),COUNT(*) FILTER (WHERE status='pending') FROM bookings").Scan(&approved, &pending); err != nil {
		t.Fatal(err)
	}
	if succeeded != 1 || conflicted != 1 || approved != 1 || pending != 1 {
		t.Fatalf("unexpected concurrent approvals: succeeded=%d conflicted=%d approved=%d pending=%d", succeeded, conflicted, approved, pending)
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
