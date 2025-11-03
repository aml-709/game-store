package handlers

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"strconv"
)

type Handler struct {
	DB *sql.DB
}

// PageData — универсальная структура, передаваемая в шаблоны.
// Заполняйте нужные поля в обработчиках (Games, Game, Purchases, Recommended и т.д.)
type PageData struct {
	UserID      int
	Username    string
	Games       interface{}
	Game        interface{}
	Purchases   interface{}
	Recommended interface{}
	// можно добавлять поля по мере необходимости
}

func hashPassword(p string) string {
	h := sha256.Sum256([]byte(p))
	return hex.EncodeToString(h[:])
}

func (h *Handler) getCurrentUser(r *http.Request) (int, error) {
	c, err := r.Cookie("user_id")
	if err != nil {
		return 0, err
	}
	id, err := strconv.Atoi(c.Value)
	if err != nil {
		log.Printf("getCurrentUser: invalid user_id cookie value=%q err=%v", c.Value, err)
		return 0, err
	}
	return id, nil
}

func (h *Handler) getUsernameByID(id int) string {
	var username string
	_ = h.DB.QueryRow("SELECT username FROM customers WHERE id = ?", id).Scan(&username)
	return username
}

func (h *Handler) renderTemplate(w http.ResponseWriter, tmplFile string, data PageData) {
	// parse all templates once so {{ template "header.html" . }} works reliably
	tmpl, err := template.ParseGlob("templates/*.html")
	if err != nil {
		log.Printf("renderTemplate: parse error %v", err)
		http.Error(w, "Template parse error", http.StatusInternalServerError)
		return
	}

	// Execute into buffer first to avoid partial writes and double WriteHeader
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, tmplFile, data); err != nil {
		log.Printf("renderTemplate: exec error %v", err)
		http.Error(w, "Template exec error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// Register handler
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		if username == "" || password == "" {
			http.Error(w, "Missing fields", http.StatusBadRequest)
			return
		}
		var exists bool
		_ = h.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM customers WHERE username = ?)", username).Scan(&exists)
		if exists {
			http.Error(w, "Username taken", http.StatusBadRequest)
			return
		}
		_, err := h.DB.Exec("INSERT INTO customers (username, password) VALUES (?, ?)", username, hashPassword(password))
		if err != nil {
			log.Printf("Register: insert error %v", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// GET
	data := PageData{UserID: 0}
	h.renderTemplate(w, "register.html", data)
}

// Login handler
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		var id int
		err := h.DB.QueryRow("SELECT id FROM customers WHERE username = ? AND password = ?", username, hashPassword(password)).Scan(&id)
		if err != nil {
			log.Printf("Login: failed for username=%q err=%v", username, err)
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		// Set cookie explicitly (persist 7 days, HttpOnly, Lax same-site)
		http.SetCookie(w, &http.Cookie{
			Name:     "user_id",
			Value:    strconv.Itoa(id),
			Path:     "/",
			MaxAge:   7 * 24 * 60 * 60,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			// Secure: true, // uncomment when using HTTPS
		})

		log.Printf("Login: user %d logged in, cookie set", id)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// GET
	data := PageData{UserID: 0}
	h.renderTemplate(w, "login.html", data)
}

// Logout handler
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "user_id",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Auth middleware
func (h *Handler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, err := h.getCurrentUser(r)
		if err != nil || uid == 0 {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// Home handler — пример: загружает список игр и передаёт UserID/Username
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	uid, _ := h.getCurrentUser(r)
	rows, err := h.DB.Query("SELECT id, title, price, image_url FROM games ORDER BY id DESC")
	if err != nil {
		log.Printf("Home: db error %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type G struct {
		ID       int
		Title    string
		Price    float64
		ImageURL string
	}
	var games []G
	for rows.Next() {
		var g G
		_ = rows.Scan(&g.ID, &g.Title, &g.Price, &g.ImageURL)
		games = append(games, g)
	}

	data := PageData{
		UserID:   uid,
		Username: h.getUsernameByID(uid),
		Games:    games,
	}
	h.renderTemplate(w, "index.html", data)
}

// GameDetail handler
func (h *Handler) GameDetail(w http.ResponseWriter, r *http.Request) {
	uid, _ := h.getCurrentUser(r)

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var g struct {
		ID          int
		Title       string
		Description string
		Price       float64
		ImageURL    string
	}

	err = h.DB.QueryRow("SELECT id, title, description, price, image_url FROM games WHERE id = ?", id).
		Scan(&g.ID, &g.Title, &g.Description, &g.Price, &g.ImageURL)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("GameDetail: db error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	data := PageData{
		UserID:   uid,
		Username: h.getUsernameByID(uid),
		Game:     g,
	}
	h.renderTemplate(w, "game.html", data)
}

// Stubs for other handlers (you can expand them as needed)
func (h *Handler) Admin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (h *Handler) AddGame(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (h *Handler) AddToCart(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (h *Handler) Cart(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (h *Handler) Library(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (h *Handler) Purchases(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (h *Handler) Account(w http.ResponseWriter, r *http.Request) {
	uid, err := h.getCurrentUser(r)
	if err != nil || uid == 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Забираем историю покупок
	type Purchase struct {
		ID    int
		Date  string
		Total float64
	}
	var purchases []Purchase
	rows, err := h.DB.Query("SELECT id, date, total FROM purchases WHERE user_id = ? ORDER BY date DESC", uid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p Purchase
			if err := rows.Scan(&p.ID, &p.Date, &p.Total); err == nil {
				purchases = append(purchases, p)
			}
		}
	} else {
		log.Printf("Account: purchases query error: %v", err)
	}

	// Рекомендованные — просто последние игры (пример)
	type Rec struct {
		ID       int
		Title    string
		Price    float64
		ImageURL string
	}
	var recs []Rec
	rrows, err := h.DB.Query("SELECT id, title, price, image_url FROM games ORDER BY id DESC LIMIT 6")
	if err == nil {
		defer rrows.Close()
		for rrows.Next() {
			var g Rec
			if err := rrows.Scan(&g.ID, &g.Title, &g.Price, &g.ImageURL); err == nil {
				recs = append(recs, g)
			}
		}
	} else {
		log.Printf("Account: recommended query error: %v", err)
	}

	data := PageData{
		UserID:      uid,
		Username:    h.getUsernameByID(uid),
		Purchases:   purchases,
		Recommended: recs,
	}
	h.renderTemplate(w, "account.html", data)
}
