package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

var (
	balances = map[int]int{
		1: 100,
	}
	balancesMu sync.Mutex
)

type giftRequest struct {
	UserID     int `json:"userId"`
	StreamerID int `json:"streamerId"`
	Amount     int `json:"amount"`
}

type giftResponse struct {
	Balance int `json:"balance"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func main() {
	http.HandleFunc("/gift", giftHandler)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func giftHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req giftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be greater than 0")
		return
	}

	select {
	case <-r.Context().Done():
		writeError(w, http.StatusRequestTimeout, "request canceled")
		return
	default:
	}

	balancesMu.Lock()
	defer balancesMu.Unlock()

	balance, ok := balances[req.UserID]
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if balance < req.Amount {
		writeError(w, http.StatusBadRequest, "insufficient balance")
		return
	}

	balance -= req.Amount
	balances[req.UserID] = balance

	writeJSON(w, http.StatusOK, giftResponse{Balance: balance})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
