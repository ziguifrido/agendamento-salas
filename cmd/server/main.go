package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
type AgendaDay struct {
	Day      string
	Bookings []Booking
}
type App struct {
	db          *sql.DB
	templates   *template.Template
	log         *slog.Logger
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}
type flash struct {
	Message, Error, RoomError, RoomDialog string
	Form                                  Booking
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
	t := template.Must(template.New("").Funcs(template.FuncMap{"now": func() string { return time.Now().Format("2006-01-02") }, "dateBR": dateBR, "weekdayBR": weekdayBR}).ParseGlob("web/templates/*.html"))
	a := &App{db: db, templates: t, log: slog.New(slog.NewJSONHandler(os.Stdout, nil))}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/templates/static"))))
	mux.HandleFunc("/", a.dashboard)
	mux.HandleFunc("/rooms", a.rooms)
	mux.HandleFunc("/rooms/", a.roomAction)
	mux.HandleFunc("/agenda/", a.navigateAgenda)
	mux.HandleFunc("/bookings", a.bookings)
	mux.HandleFunc("/bookings/", a.bookingAction)
	mux.HandleFunc("/events", a.events)
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
	view := r.URL.Query().Get("view")
	if view == "day" || view == "week" {
		setAgendaView(w, view)
	} else {
		view = agendaView(r)
	}
	if view == "" {
		view = "day"
	}
	roomID, _ := strconv.Atoi(r.URL.Query().Get("room_id"))
	if r.URL.Query().Has("room_id") {
		setAgendaRoom(w, roomID)
	} else {
		roomID = agendaRoom(r)
	}
	data := map[string]any{"Day": day, "View": view, "RoomID": roomID, "Form": f.Form, "Query": query, "Rooms": a.roomList(), "Bookings": a.bookingList(day, roomID, query), "Message": f.Message, "Error": f.Error, "RoomError": f.RoomError, "RoomDialog": f.RoomDialog}
	if view == "week" {
		data["Week"] = a.weekAgenda(day, roomID, query)
	}
	return data
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
func (a *App) bookingList(day string, roomID int, q string) []Booking {
	rows, err := a.db.Query(`SELECT b.id,b.room_id,r.name,b.owner,b.title,b.description,b.day,b.starts,b.ends FROM bookings b JOIN rooms r ON r.id=b.room_id WHERE b.day=? AND (?=0 OR b.room_id=?) AND (?='' OR r.name LIKE ? OR b.owner LIKE ? OR b.title LIKE ?) ORDER BY b.starts`, day, roomID, roomID, q, "%"+q+"%", "%"+q+"%", "%"+q+"%")
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
func (a *App) weekAgenda(day string, roomID int, q string) []AgendaDay {
	start := weekStart(day)
	out := make([]AgendaDay, 5)
	for i := range out {
		out[i].Day = start.AddDate(0, 0, i).Format("2006-01-02")
	}
	rows, err := a.db.Query(`SELECT b.id,b.room_id,r.name,b.owner,b.title,b.description,b.day,b.starts,b.ends FROM bookings b JOIN rooms r ON r.id=b.room_id WHERE b.day>=? AND b.day<? AND (?=0 OR b.room_id=?) AND (?='' OR r.name LIKE ? OR b.owner LIKE ? OR b.title LIKE ?) ORDER BY b.day,b.starts`, out[0].Day, start.AddDate(0, 0, 5).Format("2006-01-02"), roomID, roomID, q, "%"+q+"%", "%"+q+"%", "%"+q+"%")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var b Booking
		if rows.Scan(&b.ID, &b.RoomID, &b.Room, &b.Owner, &b.Title, &b.Description, &b.Day, &b.Starts, &b.Ends) == nil {
			for i := range out {
				if out[i].Day == b.Day {
					out[i].Bookings = append(out[i].Bookings, b)
					break
				}
			}
		}
	}
	if rows.Err() != nil {
		return out
	}
	return out
}
func (a *App) rooms(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		name := strings.TrimSpace(r.FormValue("name"))
		cap := r.FormValue("capacity")
		if name == "" || cap == "" {
			a.redirectRoom(w, r, "", "Preencha nome e capacidade")
			return
		}
		_, err := a.db.Exec("INSERT INTO rooms(name,description,capacity,location,resources) VALUES(?,?,?,?,?)", name, strings.TrimSpace(r.FormValue("description")), cap, strings.TrimSpace(r.FormValue("location")), strings.TrimSpace(r.FormValue("resources")))
		if err != nil {
			a.redirectRoom(w, r, "", "Sala já existe ou capacidade inválida")
			return
		}
		a.notify()
		a.redirectRoom(w, r, "Sala criada", "")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (a *App) roomAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/rooms/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "edit":
		name, capacity := strings.TrimSpace(r.FormValue("name")), r.FormValue("capacity")
		if name == "" || capacity == "" {
			a.redirectManage(w, r, "", "Preencha nome e capacidade")
			return
		}
		_, err = a.db.Exec("UPDATE rooms SET name=?,description=?,capacity=?,location=?,resources=? WHERE id=?", name, strings.TrimSpace(r.FormValue("description")), capacity, strings.TrimSpace(r.FormValue("location")), strings.TrimSpace(r.FormValue("resources")), id)
		if err != nil {
			a.redirectManage(w, r, "", "Sala já existe ou capacidade inválida")
			return
		}
		a.notify()
		a.redirectManage(w, r, "Sala atualizada", "")
	case "delete":
		_, err = a.db.Exec("DELETE FROM rooms WHERE id=?", id)
		if err != nil {
			a.redirectManage(w, r, "", "Não é possível excluir uma sala com agendamentos")
			return
		}
		a.notify()
		a.redirectManage(w, r, "Sala excluída", "")
	default:
		http.NotFound(w, r)
	}
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
	a.notify()
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
	a.notify()
	a.redirect(w, r, "Reserva cancelada", "")
}
func (a *App) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := a.subscribe()
	defer a.unsubscribe(ch)
	fmt.Fprint(w, ": conectado\n\n")
	flusher.Flush()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprint(w, "event: change\ndata: {}\n\n")
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}
func (a *App) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	a.mu.Lock()
	if a.subscribers == nil {
		a.subscribers = map[chan struct{}]struct{}{}
	}
	a.subscribers[ch] = struct{}{}
	a.mu.Unlock()
	return ch
}
func (a *App) unsubscribe(ch chan struct{}) {
	a.mu.Lock()
	delete(a.subscribers, ch)
	a.mu.Unlock()
}
func (a *App) notify() {
	a.mu.Lock()
	for ch := range a.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	a.mu.Unlock()
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
func (a *App) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.templates.ExecuteTemplate(w, name, data); err != nil {
		a.log.Error("render", "error", err)
		http.Error(w, "erro interno", 500)
	}
}
