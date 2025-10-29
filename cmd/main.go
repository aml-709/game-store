package main

import (
	"github.com/aml-709/game-store/internal/handlers"
	"github.com/aml-709/game-store/internal/storage"
	"net/http"
)

func main() {
	db := storage.InitDB()
	h := &handlers.Handler{DB: db}

	http.HandleFunc("/", h.Home)
	http.HandleFunc("/game", h.GameDetail)
	http.HandleFunc("/admin", h.Admin)
	http.HandleFunc("/add-game", h.AddGame)
	http.HandleFunc("/add-to-cart", h.AddToCart)
	http.HandleFunc("/cart", h.Cart)
	http.HandleFunc("/checkout", h.Checkout)
	http.HandleFunc("/library", h.Library)
	http.HandleFunc("/purchases", h.Purchases)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	println("Server running on http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
