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
	"time"
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
	PurchaseID  int
	Comments    interface{}
	EditComment interface{} // <--- new: data for edit form
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
		return 0, err
	}
	return id, nil
}

func (h *Handler) renderTemplate(w http.ResponseWriter, tmplFile string, data PageData) {
	// template helper: умножение (поддерживает разные типы)
	mul := func(a, b interface{}) float64 {
		toFloat := func(v interface{}) float64 {
			switch t := v.(type) {
			case float64:
				return t
			case float32:
				return float64(t)
			case int:
				return float64(t)
			case int64:
				return float64(t)
			case uint:
				return float64(t)
			case string:
				if f, err := strconv.ParseFloat(t, 64); err == nil {
					return f
				}
			}
			return 0
		}
		return toFloat(a) * toFloat(b)
	}

	funcs := template.FuncMap{
		"mul": mul,
	}

	tmpl, err := template.New("").Funcs(funcs).ParseGlob("templates/*.html")
	if err != nil {
		log.Printf("renderTemplate: parse error %v", err)
		http.Error(w, "Template parse error", http.StatusInternalServerError)
		return
	}

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

// Ensure DB tables for cart/purchases/library exist
func (h *Handler) ensureSchema() {
	if h == nil || h.DB == nil {
		return
	}

	// helper: does table exist?
	tableExists := func(name string) bool {
		var cnt int
		err := h.DB.QueryRow("SELECT count(name) FROM sqlite_master WHERE type='table' AND name = ?", name).Scan(&cnt)
		if err != nil {
			log.Printf("ensureSchema: tableExists %s check error: %v", name, err)
			return false
		}
		return cnt > 0
	}

	// helper: does column exist in table?
	hasColumn := func(table, col string) bool {
		rows, err := h.DB.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			return false
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err == nil {
				if name == col {
					return true
				}
			}
		}
		return false
	}

	// create or migrate purchases table
	if !tableExists("purchases") {
		_, err := h.DB.Exec(`
            CREATE TABLE IF NOT EXISTS purchases (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                user_id INTEGER,
                total REAL,
                created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
                paid INTEGER DEFAULT 0
            );
        `)
		if err != nil {
			log.Printf("ensureSchema: create purchases error: %v", err)
		}
	} else {
		// add missing columns to purchases
		if !hasColumn("purchases", "user_id") {
			if _, err := h.DB.Exec("ALTER TABLE purchases ADD COLUMN user_id INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column user_id error: %v", err)
			}
		}
		if !hasColumn("purchases", "total") {
			if _, err := h.DB.Exec("ALTER TABLE purchases ADD COLUMN total REAL;"); err != nil {
				log.Printf("ensureSchema: add column total error: %v", err)
			}
		}
		if !hasColumn("purchases", "created_at") {
			if _, err := h.DB.Exec("ALTER TABLE purchases ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP;"); err != nil {
				log.Printf("ensureSchema: add column created_at error: %v", err)
			}
		}
		if !hasColumn("purchases", "paid") {
			if _, err := h.DB.Exec("ALTER TABLE purchases ADD COLUMN paid INTEGER DEFAULT 0;"); err != nil {
				log.Printf("ensureSchema: add column paid error: %v", err)
			}
		}
	}

	// ensure cart_items
	if !tableExists("cart_items") {
		_, err := h.DB.Exec(`
            CREATE TABLE IF NOT EXISTS cart_items (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                user_id INTEGER,
                game_id INTEGER,
                quantity INTEGER DEFAULT 1
            );
        `)
		if err != nil {
			log.Printf("ensureSchema: create cart_items error: %v", err)
		}
	} else {
		if !hasColumn("cart_items", "user_id") {
			if _, err := h.DB.Exec("ALTER TABLE cart_items ADD COLUMN user_id INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column cart_items.user_id error: %v", err)
			}
		}
		if !hasColumn("cart_items", "game_id") {
			if _, err := h.DB.Exec("ALTER TABLE cart_items ADD COLUMN game_id INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column cart_items.game_id error: %v", err)
			}
		}
		if !hasColumn("cart_items", "quantity") {
			if _, err := h.DB.Exec("ALTER TABLE cart_items ADD COLUMN quantity INTEGER DEFAULT 1;"); err != nil {
				log.Printf("ensureSchema: add column cart_items.quantity error: %v", err)
			}
		}
	}

	// ensure purchase_items and required columns
	if !tableExists("purchase_items") {
		_, err := h.DB.Exec(`
            CREATE TABLE IF NOT EXISTS purchase_items (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                purchase_id INTEGER,
                game_id INTEGER,
                price REAL,
                quantity INTEGER
            );
        `)
		if err != nil {
			log.Printf("ensureSchema: create purchase_items error: %v", err)
		}
	} else {
		if !hasColumn("purchase_items", "purchase_id") {
			if _, err := h.DB.Exec("ALTER TABLE purchase_items ADD COLUMN purchase_id INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column purchase_items.purchase_id error: %v", err)
			}
		}
		if !hasColumn("purchase_items", "game_id") {
			if _, err := h.DB.Exec("ALTER TABLE purchase_items ADD COLUMN game_id INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column purchase_items.game_id error: %v", err)
			}
		}
		if !hasColumn("purchase_items", "price") {
			if _, err := h.DB.Exec("ALTER TABLE purchase_items ADD COLUMN price REAL;"); err != nil {
				log.Printf("ensureSchema: add column purchase_items.price error: %v", err)
			}
		}
		if !hasColumn("purchase_items", "quantity") {
			if _, err := h.DB.Exec("ALTER TABLE purchase_items ADD COLUMN quantity INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column purchase_items.quantity error: %v", err)
			}
		}
	}

	// ensure comments table
	if !tableExists("comments") {
		_, err := h.DB.Exec(`
            CREATE TABLE IF NOT EXISTS comments (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                game_id INTEGER,
                user_id INTEGER,
                rating INTEGER,
                text TEXT,
                created_at DATETIME DEFAULT CURRENT_TIMESTAMP
            );
        `)
		if err != nil {
			log.Printf("ensureSchema: create comments error: %v", err)
		}
	} else {
		if !hasColumn("comments", "game_id") {
			if _, err := h.DB.Exec("ALTER TABLE comments ADD COLUMN game_id INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column comments.game_id error: %v", err)
			}
		}
		if !hasColumn("comments", "user_id") {
			if _, err := h.DB.Exec("ALTER TABLE comments ADD COLUMN user_id INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column comments.user_id error: %v", err)
			}
		}
		if !hasColumn("comments", "rating") {
			if _, err := h.DB.Exec("ALTER TABLE comments ADD COLUMN rating INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column comments.rating error: %v", err)
			}
		}
		if !hasColumn("comments", "text") {
			if _, err := h.DB.Exec("ALTER TABLE comments ADD COLUMN text TEXT;"); err != nil {
				log.Printf("ensureSchema: add column comments.text error: %v", err)
			}
		}
		if !hasColumn("comments", "created_at") {
			if _, err := h.DB.Exec("ALTER TABLE comments ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP;"); err != nil {
				log.Printf("ensureSchema: add column comments.created_at error: %v", err)
			}
		}
	}

	// ensure user_games
	if !tableExists("user_games") {
		_, err := h.DB.Exec(`
            CREATE TABLE IF NOT EXISTS user_games (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                user_id INTEGER,
                game_id INTEGER,
                UNIQUE(user_id, game_id)
            );
        `)
		if err != nil {
			log.Printf("ensureSchema: create user_games error: %v", err)
		}
	} else {
		if !hasColumn("user_games", "user_id") {
			if _, err := h.DB.Exec("ALTER TABLE user_games ADD COLUMN user_id INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column user_games.user_id error: %v", err)
			}
		}
		if !hasColumn("user_games", "game_id") {
			if _, err := h.DB.Exec("ALTER TABLE user_games ADD COLUMN game_id INTEGER;"); err != nil {
				log.Printf("ensureSchema: add column user_games.game_id error: %v", err)
			}
		}
	}
}

