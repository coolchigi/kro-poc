package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	 _ "github.com/lib/pq"
)

type Subscription struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	Cost         float64 `json:"cost"`
	BillingCycle string  `json:"billingCycle"`
	NextBilling  string  `json:"nextBilling"`
	Description  string  `json:"description"`
}

var db *sql.DB

func main() {
	var err error
	connStr := "postgres://postgres:postgres@localhost:5432/subscriptions?sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatalf("Error pinging database: %v", err)
	}
	fmt.Println("Successfully connected to database")

	err = initDB()
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}
	fmt.Println("Database tables initialized")
	

	r := mux.NewRouter()
	
	r.HandleFunc("/api/health", healthCheck).Methods("GET")
	r.HandleFunc("/api/dbcheck", dbCheck).Methods("GET")

	r.HandleFunc("/api/subscriptions", getSubscriptions).Methods("GET")
	r.HandleFunc("/api/subscriptions", createSubscription).Methods("POST")
	r.HandleFunc("/api/subscriptions/{id}", getSubscription).Methods("GET")
	r.HandleFunc("/api/subscriptions/{id}", updateSubscription).Methods("PUT")
	r.HandleFunc("/api/subscriptions/{id}", deleteSubscription).Methods("DELETE")

	r.HandleFunc("/api/stats", getStats).Methods("GET")
	
	port := "8080"
	fmt.Printf("Starting server on port %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","message":"Server is running"}`))
}

func dbCheck(w http.ResponseWriter, r *http.Request) {
	err := db.Ping()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":"error","message":"Database connection failed"}`))
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","message":"Database connection successful"}`))
}

func initDB() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS subscriptions (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			category TEXT NOT NULL,
			cost DECIMAL(10,2) NOT NULL,
			billing_cycle TEXT NOT NULL,
			next_billing DATE NOT NULL,
			description TEXT
		)
	`)
	return err
}

func getSubscriptions(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, name, category, cost, billing_cycle, next_billing, description 
		FROM subscriptions
		ORDER BY next_billing ASC
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var subscriptions []Subscription
	for rows.Next() {
		var s Subscription
		var nextBilling string
		if err := rows.Scan(&s.ID, &s.Name, &s.Category, &s.Cost, &s.BillingCycle, &nextBilling, &s.Description); err != nil {
			http.Error(w, fmt.Sprintf("Row scan error: %v", err), http.StatusInternalServerError)
			return
		}
		s.NextBilling = nextBilling
		subscriptions = append(subscriptions, s)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(subscriptions); err != nil {
		http.Error(w, fmt.Sprintf("JSON encoding error: %v", err), http.StatusInternalServerError)
		return
	}
}


func getSubscription(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var s Subscription
	var nextBilling string
	err := db.QueryRow(`
		SELECT id, name, category, cost, billing_cycle, next_billing, description 
		FROM subscriptions 
		WHERE id = $1
	`, id).Scan(&s.ID, &s.Name, &s.Category, &s.Cost, &s.BillingCycle, &nextBilling, &s.Description)
	
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Subscription not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		}
		return
	}
	
	s.NextBilling = nextBilling
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s); err != nil {
		http.Error(w, fmt.Sprintf("JSON encoding error: %v", err), http.StatusInternalServerError)
	}
}

// CreateSubscription creates a new subscription
func createSubscription(w http.ResponseWriter, r *http.Request) {
	var s Subscription
	
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading body: %v", err), http.StatusBadRequest)
		return
	}
	
	fmt.Println("Received body:", string(bodyBytes))
	
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	
	if s.Name == "" || s.Category == "" || s.Cost <= 0 || s.BillingCycle == "" || s.NextBilling == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}
	
	fmt.Printf("Parsed subscription: %+v\n", s)
	
	
	var id int
	err = db.QueryRow(`
		INSERT INTO subscriptions (name, category, cost, billing_cycle, next_billing, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, s.Name, s.Category, s.Cost, s.BillingCycle, s.NextBilling, s.Description).Scan(&id)
	
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	
	s.ID = id
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(s); err != nil {
		http.Error(w, fmt.Sprintf("JSON encoding error: %v", err), http.StatusInternalServerError)
	}
}

// UpdateSubscription updates an existing subscription
func updateSubscription(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	var s Subscription
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	
	if s.Name == "" || s.Category == "" || s.Cost <= 0 || s.BillingCycle == "" || s.NextBilling == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}
	
	result, err := db.Exec(`
		UPDATE subscriptions
		SET name = $1, category = $2, cost = $3, billing_cycle = $4, next_billing = $5, description = $6
		WHERE id = $7
	`, s.Name, s.Category, s.Cost, s.BillingCycle, s.NextBilling, s.Description, id)
	
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		return
	}
	
	idInt, err := strconv.Atoi(id)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	
	s.ID = idInt
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s); err != nil {
		http.Error(w, fmt.Sprintf("JSON encoding error: %v", err), http.StatusInternalServerError)
	}
}

// deleteSubscription removes a subscription
func deleteSubscription(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	result, err := db.Exec("DELETE FROM subscriptions WHERE id = $1", id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// getStats returns statistics about the subscriptions
func getStats(w http.ResponseWriter, r *http.Request) {
	// Get total monthly spend by category
	rows, err := db.Query(`
		SELECT category, SUM(cost) as total_cost
		FROM subscriptions
		GROUP BY category
		ORDER BY total_cost DESC
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type CategoryStat struct {
		Category string  `json:"category"`
		Cost     float64 `json:"cost"`
	}

	stats := struct {
		TotalMonthly float64        `json:"totalMonthly"`
		ByCategory   []CategoryStat `json:"byCategory"`
		Upcoming     []Subscription `json:"upcoming"`
	}{
		TotalMonthly: 0,
		ByCategory:   []CategoryStat{},
		Upcoming:     []Subscription{},
	}

	for rows.Next() {
		var cs CategoryStat
		if err := rows.Scan(&cs.Category, &cs.Cost); err != nil {
			http.Error(w, fmt.Sprintf("Row scan error: %v", err), http.StatusInternalServerError)
			return
		}
		stats.ByCategory = append(stats.ByCategory, cs)
		stats.TotalMonthly += cs.Cost
	}

	upcomingRows, err := db.Query(`
		SELECT id, name, category, cost, billing_cycle, next_billing, description
		FROM subscriptions
		WHERE next_billing BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '7 days'
		ORDER BY next_billing ASC
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer upcomingRows.Close()

	for upcomingRows.Next() {
		var s Subscription
		var nextBilling string
		if err := upcomingRows.Scan(&s.ID, &s.Name, &s.Category, &s.Cost, &s.BillingCycle, &nextBilling, &s.Description); err != nil {
			http.Error(w, fmt.Sprintf("Row scan error: %v", err), http.StatusInternalServerError)
			return
		}
		s.NextBilling = nextBilling
		stats.Upcoming = append(stats.Upcoming, s)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, fmt.Sprintf("JSON encoding error: %v", err), http.StatusInternalServerError)
	}
}