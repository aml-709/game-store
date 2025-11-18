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
	http.HandleFunc("/account", h.AuthMiddleware(h.Account))
	http.HandleFunc("/cart", h.AuthMiddleware(h.Cart))
	http.HandleFunc("/checkout", h.AuthMiddleware(h.Checkout))
	http.HandleFunc("/library", h.AuthMiddleware(h.Library))
	http.HandleFunc("/purchases", h.AuthMiddleware(h.Purchases))
	http.HandleFunc("/add-to-cart", h.AuthMiddleware(h.AddToCart))
	http.HandleFunc("/remove-from-cart", h.AuthMiddleware(h.RemoveFromCart))
	http.HandleFunc("/game/comment", h.AuthMiddleware(h.AddComment))
	http.HandleFunc("/comment/delete", h.AuthMiddleware(h.DeleteComment))
	http.HandleFunc("/comment/edit", h.AuthMiddleware(h.EditComment))
	http.HandleFunc("/comment/update", h.AuthMiddleware(h.UpdateComment))
	http.HandleFunc("/pay", h.AuthMiddleware(h.Pay))

	// Public routes
	http.HandleFunc("/", h.Home)
	http.HandleFunc("/game", h.GameDetail)

	// Static files (single registration)
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	log.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
