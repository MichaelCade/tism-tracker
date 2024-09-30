package main

import (
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
)

type User struct {
	Name     string
	Miles    float64
	Progress float64
}

var (
	users = map[string]*User{
		"Cadey": {Name: "Cadey", Miles: 0, Progress: 0},
		"Gemma": {Name: "Gemma", Miles: 0, Progress: 0},
	}
	mileGoal = 100.0 // The goal in miles
	mu       sync.Mutex
)

func kmToMiles(km float64) float64 {
	return km * 0.621371
}

// Function to render only the progress section
func renderProgressSection(w http.ResponseWriter) error {
	tmpl, err := template.ParseFiles("templates/progress.html") // separate progress HTML
	if err != nil {
		return err
	}
	return tmpl.Execute(w, users)
}

func logDistance(w http.ResponseWriter, r *http.Request) {
	// Ensure this is a POST request
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Parse the form data
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Get values from the form
	name := r.FormValue("name")
	distanceStr := r.FormValue("distance")
	unit := r.FormValue("unit")

	// Validate form values
	if name == "" || distanceStr == "" || unit == "" {
		http.Error(w, "Missing form fields", http.StatusBadRequest)
		return
	}

	// Parse the distance
	distance, err := strconv.ParseFloat(distanceStr, 64)
	if err != nil {
		http.Error(w, "Invalid distance", http.StatusBadRequest)
		return
	}

	// Lock to avoid race conditions
	mu.Lock()
	defer mu.Unlock()

	// Get the user
	user, exists := users[name]
	if !exists {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Update user's miles based on the unit
	if unit == "kilometers" {
		distance = kmToMiles(distance) // Convert kilometers to miles
	}

	// Update user's total miles
	user.Miles += distance
	user.Progress = math.Min(100, (user.Miles/mileGoal)*100) // Cap progress at 100%

	// Log the new distance and progress for debugging
	log.Printf("%s has logged %.2f miles. Total: %.2f miles. Progress: %.2f%%", name, distance, user.Miles, user.Progress)

	// Render the updated progress section
	if err := renderProgressSection(w); err != nil {
		http.Error(w, "Error rendering progress", http.StatusInternalServerError)
		return
	}
}

func getProgress(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	// Render the full page when accessing the root path
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, users); err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

func main() {
	// Serve static files like CSS
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Handle the main page
	http.HandleFunc("/", getProgress)

	// Handle logging distance
	http.HandleFunc("/log", logDistance)

	fmt.Println("Server starting on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
