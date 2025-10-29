package handlers

import (
	"database/sql"
	"html/template"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	DB *sql.DB
}

// --- Главная страница ---
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	rows, _ := h.DB.Query("SELECT id, title, price, image_url FROM games")
	defer rows.Close()

	type Game struct {
		ID       int
		Title    string
		Price    float64
		ImageURL string
	}
	var games []Game
	for rows.Next() {
		var g Game
		rows.Scan(&g.ID, &g.Title, &g.Price, &g.ImageURL)
		games = append(games, g)
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/index.html",
	))
	tmpl.ExecuteTemplate(w, "index.html", games)
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
	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/cart.html",
	))
	tmpl.ExecuteTemplate(w, "cart.html", games)
}

// --- Оплата (оформление покупки) ---
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("cart")
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}
	ids := split(cookie.Value)
	// Добавляем покупку в историю
	res, _ := h.DB.Exec("INSERT INTO purchases (customer_id, total) VALUES (?, ?)", 1, 100.0)
	purchaseID, _ := res.LastInsertId()
	for _, id := range ids {
		h.DB.Exec("INSERT INTO purchase_items (purchase_id, game_id) VALUES (?, ?)", purchaseID, id)
		h.DB.Exec("INSERT INTO library (customer_id, game_id, purchase_id) VALUES (?, ?, ?)", 1, id, purchaseID)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "cart",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	tmpl := template.Must(template.ParseFiles(
		"templates/header.html",
		"templates/pay.html",
	))
	tmpl.ExecuteTemplate(w, "pay.html", map[string]interface{}{
		"PurchaseID": purchaseID,
		"Email":      "user@example.com",
	})
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
