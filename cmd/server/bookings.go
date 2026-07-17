package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *App) bookingList(day string, roomID int, q string, auth *authState) ([]Booking, error) {
	rows, err := a.db.Query(`SELECT b.id,b.room_id,r.name,b.owner,b.title,b.description,b.day,b.starts,b.ends,b.requester_email,b.status FROM bookings b JOIN rooms r ON r.id=b.room_id WHERE b.day=? AND b.status!='rejected' AND (?=0 OR b.room_id=?) AND (?='' OR r.name LIKE ? OR b.owner LIKE ? OR b.title LIKE ?) ORDER BY b.starts`, day, roomID, roomID, q, "%"+q+"%", "%"+q+"%", "%"+q+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Booking
	for rows.Next() {
		var v Booking
		if err := rows.Scan(&v.ID, &v.RoomID, &v.Room, &v.Owner, &v.Title, &v.Description, &v.Day, &v.Starts, &v.Ends, &v.RequesterEmail, &v.Status); err != nil {
			return nil, err
		}
		setBookingPermissions(&v, auth)
		out = append(out, v)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (a *App) weekAgenda(day string, roomID int, q string, auth *authState) ([]AgendaDay, error) {
	start := weekStart(day)
	out := make([]AgendaDay, 5)
	for i := range out {
		out[i].Day = start.AddDate(0, 0, i).Format("2006-01-02")
	}
	rows, err := a.db.Query(`SELECT b.id,b.room_id,r.name,b.owner,b.title,b.description,b.day,b.starts,b.ends,b.requester_email,b.status FROM bookings b JOIN rooms r ON r.id=b.room_id WHERE b.day>=? AND b.day<? AND b.status!='rejected' AND (?=0 OR b.room_id=?) AND (?='' OR r.name LIKE ? OR b.owner LIKE ? OR b.title LIKE ?) ORDER BY b.day,b.starts`, out[0].Day, start.AddDate(0, 0, 5).Format("2006-01-02"), roomID, roomID, q, "%"+q+"%", "%"+q+"%", "%"+q+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.RoomID, &b.Room, &b.Owner, &b.Title, &b.Description, &b.Day, &b.Starts, &b.Ends, &b.RequesterEmail, &b.Status); err != nil {
			return nil, err
		}
		setBookingPermissions(&b, auth)
		for i := range out {
			if out[i].Day == b.Day {
				out[i].Bookings = append(out[i].Bookings, b)
				break
			}
		}
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (a *App) bookingRequests(auth *authState) ([]Booking, error) {
	query := `SELECT b.id,b.room_id,r.name,b.owner,b.title,b.description,b.day,b.starts,b.ends,b.requester_email,b.status
FROM bookings b JOIN rooms r ON r.id=b.room_id WHERE `
	var args []any
	if auth.User.Role == roleAdmin {
		query += "b.status='pending'"
	} else {
		query += "b.requester_email=?"
		args = append(args, auth.User.Email)
	}
	query += " ORDER BY b.day,b.starts"
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bookings []Booking
	for rows.Next() {
		var booking Booking
		if err := rows.Scan(&booking.ID, &booking.RoomID, &booking.Room, &booking.Owner, &booking.Title, &booking.Description, &booking.Day, &booking.Starts, &booking.Ends, &booking.RequesterEmail, &booking.Status); err != nil {
			return nil, err
		}
		setBookingPermissions(&booking, auth)
		bookings = append(bookings, booking)
	}
	return bookings, rows.Err()
}

func setBookingPermissions(booking *Booking, auth *authState) {
	admin := auth.User.Role == roleAdmin
	booking.CanCancel = admin || booking.Status == "pending" && strings.EqualFold(booking.RequesterEmail, auth.User.Email)
	booking.CanReview = admin && booking.Status == "pending"
	booking.CanEdit = admin
}

func (a *App) bookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	auth := currentAuth(r)
	day, starts, ends := dateISO(r.FormValue("day")), r.FormValue("starts"), r.FormValue("ends")
	if !validBooking(day, starts, ends) {
		a.redirect(w, r, "", "Data ou horário inválido")
		return
	}
	room, roomErr := strconv.Atoi(r.FormValue("room_id"))
	owner, title := strings.TrimSpace(r.FormValue("owner")), strings.TrimSpace(r.FormValue("title"))
	description := strings.TrimSpace(r.FormValue("description"))
	if roomErr != nil || room < 1 || owner == "" || title == "" {
		a.redirect(w, r, "", "Preencha os campos obrigatórios")
		return
	}
	if !validFields(field{owner, maxNameBytes}, field{title, maxTitleBytes}, field{description, maxTextBytes}) {
		a.redirect(w, r, "", "Um ou mais campos excedem o tamanho permitido")
		return
	}
	status := "pending"
	if auth.User.Role == roleAdmin {
		status = "approved"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var result sql.Result
	var err error
	if status == "approved" {
		result, err = a.db.ExecContext(r.Context(), `INSERT INTO bookings(room_id,owner,title,description,day,starts,ends,requester_email,status,updated_at)
SELECT ?,?,?,?,?,?,?,?,?,?
WHERE NOT EXISTS (SELECT 1 FROM bookings WHERE status='approved' AND room_id=? AND day=? AND starts < ? AND ends > ?)`, room, owner, title, description, day, starts, ends, auth.User.Email, status, now, room, day, ends, starts)
	} else {
		result, err = a.db.ExecContext(r.Context(), "INSERT INTO bookings(room_id,owner,title,description,day,starts,ends,requester_email,status,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", room, owner, title, description, day, starts, ends, auth.User.Email, status, now)
	}
	if err != nil {
		a.redirect(w, r, "", "Não foi possível criar a reserva")
		return
	}
	if inserted, _ := result.RowsAffected(); status == "approved" && inserted == 0 {
		a.redirect(w, r, "", "Esta sala já está reservada nesse horário")
		return
	}
	a.notify()
	message := "Solicitação de reserva enviada para aprovação."
	if status == "approved" {
		message = "Reserva criada"
	}
	a.redirect(w, r, message, "")
}

func (a *App) bookingAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/bookings/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	auth := currentAuth(r)
	switch parts[1] {
	case "cancel":
		query, args := "DELETE FROM bookings WHERE id=? AND status!='rejected'", []any{id}
		if auth.User.Role != roleAdmin {
			query += " AND status='pending' AND requester_email=?"
			args = append(args, auth.User.Email)
		}
		result, err := a.db.ExecContext(r.Context(), query, args...)
		if err != nil {
			a.redirectAction(w, r, "", "Não foi possível cancelar a reserva.")
			return
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			a.redirectAction(w, r, "", "Reserva não encontrada ou cancelamento não permitido.")
			return
		}
		a.notify()
		a.redirectAction(w, r, "Reserva cancelada.", "")
	case "approve":
		if _, ok := requireAdmin(w, r); !ok {
			return
		}
		result, err := a.db.ExecContext(r.Context(), `UPDATE bookings SET status='approved',updated_at=?
WHERE id=? AND status='pending' AND NOT EXISTS (
SELECT 1 FROM bookings approved WHERE approved.status='approved' AND approved.id!=bookings.id
AND approved.room_id=bookings.room_id AND approved.day=bookings.day
AND approved.starts<bookings.ends AND approved.ends>bookings.starts)`, time.Now().UTC().Format(time.RFC3339), id)
		if err != nil {
			a.redirectAction(w, r, "", "Não foi possível aprovar a reserva.")
			return
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			a.redirectAction(w, r, "", "A solicitação não existe mais ou o horário já foi ocupado.")
			return
		}
		a.notify()
		a.redirectAction(w, r, "Reserva aprovada.", "")
	case "reject":
		if _, ok := requireAdmin(w, r); !ok {
			return
		}
		result, err := a.db.ExecContext(r.Context(), "UPDATE bookings SET status='rejected',updated_at=? WHERE id=? AND status='pending'", time.Now().UTC().Format(time.RFC3339), id)
		if err != nil {
			a.redirectAction(w, r, "", "Não foi possível rejeitar a reserva.")
			return
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			a.redirectAction(w, r, "", "Solicitação pendente não encontrada.")
			return
		}
		a.notify()
		a.redirectAction(w, r, "Solicitação rejeitada.", "")
	case "edit":
		if _, ok := requireAdmin(w, r); !ok {
			return
		}
		a.editBooking(w, r, id)
	default:
		http.NotFound(w, r)
		return
	}
}

func (a *App) editBooking(w http.ResponseWriter, r *http.Request, id int) {
	day, starts, ends := dateISO(r.FormValue("day")), r.FormValue("starts"), r.FormValue("ends")
	room, roomErr := strconv.Atoi(r.FormValue("room_id"))
	owner, title, description := strings.TrimSpace(r.FormValue("owner")), strings.TrimSpace(r.FormValue("title")), strings.TrimSpace(r.FormValue("description"))
	if roomErr != nil || room < 1 || owner == "" || title == "" || !validBooking(day, starts, ends) || !validFields(field{owner, maxNameBytes}, field{title, maxTitleBytes}, field{description, maxTextBytes}) {
		a.redirectAction(w, r, "", "Dados da reserva inválidos.")
		return
	}
	result, err := a.db.ExecContext(r.Context(), `UPDATE bookings SET room_id=?,owner=?,title=?,description=?,day=?,starts=?,ends=?,updated_at=?
WHERE id=? AND status!='rejected' AND (status!='approved' OR NOT EXISTS (
SELECT 1 FROM bookings approved WHERE approved.status='approved' AND approved.id!=bookings.id
AND approved.room_id=? AND approved.day=? AND approved.starts<? AND approved.ends>?))`, room, owner, title, description, day, starts, ends, time.Now().UTC().Format(time.RFC3339), id, room, day, ends, starts)
	if err != nil {
		a.redirectAction(w, r, "", "Não foi possível editar a reserva.")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		a.redirectAction(w, r, "", "Reserva não encontrada ou horário indisponível.")
		return
	}
	a.notify()
	a.redirectAction(w, r, "Reserva atualizada.", "")
}

type field struct {
	value string
	max   int
}

func validFields(fields ...field) bool {
	for _, field := range fields {
		if utf8.RuneCountInString(field.value) > field.max {
			return false
		}
	}
	return true
}

func validBooking(day, starts, ends string) bool {
	d, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return false
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	start, startErr := time.Parse("15:04", starts)
	end, endErr := time.Parse("15:04", ends)
	return !d.Before(today) && startErr == nil && endErr == nil && start.Before(end)
}
