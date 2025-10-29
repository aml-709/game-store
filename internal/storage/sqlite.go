package storage

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"log"
)

func InitDB() *sql.DB {
	db, err := sql.Open("sqlite", "games.db")
	if err != nil {
		log.Fatal(err)
	}

	// --- Таблица игр ---
	createGames := `
	CREATE TABLE IF NOT EXISTS games (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		description TEXT,
		price REAL NOT NULL,
		image_url TEXT
	);`
	_, err = db.Exec(createGames)
	if err != nil {
		log.Fatal("Error creating games table:", err)
	}

	// --- Таблица пользователей ---
	createUsers := `
	CREATE TABLE IF NOT EXISTS customers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL
	);`
	_, err = db.Exec(createUsers)
	if err != nil {
		log.Fatal("Error creating customers table:", err)
	}

	// --- Таблица покупок (заказов) ---
	createPurchases := `
	CREATE TABLE IF NOT EXISTS purchases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		customer_id INTEGER,
		total REAL,
		purchase_date DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(customer_id) REFERENCES customers(id)
	);`
	_, err = db.Exec(createPurchases)
	if err != nil {
		log.Fatal("Error creating purchases table:", err)
	}

	// --- Таблица элементов покупок (товары в заказе) ---
	createPurchaseItems := `
	CREATE TABLE IF NOT EXISTS purchase_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		purchase_id INTEGER,
		game_id INTEGER,
		quantity INTEGER DEFAULT 1,
		FOREIGN KEY(purchase_id) REFERENCES purchases(id),
		FOREIGN KEY(game_id) REFERENCES games(id)
	);`
	_, err = db.Exec(createPurchaseItems)
	if err != nil {
		log.Fatal("Error creating purchase_items table:", err)
	}

	// --- Таблица библиотеки купленных игр ---
	createLibrary := `
	CREATE TABLE IF NOT EXISTS library (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		customer_id INTEGER,
		game_id INTEGER,
		purchase_id INTEGER,
		FOREIGN KEY(customer_id) REFERENCES customers(id),
		FOREIGN KEY(game_id) REFERENCES games(id),
		FOREIGN KEY(purchase_id) REFERENCES purchases(id)
	);`
	_, err = db.Exec(createLibrary)
	if err != nil {
		log.Fatal("Error creating library table:", err)
	}

	log.Println("✅ Database initialized successfully")
	return db
}
