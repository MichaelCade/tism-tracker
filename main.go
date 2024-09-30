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
	Name      string
	Miles     float64
	Progress  float64
	Runs      int
	Walks     int
	WalkMiles float64
	RunMiles  float64
	WalkPct   float64
	RunPct    float64
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
		log.Printf("Error loading progress template: %v", err)
		return err
	}
	if err := tmpl.Execute(w, users); err != nil {
		log.Printf("Error rendering progress template: %v", err)
		return err
	}
	return nil
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
	activity := r.FormValue("activity")

	// Validate form values
	if name == "" || distanceStr == "" || unit == "" || activity == "" {
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

	// Update user's total miles and activity-specific miles
	if activity == "walk" {
		user.WalkMiles += distance
		user.Walks++
	} else if activity == "run" {
		user.RunMiles += distance
		user.Runs++
	}
	user.Miles += distance
	user.Progress = math.Min(100, (user.Miles/mileGoal)*100) // Cap progress at 100%
	user.WalkPct = math.Min(100, (user.WalkMiles/mileGoal)*100)
	user.RunPct = math.Min(100, (user.RunMiles/mileGoal)*100)

	// Log the new distance and progress for debugging
	log.Printf("%s has logged %.2f miles (%s). Total: %.2f miles. Progress: %.2f%%", name, distance, activity, user.Miles, user.Progress)

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
	tmpl, err := template.ParseFiles("templates/index.html", "templates/progress.html")
	if err != nil {
		log.Printf("Error loading templates: %v", err)
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, users); err != nil {
		log.Printf("Error rendering templates: %v", err)
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
