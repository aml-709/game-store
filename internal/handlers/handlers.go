package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"html/template"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	DB *sql.DB
}

// --- Auth helpers ---
func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func (h *Handler) getCurrentUser(r *http.Request) (int, error) {
	cookie, err := r.Cookie("user_id")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(cookie.Value)
}

// --- Auth handlers ---
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		username := r.FormValue("username")
		password := r.FormValue("password")

		var exists bool
		err := h.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM customers WHERE username = ?)",
			username).Scan(&exists)
		if err != nil || exists {
			http.Error(w, "Username already taken", http.StatusBadRequest)
			return
		}

		hashedPassword := hashPassword(password)
		_, err = h.DB.Exec("INSERT INTO customers (username, password) VALUES (?, ?)",
			username, hashedPassword)
		if err != nil {
			http.Error(w, "Registration failed", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/register.html",
	))
	tmpl.ExecuteTemplate(w, "register.html", nil)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		username := r.FormValue("username")
		password := r.FormValue("password")
		hashedPassword := hashPassword(password)

		var id int
		err := h.DB.QueryRow("SELECT id FROM customers WHERE username = ? AND password = ?",
			username, hashedPassword).Scan(&id)
		if err != nil {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:  "user_id",
			Value: strconv.Itoa(id),
			Path:  "/",
		})

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/login.html",
	))
	tmpl.ExecuteTemplate(w, "login.html", nil)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "user_id",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- Existing handlers with auth ---

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.getCurrentUser(r)

	rows, _ := h.DB.Query("SELECT id, title, price, image_url FROM games")
	defer rows.Close()

	var games []struct {
		ID       int
		Title    string
		Price    float64
		ImageURL string
	}
	for rows.Next() {
		var g struct {
			ID       int
			Title    string
			Price    float64
			ImageURL string
		}
		rows.Scan(&g.ID, &g.Title, &g.Price, &g.ImageURL)
		games = append(games, g)
	}

	data := struct {
		UserID int
		Games  interface{}
	}{
		UserID: userID,
		Games:  games,
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/index.html",
	))
	tmpl.ExecuteTemplate(w, "index.html", data)
}

// --- Страница игры ---
func (h *Handler) GameDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	row := h.DB.QueryRow("SELECT id, title, description, price, image_url FROM games WHERE id = ?", id)
	var g struct {
		ID          int
		Title       string
		Description string
		Price       float64
		ImageURL    string
	}
	row.Scan(&g.ID, &g.Title, &g.Description, &g.Price, &g.ImageURL)

	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/game.html",
	))
	tmpl.ExecuteTemplate(w, "game.html", g)
}

// --- Админка (форма добавления игры) ---
func (h *Handler) Admin(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/admin.html",
	))
	tmpl.ExecuteTemplate(w, "admin.html", nil)
}

// --- Обработка добавления игры ---
func (h *Handler) AddGame(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		title := r.FormValue("title")
		description := r.FormValue("description")
		price, _ := strconv.ParseFloat(r.FormValue("price"), 64)
		imageURL := r.FormValue("image_url")
		h.DB.Exec("INSERT INTO games (title, description, price, image_url) VALUES (?, ?, ?, ?)", title, description, price, imageURL)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// --- Добавление игры в корзину ---
func (h *Handler) AddToCart(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	cart := r.FormValue("cart")
	if cart != "" {
		cart += "," + id
	} else {
		cart = id
	}
	http.SetCookie(w, &http.Cookie{
		Name:  "cart",
		Value: cart,
		Path:  "/",
	})
	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

// --- Просмотр корзины ---
func (h *Handler) Cart(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.getCurrentUser(r)
	cookie, err := r.Cookie("cart")
	var games []struct {
		ID    int
		Title string
		Price float64
	}
	if err == nil && cookie.Value != "" {
		ids := split(cookie.Value)
		query := "SELECT id, title, price FROM games WHERE id IN (?" + strings.Repeat(",?", len(ids)-1) + ")"
		args := make([]interface{}, len(ids))
		for i, v := range ids {
			args[i] = v
		}
		rows, _ := h.DB.Query(query, args...)
		defer rows.Close()
		for rows.Next() {
			var g struct {
				ID    int
				Title string
				Price float64
			}
			rows.Scan(&g.ID, &g.Title, &g.Price)
			games = append(games, g)
		}
	}

	data := struct {
		UserID int
		Games  interface{}
	}{
		UserID: userID,
		Games:  games,
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/cart.html",
	))
	tmpl.ExecuteTemplate(w, "cart.html", data)
}

// --- Оплата (оформление покупки) ---
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.getCurrentUser(r)
	cookie, err := r.Cookie("cart")
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	ids := split(cookie.Value)
	res, _ := h.DB.Exec("INSERT INTO purchases (customer_id, total) VALUES (?, ?)", userID, 100.0)
	purchaseID, _ := res.LastInsertId()

	for _, id := range ids {
		h.DB.Exec("INSERT INTO purchase_items (purchase_id, game_id) VALUES (?, ?)", purchaseID, id)
		h.DB.Exec("INSERT INTO library (customer_id, game_id, purchase_id) VALUES (?, ?, ?)", userID, id, purchaseID)
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "cart",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	data := struct {
		UserID     int
		PurchaseID int64
		Email      string
	}{
		UserID:     userID,
		PurchaseID: purchaseID,
		Email:      "user@example.com",
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/pay.html",
	))
	tmpl.ExecuteTemplate(w, "pay.html", data)
}

// --- Просмотр библиотеки пользователя ---
func (h *Handler) Library(w http.ResponseWriter, r *http.Request) {
	rows, _ := h.DB.Query("SELECT games.id, games.title FROM library JOIN games ON library.game_id = games.id WHERE library.customer_id = ?", 1)
	defer rows.Close()
	var games []struct {
		ID    int
		Title string
	}
	for rows.Next() {
		var g struct {
			ID    int
			Title string
		}
		rows.Scan(&g.ID, &g.Title)
		games = append(games, g)
	}
	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/library.html",
	))
	tmpl.ExecuteTemplate(w, "library.html", games)
}

// --- Просмотр истории покупок ---
func (h *Handler) Purchases(w http.ResponseWriter, r *http.Request) {
	rows, _ := h.DB.Query("SELECT id, total, purchase_date FROM purchases WHERE customer_id = ?", 1)
	defer rows.Close()
	var purchases []struct {
		ID    int
		Total float64
		Date  string
	}
	for rows.Next() {
		var p struct {
			ID    int
			Total float64
			Date  string
		}
		rows.Scan(&p.ID, &p.Total, &p.Date)
		purchases = append(purchases, p)
	}
	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/orders.html",
	))
	tmpl.ExecuteTemplate(w, "orders.html", purchases)
}

// --- Вспомогательная функция ---
func split(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// AuthMiddleware handles authentication
func (h *Handler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := h.getCurrentUser(r)
		if err != nil || userID == 0 {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}
