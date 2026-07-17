package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

func (a *App) roomList() ([]Room, error) {
	rows, err := a.db.Query("SELECT id,name,description,capacity,location,resources FROM rooms ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Room
	for rows.Next() {
		var v Room
		if err := rows.Scan(&v.ID, &v.Name, &v.Description, &v.Capacity, &v.Location, &v.Resources); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (a *App) rooms(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
	if r.Method == http.MethodPost {
		name := strings.TrimSpace(r.FormValue("name"))
		capacity, err := strconv.Atoi(r.FormValue("capacity"))
		description := strings.TrimSpace(r.FormValue("description"))
		location := strings.TrimSpace(r.FormValue("location"))
		resources := strings.TrimSpace(r.FormValue("resources"))
		if name == "" || capacity < 1 {
			a.redirectRoom(w, r, "", "Preencha nome e capacidade")
			return
		}
		if !validFields(field{name, maxNameBytes}, field{description, maxTextBytes}, field{location, maxDetailBytes}, field{resources, maxDetailBytes}) {
			a.redirectRoom(w, r, "", "Um ou mais campos excedem o tamanho permitido")
			return
		}
		_, err = a.db.Exec("INSERT INTO rooms(name,description,capacity,location,resources) VALUES(?,?,?,?,?)", name, description, capacity, location, resources)
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
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
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
		name := strings.TrimSpace(r.FormValue("name"))
		capacity, capacityErr := strconv.Atoi(r.FormValue("capacity"))
		description := strings.TrimSpace(r.FormValue("description"))
		location := strings.TrimSpace(r.FormValue("location"))
		resources := strings.TrimSpace(r.FormValue("resources"))
		if name == "" || capacityErr != nil || capacity < 1 {
			a.redirectManage(w, r, "", "Preencha nome e capacidade")
			return
		}
		if !validFields(field{name, maxNameBytes}, field{description, maxTextBytes}, field{location, maxDetailBytes}, field{resources, maxDetailBytes}) {
			a.redirectManage(w, r, "", "Um ou mais campos excedem o tamanho permitido")
			return
		}
		var result sql.Result
		result, err = a.db.Exec("UPDATE rooms SET name=?,description=?,capacity=?,location=?,resources=? WHERE id=?", name, description, capacity, location, resources, id)
		if err != nil {
			a.redirectManage(w, r, "", "Sala já existe ou capacidade inválida")
			return
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			a.redirectManage(w, r, "", "Sala não encontrada")
			return
		}
		a.notify()
		a.redirectManage(w, r, "Sala atualizada", "")
	case "delete":
		var result sql.Result
		result, err = a.db.ExecContext(r.Context(), "DELETE FROM rooms WHERE id=?", id)
		if err != nil {
			a.redirectManage(w, r, "", "Não é possível excluir uma sala com agendamentos")
			return
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			a.redirectManage(w, r, "", "Sala não encontrada")
			return
		}
		a.notify()
		a.redirectManage(w, r, "Sala excluída", "")
	default:
		http.NotFound(w, r)
	}
}
