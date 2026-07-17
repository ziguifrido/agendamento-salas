package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	if currentAuth(r) == nil {
		f := readFlash(w, r)
		a.render(w, "login.html", map[string]any{"Error": f.AuthError})
		return
	}
	data, err := a.data(w, r)
	if err != nil {
		a.serverError(w, "load dashboard", err)
		return
	}
	a.render(w, "dashboard.html", data)
}

func (a *App) navigateAgenda(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/agenda/")
	if action == "filter" {
		query := strings.TrimSpace(r.FormValue("q"))
		if utf8.RuneCountInString(query) > maxTitleBytes {
			http.Error(w, "pesquisa muito longa", http.StatusBadRequest)
			return
		}
		day := dateISO(r.FormValue("day"))
		if day == "" {
			day = agendaDay(r)
		}
		if day == "" {
			day = time.Now().Format("2006-01-02")
		}
		view := r.FormValue("view")
		if view != "week" {
			view = "day"
		}
		roomID, err := strconv.Atoi(r.FormValue("room_id"))
		if err != nil || roomID < 0 {
			roomID = 0
		}
		setAgendaDay(w, day)
		setAgendaQuery(w, query)
		setAgendaView(w, view)
		setAgendaRoom(w, roomID)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	day := agendaDay(r)
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	d, _ := time.Parse("2006-01-02", day)
	switch action {
	case "previous":
		d = d.AddDate(0, 0, -1)
	case "next":
		d = d.AddDate(0, 0, 1)
	case "today":
		if today := dateISO(r.FormValue("day")); today != "" {
			d, _ = time.Parse("2006-01-02", today)
		} else {
			d = time.Now()
		}
	default:
		http.NotFound(w, r)
		return
	}
	setAgendaDay(w, d.Format("2006-01-02"))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) data(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	auth := currentAuth(r)
	day := agendaDay(r)
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	f := readFlash(w, r)
	setAgendaDay(w, day)
	if f.Form.Day == "" {
		f.Form.Day = day
	}
	if f.Form.Owner == "" {
		f.Form.Owner = auth.User.Name
	}
	query := agendaQuery(r)
	view := agendaView(r)
	if view == "" {
		view = "day"
	}
	roomID := agendaRoom(r)
	rooms, err := a.roomList()
	if err != nil {
		return nil, err
	}
	data := map[string]any{"Day": day, "View": view, "RoomID": roomID, "Form": f.Form, "Query": query, "Rooms": rooms, "Message": f.Message, "Error": f.Error, "ActionError": f.ActionError, "RoomError": f.RoomError, "RoomDialog": f.RoomDialog, "CurrentUser": auth.User, "IsAdmin": auth.User.Role == roleAdmin, "CSRF": auth.CSRF}
	if view == "week" {
		data["Week"], err = a.weekAgenda(day, roomID, query, auth)
	} else {
		data["Bookings"], err = a.bookingList(day, roomID, query, auth)
	}
	if err != nil {
		return nil, err
	}
	data["Requests"], err = a.bookingRequests(auth)
	if err != nil {
		return nil, err
	}
	if auth.User.Role == roleAdmin {
		data["Users"], err = a.userList()
	}
	return data, err
}

func dateISO(day string) string {
	for _, layout := range []string{"02/01/2006", "2006-01-02"} {
		if d, err := time.Parse(layout, day); err == nil {
			return d.Format("2006-01-02")
		}
	}
	return ""
}

func dateBR(day string) string {
	d, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return d.Format("02/01/2006")
}

func weekdayBR(day string) string {
	d, err := time.Parse("2006-01-02", day)
	if err != nil {
		return ""
	}
	return []string{"Domingo", "Segunda-feira", "Terça-feira", "Quarta-feira", "Quinta-feira", "Sexta-feira", "Sábado"}[d.Weekday()]
}

func weekStart(day string) time.Time {
	d, _ := time.Parse("2006-01-02", day)
	offset := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -offset)
}

func (a *App) redirect(w http.ResponseWriter, r *http.Request, msg, problem string) {
	f := flash{Message: msg, Error: problem, Form: Booking{Day: dateISO(r.FormValue("day"))}}
	if f.Form.Day == "" {
		f.Form.Day = time.Now().Format("2006-01-02")
	}
	if problem != "" {
		f.Form.RoomID, _ = strconv.Atoi(r.FormValue("room_id"))
		f.Form.Owner, f.Form.Title, f.Form.Description, f.Form.Starts, f.Form.Ends = r.FormValue("owner"), r.FormValue("title"), r.FormValue("description"), r.FormValue("starts"), r.FormValue("ends")
	}
	setFlash(w, f)
	setAgendaDay(w, f.Form.Day)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) redirectRoom(w http.ResponseWriter, r *http.Request, msg, problem string) {
	dialog := ""
	if problem != "" {
		dialog = "create"
	}
	setFlash(w, flash{Message: msg, RoomError: problem, RoomDialog: dialog})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) redirectManage(w http.ResponseWriter, r *http.Request, msg, problem string) {
	dialog := ""
	if problem != "" {
		dialog = "manage"
	}
	setFlash(w, flash{Message: msg, RoomError: problem, RoomDialog: dialog})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) redirectAction(w http.ResponseWriter, r *http.Request, msg, problem string) {
	setFlash(w, flash{Message: msg, ActionError: problem})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func setFlash(w http.ResponseWriter, f flash) {
	data, _ := json.Marshal(f)
	if len(data) > maxFlashBytes {
		f.Form = Booking{}
		data, _ = json.Marshal(f)
	}
	http.SetCookie(w, &http.Cookie{Name: "flash", Value: base64.RawURLEncoding.EncodeToString(data), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func readFlash(w http.ResponseWriter, r *http.Request) (f flash) {
	c, err := r.Cookie("flash")
	if err != nil {
		return
	}
	data, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err == nil {
		json.Unmarshal(data, &f)
	}
	http.SetCookie(w, &http.Cookie{Name: "flash", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	return
}

func setAgendaDay(w http.ResponseWriter, day string) {
	http.SetCookie(w, &http.Cookie{Name: "agenda_day", Value: day, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func agendaDay(r *http.Request) string {
	c, err := r.Cookie("agenda_day")
	if err != nil {
		return ""
	}
	return dateISO(c.Value)
}

func setAgendaView(w http.ResponseWriter, view string) {
	http.SetCookie(w, &http.Cookie{Name: "agenda_view", Value: view, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func agendaView(r *http.Request) string {
	c, err := r.Cookie("agenda_view")
	if err != nil {
		return ""
	}
	return c.Value
}

func setAgendaRoom(w http.ResponseWriter, roomID int) {
	http.SetCookie(w, &http.Cookie{Name: "agenda_room", Value: strconv.Itoa(roomID), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func agendaRoom(r *http.Request) int {
	c, err := r.Cookie("agenda_room")
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(c.Value)
	return n
}

func setAgendaQuery(w http.ResponseWriter, query string) {
	value := base64.RawURLEncoding.EncodeToString([]byte(query))
	http.SetCookie(w, &http.Cookie{Name: "agenda_query", Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func agendaQuery(r *http.Request) string {
	c, err := r.Cookie("agenda_query")
	if err != nil {
		return ""
	}
	value, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return ""
	}
	return string(value)
}

func (a *App) render(w http.ResponseWriter, name string, data any) {
	var page bytes.Buffer
	if err := a.templates.ExecuteTemplate(&page, name, data); err != nil {
		a.serverError(w, "render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page.WriteTo(w)
}