func (h *Handler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, err := h.getCurrentUser(r)
		if err != nil || uid == 0 {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		// user is authenticated — call next handler
		next(w, r)
	}
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

// GameDetail handler (добавлен сбор комментариев)
func (h *Handler) GameDetail(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
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

	// load comments (join with customers to get username). include user_id to check ownership
	type C struct {
		ID        int
		Rating    int
		Text      string
		AuthorID  int
		Author    string
		CreatedAt string
	}
	var comments []C
	cRows, err := h.DB.Query(`
        SELECT c.id, c.rating, c.text, c.user_id, co.username, c.created_at
        FROM comments c
        LEFT JOIN customers co ON co.id = c.user_id
        WHERE c.game_id = ?
        ORDER BY c.created_at DESC
    `, id)
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var cm C
			if err := cRows.Scan(&cm.ID, &cm.Rating, &cm.Text, &cm.AuthorID, &cm.Author, &cm.CreatedAt); err == nil {
				comments = append(comments, cm)
			}
		}
	} else {
		log.Printf("GameDetail: comments query error: %v", err)
	}

	data := PageData{
		UserID:   uid,
		Username: h.getUsernameByID(uid),
		Game:     g,
		Comments: comments,
	}
	h.renderTemplate(w, "game.html", data)
}

