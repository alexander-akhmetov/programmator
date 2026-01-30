#!/bin/bash
# Prepares a bare Go project for testing programmator plan create command.
# Creates a minimal project where you can ask Claude to plan a feature.
#
# Usage: ./scripts/prep-plan-test.sh
# Then: programmator plan create "Add user authentication with JWT"

set -e

TEST_DIR="/tmp/programmator-plan-test"

echo "==> Cleaning up previous test project..."
rm -rf "$TEST_DIR"

echo "==> Creating test project at $TEST_DIR..."
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Initialize Go module
go mod init example.com/planproject

# Create a basic web server
cat > main.go << 'EOF'
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/items", itemsHandler)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func itemsHandler(w http.ResponseWriter, r *http.Request) {
	items := []Item{
		{ID: 1, Name: "Item 1"},
		{ID: 2, Name: "Item 2"},
	}
	json.NewEncoder(w).Encode(items)
}

type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
EOF

# Create handlers directory
mkdir -p handlers
cat > handlers/handlers.go << 'EOF'
package handlers

import (
	"net/http"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO: implement routing
	w.WriteHeader(http.StatusNotFound)
}
EOF

# Create models directory
mkdir -p models
cat > models/models.go << 'EOF'
package models

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type Item struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     int    `json:"owner_id"`
}
EOF

# Create plans directory
mkdir -p plans

# Initialize git repo
git init -q
git add -A
git commit -q -m "Initial commit: basic web server"

echo ""
echo "==> Plan test project created at $TEST_DIR"
echo ""
echo "The project is a basic Go web server with:"
echo "  - Health check endpoint"
echo "  - Items API endpoint"
echo "  - Stub handlers package"
echo "  - User and Item models"
echo ""
echo "To test programmator plan creation, run:"
echo "  cd $TEST_DIR"
echo "  programmator plan create \"Add user authentication with JWT\""
echo ""
echo "Other plan ideas to try:"
echo "  programmator plan create \"Add rate limiting to the API\""
echo "  programmator plan create \"Add database persistence with SQLite\""
echo "  programmator plan create \"Add request logging middleware\""
