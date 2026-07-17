package main

import (
	"database/sql"
	"strings"
)

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	if !strings.HasPrefix(path, "file:") {
		path = "file:" + path
	}
	return path + separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS rooms (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '', capacity INTEGER NOT NULL CHECK(capacity > 0), location TEXT NOT NULL DEFAULT '', resources TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS bookings (id INTEGER PRIMARY KEY, room_id INTEGER NOT NULL REFERENCES rooms(id), owner TEXT NOT NULL, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', day TEXT NOT NULL, starts TEXT NOT NULL, ends TEXT NOT NULL, CHECK(starts < ends));
CREATE TABLE IF NOT EXISTS users (email TEXT PRIMARY KEY COLLATE NOCASE, name TEXT NOT NULL, avatar_url TEXT NOT NULL DEFAULT '', role TEXT NOT NULL CHECK(role IN ('admin','user')), created_at TEXT NOT NULL, updated_at TEXT NOT NULL, last_login TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (token_hash TEXT PRIMARY KEY, user_email TEXT NOT NULL REFERENCES users(email) ON DELETE CASCADE, csrf_token TEXT NOT NULL, expires_at TEXT NOT NULL, created_at TEXT NOT NULL);`); err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"requester_email", "TEXT NOT NULL DEFAULT ''"},
		{"status", "TEXT NOT NULL DEFAULT 'approved' CHECK(status IN ('pending','approved','rejected'))"},
		{"updated_at", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := addColumn(db, "bookings", column.name, column.definition); err != nil {
			return err
		}
	}
	_, err := db.Exec(`CREATE INDEX IF NOT EXISTS booking_room_time ON bookings(room_id, day, starts, ends);
CREATE INDEX IF NOT EXISTS booking_search ON bookings(day, owner, title);
CREATE INDEX IF NOT EXISTS booking_status_day ON bookings(status, day);
CREATE INDEX IF NOT EXISTS session_expiry ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS user_last_login ON users(last_login);`)
	return err
}

func addColumn(db *sql.DB, table, name, definition string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var column, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &column, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if column == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + name + " " + definition)
	return err
}