// AddComment — принимает POST с полями id (game_id), rating (1..5), text
func (h *Handler) AddComment(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, err := h.getCurrentUser(r)
	if err != nil || uid == 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	gameIDStr := r.FormValue("id")
	if gameIDStr == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	gameID, err := strconv.Atoi(gameIDStr)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	ratingStr := r.FormValue("rating")
	rating, _ := strconv.Atoi(ratingStr)
	if rating < 1 || rating > 5 {
		rating = 3 // default нормально
	}
	text := r.FormValue("text")
	_, err = h.DB.Exec("INSERT INTO comments (game_id, user_id, rating, text, created_at) VALUES (?, ?, ?, ?, ?)",
		gameID, uid, rating, text, time.Now().Format(time.RFC3339))
	if err != nil {
		log.Printf("AddComment: insert error: %v", err)
	}
	http.Redirect(w, r, "/game?id="+strconv.Itoa(gameID), http.StatusSeeOther)
}

// DeleteComment — удаляет комментарий (только владелец)
func (h *Handler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, err := h.getCurrentUser(r)
	if err != nil || uid == 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	cidStr := r.FormValue("comment_id")
	gidStr := r.FormValue("game_id")
	if cidStr == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	cid, err := strconv.Atoi(cidStr)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// verify owner
	var owner int
	if err := h.DB.QueryRow("SELECT user_id FROM comments WHERE id = ?", cid).Scan(&owner); err != nil {
		log.Printf("DeleteComment: select owner error: %v", err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if owner != uid {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if _, err := h.DB.Exec("DELETE FROM comments WHERE id = ?", cid); err != nil {
		log.Printf("DeleteComment: delete error: %v", err)
	}

	// redirect back to game page
	if gidStr != "" {
		http.Redirect(w, r, "/game?id="+gidStr, http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// EditComment — показывает форму редактирования (только владелец)
func (h *Handler) EditComment(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, err := h.getCurrentUser(r)
	if err != nil || uid == 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	cidStr := r.URL.Query().Get("id")
	if cidStr == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	cid, err := strconv.Atoi(cidStr)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var comment struct {
		ID     int
		GameID int
		Rating int
		Text   string
		UserID int
	}
	if err := h.DB.QueryRow("SELECT id, game_id, rating, text, user_id FROM comments WHERE id = ?", cid).
		Scan(&comment.ID, &comment.GameID, &comment.Rating, &comment.Text, &comment.UserID); err != nil {
		log.Printf("EditComment: select error: %v", err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if comment.UserID != uid {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := PageData{
		UserID:   uid,
		Username: h.getUsernameByID(uid),
		EditComment: map[string]interface{}{
			"ID":     comment.ID,
			"GameID": comment.GameID,
			"Rating": comment.Rating,
			"Text":   comment.Text,
		},
	}
	h.renderTemplate(w, "comment_edit.html", data)
}

// UpdateComment — обрабатывает POST редактирования
func (h *Handler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, err := h.getCurrentUser(r)
	if err != nil || uid == 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	cidStr := r.FormValue("comment_id")
	text := r.FormValue("text")
	ratingStr := r.FormValue("rating")
	if cidStr == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	cid, err := strconv.Atoi(cidStr)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	rating, _ := strconv.Atoi(ratingStr)
	if rating < 1 || rating > 5 {
		rating = 3
	}

	// verify owner and get game_id for redirect
	var owner, gameID int
	if err := h.DB.QueryRow("SELECT user_id, game_id FROM comments WHERE id = ?", cid).Scan(&owner, &gameID); err != nil {
		log.Printf("UpdateComment: select error: %v", err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if owner != uid {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if _, err := h.DB.Exec("UPDATE comments SET rating = ?, text = ?, created_at = ? WHERE id = ?", rating, text, time.Now().Format(time.RFC3339), cid); err != nil {
		log.Printf("UpdateComment: update error: %v", err)
	}

	http.Redirect(w, r, "/game?id="+strconv.Itoa(gameID), http.StatusSeeOther)
}

// Stubs for other handlers (you can expand them as needed)
func (h *Handler) Admin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (h *Handler) AddGame(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// AddToCart — добавляет игру в корзину (увеличивает количество, если уже есть)
func (h *Handler) AddToCart(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, err := h.getCurrentUser(r)
	if err != nil || uid == 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	idStr := r.FormValue("id")
	if idStr == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	gameID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	qty := 1
	if q := r.FormValue("quantity"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			qty = v
		}
	}

	// upsert: если есть — увеличить, иначе вставить
	_, err = h.DB.Exec(`
        INSERT INTO cart_items (user_id, game_id, quantity)
        VALUES (?, ?, ?)
        ON CONFLICT(rowid) DO NOTHING
    `, uid, gameID, qty)
	// simpler fallback: try update then insert
	if err != nil {
		// try update existing
		res, _ := h.DB.Exec("UPDATE cart_items SET quantity = quantity + ? WHERE user_id = ? AND game_id = ?", qty, uid, gameID)
		ra, _ := res.RowsAffected()
		if ra == 0 {
			_, _ = h.DB.Exec("INSERT INTO cart_items (user_id, game_id, quantity) VALUES (?, ?, ?)", uid, gameID, qty)
		}
	}
	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

// Cart — показывает корзину (теперь возвращает id записи корзины для корректного удаления)
func (h *Handler) Cart(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, _ := h.getCurrentUser(r)

	// Используем map для совместимости с template (ключи доступны)
	rows, err := h.DB.Query(`
        SELECT c.id, g.id, g.title, g.price, g.image_url, c.quantity
        FROM cart_items c
        JOIN games g ON g.id = c.game_id
        WHERE c.user_id = ?
    `, uid)
	if err != nil {
		log.Printf("Cart: db error %v", err)
		// показываем пустую корзину при ошибке
		data := PageData{UserID: uid, Username: h.getUsernameByID(uid), Games: []interface{}{}}
		h.renderTemplate(w, "cart.html", data)
		return
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var cartID, gameID, qty int
		var title, imageURL string
		var price float64
		if err := rows.Scan(&cartID, &gameID, &title, &price, &imageURL, &qty); err != nil {
			log.Printf("Cart: scan error: %v", err)
			continue
		}
		items = append(items, map[string]interface{}{
			"CartID":   cartID,
			"ID":       gameID,
			"Title":    title,
			"Price":    price,
			"ImageURL": imageURL,
			"Quantity": qty,
		})
	}

	data := PageData{
		UserID:   uid,
		Username: h.getUsernameByID(uid),
		Games:    items,
	}
	h.renderTemplate(w, "cart.html", data)
}

// Checkout — GET показывает форму, POST создаёт заказ и перенаправляет на /pay
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, _ := h.getCurrentUser(r)
	if r.Method == http.MethodGet {
		// собрать текущую корзину и сумму
		type Item struct {
			ID       int
			Title    string
			Price    float64
			Quantity int
		}
		var items []Item
		rows, err := h.DB.Query(`
            SELECT g.id, g.title, g.price, c.quantity
            FROM cart_items c
            JOIN games g ON g.id = c.game_id
            WHERE c.user_id = ?
        `, uid)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var it Item
				if err := rows.Scan(&it.ID, &it.Title, &it.Price, &it.Quantity); err == nil {
					items = append(items, it)
				}
			}
		}
		// compute total
		var total float64
		for _, it := range items {
			total += it.Price * float64(it.Quantity)
		}
		data := PageData{
			UserID:   uid,
			Username: h.getUsernameByID(uid),
			Games:    items,
		}
		// attach total via Recommended as hack (or extend PageData) — лучше добавить поле, но для минимальных изменений используем Username/other
		// We'll pass total in Username? Instead extend PageData — but to keep minimal edits, embed total in Recommended? Better: extend PageData.
		h.renderTemplate(w, "checkout.html", data)
		return
	}

	// POST — создаём запись purchase и purchase_items, очищаем корзину, редирект на /pay?purchase_id=...
	tx, err := h.DB.Begin()
	if err != nil {
		log.Printf("Checkout: tx begin error %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	// calculate total
	rows2, err := tx.Query(`
        SELECT g.id, g.price, c.quantity
        FROM cart_items c
        JOIN games g ON g.id = c.game_id
        WHERE c.user_id = ?
    `, uid)
	if err != nil {
		tx.Rollback()
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows2.Close()
	var total float64
	type cartRow struct {
		gameID int
		price  float64
		qty    int
	}
	var cartRows []cartRow
	for rows2.Next() {
		var gr cartRow
		if err := rows2.Scan(&gr.gameID, &gr.price, &gr.qty); err == nil {
			total += gr.price * float64(gr.qty)
			cartRows = append(cartRows, gr)
		}
	}
	if len(cartRows) == 0 {
		tx.Rollback()
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}
	res, err := tx.Exec("INSERT INTO purchases (user_id, total, paid, created_at) VALUES (?, ?, 0, ?)", uid, total, time.Now().Format(time.RFC3339))
	if err != nil {
		tx.Rollback()
		log.Printf("Checkout: insert purchase error %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	pid, _ := res.LastInsertId()
	for _, cr := range cartRows {
		_, err := tx.Exec("INSERT INTO purchase_items (purchase_id, game_id, price, quantity) VALUES (?, ?, ?, ?)", pid, cr.gameID, cr.price, cr.qty)
		if err != nil {
			tx.Rollback()
			log.Printf("Checkout: insert purchase_items error %v", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
	}
	// clear cart
	_, err = tx.Exec("DELETE FROM cart_items WHERE user_id = ?", uid)
	if err != nil {
		tx.Rollback()
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("Checkout: commit error %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/pay?purchase_id="+strconv.FormatInt(pid, 10), http.StatusSeeOther)
}

// Pay — мок-оплата: отмечаем purchase как оплаченный, добавляем игры в библиотеку
func (h *Handler) Pay(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, _ := h.getCurrentUser(r)
	pidStr := r.FormValue("purchase_id")
	if pidStr == "" {
		pidStr = r.URL.Query().Get("purchase_id")
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid == 0 {
		http.Redirect(w, r, "/purchases", http.StatusSeeOther)
		return
	}

	// verify purchase belongs to user
	var owner int
	err = h.DB.QueryRow("SELECT user_id FROM purchases WHERE id = ?", pid).Scan(&owner)
	if err != nil || owner != uid {
		http.Redirect(w, r, "/purchases", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		data := PageData{
			UserID:     uid,
			Username:   h.getUsernameByID(uid),
			PurchaseID: pid, // передаём в шаблон
		}
		h.renderTemplate(w, "pay.html", data)
		return
	}

	// POST -> mark paid and add to user_games
	tx, err := h.DB.Begin()
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	_, err = tx.Exec("UPDATE purchases SET paid = 1 WHERE id = ?", pid)
	if err != nil {
		tx.Rollback()
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	// select items
	rows, err := tx.Query("SELECT game_id, quantity FROM purchase_items WHERE purchase_id = ?", pid)
	if err != nil {
		tx.Rollback()
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var gid, qty int
		if err := rows.Scan(&gid, &qty); err == nil {
			// insert into user_games (ignore duplicates)
			_, _ = tx.Exec("INSERT OR IGNORE INTO user_games (user_id, game_id) VALUES (?, ?)", uid, gid)
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/library", http.StatusSeeOther)
}

// Purchases — история пользователя
func (h *Handler) Purchases(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, _ := h.getCurrentUser(r)

	type P struct {
		ID    int
		Date  string
		Total float64
	}
	var out []P
	rows, err := h.DB.Query("SELECT id, created_at, total FROM purchases WHERE user_id = ? ORDER BY created_at DESC", uid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p P
			if err := rows.Scan(&p.ID, &p.Date, &p.Total); err == nil {
				out = append(out, p)
			}
		}
	} else {
		log.Printf("Purchases: db error %v", err)
	}
	data := PageData{
		UserID:    uid,
		Username:  h.getUsernameByID(uid),
		Purchases: out,
	}
	h.renderTemplate(w, "orders.html", data)
}

// Library — список купленных игр
func (h *Handler) Library(w http.ResponseWriter, r *http.Request) {
	h.ensureSchema()
	uid, _ := h.getCurrentUser(r)

	type G struct {
		ID    int
		Title string
	}
	var libs []G
	rows, err := h.DB.Query(`
        SELECT g.id, g.title
        FROM user_games ug
        JOIN games g ON g.id = ug.game_id
        WHERE ug.user_id = ?
    `, uid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var g G
			if err := rows.Scan(&g.ID, &g.Title); err == nil {
				libs = append(libs, g)
			}
		}
	} else {
		log.Printf("Library: db error %v", err)
	}
	data := PageData{
		UserID:   uid,
		Username: h.getUsernameByID(uid),
		Games:    libs,
	}
	h.renderTemplate(w, "library.html", data)
}

// Account — страница аккаунта: показывает покупки и рекомендации
func (h *Handler) Account(w http.ResponseWriter, r *http.Request) {
	uid, err := h.getCurrentUser(r)
	if err != nil || uid == 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	type Purchase struct {
		ID    int
		Date  string
		Total float64
	}
	var purchases []Purchase
	rows, err := h.DB.Query("SELECT id, created_at, total FROM purchases WHERE user_id = ? ORDER BY created_at DESC", uid)
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

func (h *Handler) Static(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/static/", http.FileServer(http.Dir("static"))).ServeHTTP(w, r)
}

// getUsernameByID возвращает имя пользователя по его id или пустую строку, если не найден.
// Используется в PageData.Username перед рендером шаблонов.
func (h *Handler) getUsernameByID(id int) string {
	if id == 0 || h == nil || h.DB == nil {
		return ""
	}
	var username string
	err := h.DB.QueryRow("SELECT username FROM customers WHERE id = ?", id).Scan(&username)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("getUsernameByID: db error for id=%d: %v", id, err)
		}
		return ""
	}
	return username
}

func (h *Handler) RemoveFromCart(w http.ResponseWriter, r *http.Request) {
	uid, err := h.getCurrentUser(r)
	if err != nil || uid == 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Попробуем сначала cart_id (рекомендуемый путь)
	cartIDStr := r.FormValue("cart_id")
	if cartIDStr == "" {
		// допускаем также query параметр
		cartIDStr = r.URL.Query().Get("cart_id")
	}
	if cartIDStr != "" {
		cid, err := strconv.Atoi(cartIDStr)
		if err == nil && cid > 0 {
			_, err = h.DB.Exec("DELETE FROM cart_items WHERE id = ? AND user_id = ?", cid, uid)
			if err != nil {
				log.Printf("RemoveFromCart (by cart_id): db error: %v", err)
			}
			http.Redirect(w, r, "/cart", http.StatusSeeOther)
			return
		}
	}

	// Fallback — удалить по game_id
	idStr := r.FormValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	if idStr == "" {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}
	gid, err := strconv.Atoi(idStr)
	if err != nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	_, err = h.DB.Exec("DELETE FROM cart_items WHERE user_id = ? AND game_id = ?", uid, gid)
	if err != nil {
		log.Printf("RemoveFromCart (by game_id): db error: %v", err)
	}
	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}
