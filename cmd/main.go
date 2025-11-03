package main

import (
	"log"
	"net/http"

	"github.com/aml-709/game-store/internal/handlers"
	"github.com/aml-709/game-store/internal/storage"
)

func main() {
	db := storage.InitDB()
	h := &handlers.Handler{DB: db}

	// Auth routes
	http.HandleFunc("/register", h.Register)
	http.HandleFunc("/login", h.Login)
	http.HandleFunc("/logout", h.Logout)

	// Protected routes
	http.HandleFunc("/cart", h.AuthMiddleware(h.Cart))
	http.HandleFunc("/checkout", h.AuthMiddleware(h.Checkout))
	http.HandleFunc("/library", h.AuthMiddleware(h.Library))
	http.HandleFunc("/purchases", h.AuthMiddleware(h.Purchases))
	http.HandleFunc("/account", h.AuthMiddleware(h.Account))

	// Public routes
	http.HandleFunc("/", h.Home)
	http.HandleFunc("/game", h.GameDetail)
	http.HandleFunc("/admin", h.Admin)
	http.HandleFunc("/add-game", h.AddGame)
	http.HandleFunc("/add-to-cart", h.AddToCart)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
