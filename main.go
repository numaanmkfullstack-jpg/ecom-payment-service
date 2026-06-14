package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
)

var redisClient *redis.Client
var ctx = context.Background()

// =====================
// DATA MODELS
// =====================

type PaymentRequest struct {
	OrderId       string  `json:"orderId"`
	Amount        float64 `json:"amount"`
	PaymentMethod string  `json:"paymentMethod"`
}

type PaymentResponse struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

// =====================
// MIDDLEWARE (LOGGING)
// =====================

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		log.Printf("➡ %s %s", r.Method, r.URL.Path)

		next.ServeHTTP(w, r)

		log.Printf("✔ %s completed in %v", r.URL.Path, time.Since(start))
	})
}

// =====================
// PAYMENT LOGIC
// =====================

func validatePayment(w http.ResponseWriter, r *http.Request) {

	var req PaymentRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Println("❌ Invalid request body:", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	log.Printf("💳 Processing payment | orderId=%s | amount=%.2f | method=%s",
		req.OrderId, req.Amount, req.PaymentMethod)

	// Business logic
	response := PaymentResponse{Approved: true}

	if req.Amount > 10000 {
		response = PaymentResponse{
			Approved: false,
			Reason:   "Amount exceeds limit",
		}
	}

	// Redis idempotency key
	key := fmt.Sprintf("payment:%s", req.OrderId)

	err := redisClient.Set(ctx, key, response.Approved, 0).Err()
	if err != nil {
		log.Printf("⚠ Redis error: %v", err)
	}

	log.Printf("📦 Payment result | orderId=%s | approved=%v",
		req.OrderId, response.Approved)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// =====================
// MAIN
// =====================

func main() {

	// Redis config
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:6379", redisHost),
		Password: "",
		DB:       0,
	})

	// Redis health check
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Redis connection failed: %v", err)
	}

	log.Println("✅ Redis connected successfully")

	// Router
	r := mux.NewRouter()

	r.HandleFunc("/validate", validatePayment).Methods("POST")

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"status": "UP",
			"service": "payment-service",
		})
	}).Methods("GET")

	// Apply middleware
	r.Use(loggingMiddleware)

	// Start server
	PORT := "3003"
	log.Printf("🚀 Payment service running on :%s", PORT)

	log.Fatal(http.ListenAndServe(":"+PORT, r))
}