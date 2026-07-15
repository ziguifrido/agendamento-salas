package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Room struct {
	ID, Capacity                           int
	Name, Description, Location, Resources string
}
type Booking struct {
	ID, RoomID                                         int
	Room, Owner, Title, Description, Day, Starts, Ends string
}
type App struct {
	db        *sql.DB
	templates *template.Template
	log       *slog.Logger
}
type flash struct {
	Message, Error string
	Form           Booking
}

func main() {
	path := os.Getenv("DATABASE_PATH")
	if path == "" {
		path = "data/reservas.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		panic(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := migrate(db); err != nil {
		panic(err)
	}
	t := template.Must(template.New("").Funcs(template.FuncMap{"now": func() string { return time.Now().Format("2006-01-02") }, "dateBR": dateBR}).ParseGlob("web/templates/*.html"))
	a := &App{db: db, templates: t, log: slog.New(slog.NewJSONHandler(os.Stdout, nil))}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/templates/static"))))
	mux.HandleFunc("/", a.dashboard)
	mux.HandleFunc("/rooms", a.rooms)
	mux.HandleFunc("/agenda/", a.navigateAgenda)
	mux.HandleFunc("/bookings", a.bookings)
	mux.HandleFunc("/bookings/", a.bookingAction)
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	a.log.Info("server started", "address", addr)
	if err := http.ListenAndServe(addr, security(mux)); err != nil {
		a.log.Error("server stopped", "error", err)
	}
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS rooms (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '', capacity INTEGER NOT NULL CHECK(capacity > 0), location TEXT NOT NULL DEFAULT '', resources TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS bookings (id INTEGER PRIMARY KEY, room_id INTEGER NOT NULL REFERENCES rooms(id), owner TEXT NOT NULL, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', day TEXT NOT NULL, starts TEXT NOT NULL, ends TEXT NOT NULL, CHECK(starts < ends));
CREATE INDEX IF NOT EXISTS booking_room_time ON bookings(room_id, day, starts, ends); CREATE INDEX IF NOT EXISTS booking_search ON bookings(day, owner, title);`)
	return err
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self';")
		if r.Method == http.MethodPost && r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host && r.Header.Get("Origin") != "https://"+r.Host {
			http.Error(w, "origem inválida", 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	a.render(w, "dashboard.html", a.data(w, r))
}
func (a *App) navigateAgenda(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	day := agendaDay(r)
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	d, _ := time.Parse("2006-01-02", day)
	switch strings.TrimPrefix(r.URL.Path, "/agenda/") {
	case "previous":
		d = d.AddDate(0, 0, -1)
	case "next":
		d = d.AddDate(0, 0, 1)
	case "today":
		d = time.Now()
	default:
		http.NotFound(w, r)
		return
	}
	setAgendaDay(w, d.Format("2006-01-02"))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (a *App) data(w http.ResponseWriter, r *http.Request) map[string]any {
	day := dateISO(r.URL.Query().Get("day"))
	if day != "" {
		setAgendaDay(w, day)
	} else {
		day = agendaDay(r)
	}
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	f := readFlash(w, r)
	if f.Form.Day != "" {
		day = f.Form.Day
	}
	setAgendaDay(w, day)
	if f.Form.Day == "" {
		f.Form.Day = day
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	return map[string]any{"Day": day, "Form": f.Form, "Query": query, "Rooms": a.roomList(), "Bookings": a.bookingList(day, query), "Message": f.Message, "Error": f.Error}
}
func (a *App) roomList() []Room {
	rows, err := a.db.Query("SELECT id,name,description,capacity,location,resources FROM rooms ORDER BY name")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Room
	for rows.Next() {
		var v Room
		if rows.Scan(&v.ID, &v.Name, &v.Description, &v.Capacity, &v.Location, &v.Resources) == nil {
			out = append(out, v)
		}
	}
	if rows.Err() != nil {
		return nil
	}
	return out
}
func (a *App) bookingList(day, q string) []Booking {
	rows, err := a.db.Query(`SELECT b.id,b.room_id,r.name,b.owner,b.title,b.description,b.day,b.starts,b.ends FROM bookings b JOIN rooms r ON r.id=b.room_id WHERE b.day=? AND (?='' OR r.name LIKE ? OR b.owner LIKE ? OR b.title LIKE ?) ORDER BY b.starts`, day, q, "%"+q+"%", "%"+q+"%", "%"+q+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Booking
	for rows.Next() {
		var v Booking
		if rows.Scan(&v.ID, &v.RoomID, &v.Room, &v.Owner, &v.Title, &v.Description, &v.Day, &v.Starts, &v.Ends) == nil {
			out = append(out, v)
		}
	}
	if rows.Err() != nil {
		return nil
	}
	return out
}
func (a *App) rooms(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		name := strings.TrimSpace(r.FormValue("name"))
		cap := r.FormValue("capacity")
		if name == "" || cap == "" {
			a.redirect(w, r, "", "Preencha nome e capacidade")
			return
		}
		_, err := a.db.Exec("INSERT INTO rooms(name,description,capacity,location,resources) VALUES(?,?,?,?,?)", name, strings.TrimSpace(r.FormValue("description")), cap, strings.TrimSpace(r.FormValue("location")), strings.TrimSpace(r.FormValue("resources")))
		if err != nil {
			a.redirect(w, r, "", "Sala já existe ou capacidade inválida")
			return
		}
		a.redirect(w, r, "Sala criada", "")
		return
	}
	a.render(w, "rooms.html", map[string]any{"Rooms": a.roomList()})
}
func (a *App) bookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	day, starts, ends := dateISO(r.FormValue("day")), r.FormValue("starts"), r.FormValue("ends")
	if !validBooking(day, starts, ends) {
		a.redirect(w, r, "", "Data ou horário inválido")
		return
	}
	room, owner, title := r.FormValue("room_id"), strings.TrimSpace(r.FormValue("owner")), strings.TrimSpace(r.FormValue("title"))
	if room == "" || owner == "" || title == "" {
		a.redirect(w, r, "", "Preencha os campos obrigatórios")
		return
	}
	var conflicts int
	err := a.db.QueryRow("SELECT COUNT(*) FROM bookings WHERE room_id=? AND day=? AND starts < ? AND ends > ?", room, day, ends, starts).Scan(&conflicts)
	if err != nil || conflicts > 0 {
		a.redirect(w, r, "", "Esta sala já está reservada nesse horário")
		return
	}
	_, err = a.db.Exec("INSERT INTO bookings(room_id,owner,title,description,day,starts,ends) VALUES(?,?,?,?,?,?,?)", room, owner, title, strings.TrimSpace(r.FormValue("description")), day, starts, ends)
	if err != nil {
		a.redirect(w, r, "", "Não foi possível criar a reserva")
		return
	}
	a.redirect(w, r, "Reserva criada", "")
}
func (a *App) bookingAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/bookings/")
	if !strings.HasSuffix(id, "/cancel") {
		http.NotFound(w, r)
		return
	}
	id = strings.TrimSuffix(id, "/cancel")
	_, err := a.db.Exec("DELETE FROM bookings WHERE id=?", id)
	if err != nil {
		a.redirect(w, r, "", "Não foi possível cancelar")
		return
	}
	a.redirect(w, r, "Reserva cancelada", "")
}
func validBooking(day, starts, ends string) bool {
	d, e := time.Parse("2006-01-02", day)
	return e == nil && !d.Before(time.Now().Truncate(24*time.Hour)) && len(starts) == 5 && len(ends) == 5 && starts < ends
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
func setFlash(w http.ResponseWriter, f flash) {
	data, _ := json.Marshal(f)
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
func (a *App) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.templates.ExecuteTemplate(w, name, data); err != nil {
		a.log.Error("render", "error", err)
		http.Error(w, "erro interno", 500)
	}
}
