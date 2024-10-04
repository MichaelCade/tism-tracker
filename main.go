package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

type User struct {
	ID               int
	Name             string
	Miles            float64
	Runs             int
	Walks            int
	WalkMiles        float64
	RunMiles         float64
	WalkPct          float64
	RunPct           float64
	DailyAvgRequired float64
	ActivityLog      []string
}

var (
	db       *sql.DB
	mileGoal = 100.0 // The goal in miles
	mu       sync.Mutex
)

func initDB() error {
	var err error
	connStr := os.Getenv("DATABASE_URL")
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return err
	}
	return db.Ping()
}

func testDBConnection() {
	err := db.Ping()
	if err != nil {
		log.Fatalf("Error pinging database: %v", err)
	}
	fmt.Println("Successfully connected to the database")

	rows, err := db.Query("SELECT id, name FROM users")
	if err != nil {
		log.Fatalf("Error querying users table: %v", err)
	}
	defer rows.Close()

	fmt.Println("Users in the database:")
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Fatalf("Error scanning row: %v", err)
		}
		fmt.Printf("ID: %d, Name: %s\n", id, name)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating over rows: %v", err)
	}
}

func kmToMiles(km float64) float64 {
	return km * 0.621371
}

// Function to calculate the daily average distance needed for the remaining days of October
func calculateDailyAverage(user *User) {
	currentDate := time.Now()
	// Set the month to October
	october := time.Date(currentDate.Year(), time.October, 1, 0, 0, 0, 0, currentDate.Location())
	endOfMonth := time.Date(october.Year(), october.Month()+1, 0, 0, 0, 0, 0, october.Location())
	var daysRemaining int
	if currentDate.Month() == time.September {
		daysRemaining = endOfMonth.Day() // All days in October
	} else if currentDate.Month() == time.October {
		daysRemaining = endOfMonth.Day() - currentDate.Day()
	} else {
		daysRemaining = 0
	}
	if daysRemaining > 0 {
		user.DailyAvgRequired = (mileGoal - user.Miles) / float64(daysRemaining)
	} else {
		user.DailyAvgRequired = 0
	}
}

// Function to render only the progress section
func renderProgressSection(w http.ResponseWriter) error {
	tmpl, err := template.ParseFiles("templates/progress.html") // Ensure this template exists
	if err != nil {
		log.Printf("Error loading progress template: %v", err)
		return err
	}
	users, err := getUsersFromDB()
	if err != nil {
		log.Printf("Error getting users from database: %v", err)
		return err
	}
	for _, user := range users {
		calculateDailyAverage(user)
	}
	if err := tmpl.Execute(w, users); err != nil {
		log.Printf("Error rendering progress template: %v", err)
		return err
	}
	return nil
}

func logDistance(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	log.Println("Received request to log distance")

	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	distanceStr := r.FormValue("distance")
	unit := r.FormValue("unit")
	activity := r.FormValue("activity")

	log.Printf("Form values - Name: %s, Distance: %s, Unit: %s, Activity: %s", name, distanceStr, unit, activity)

	distance, err := strconv.ParseFloat(distanceStr, 64)
	if err != nil {
		log.Printf("Invalid distance value: %v", err)
		http.Error(w, "Invalid distance value", http.StatusBadRequest)
		return
	}

	// Convert kilometers to miles if necessary
	if unit == "kilometers" {
		log.Printf("Converting distance from kilometers to miles: %f km", distance)
		distance = kmToMiles(distance)
		log.Printf("Converted distance: %f miles", distance)
	}

	user, err := getUserFromDB(name)
	if err != nil {
		log.Printf("User not found: %v", err)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Update user data based on activity type
	switch activity {
	case "walk":
		log.Printf("Logging walk activity for user %s: %f miles", name, distance)
		user.WalkMiles += distance
		user.Walks++
	case "run":
		log.Printf("Logging run activity for user %s: %f miles", name, distance)
		user.RunMiles += distance
		user.Runs++
	default:
		log.Printf("Invalid activity type: %s", activity)
		http.Error(w, "Invalid activity type", http.StatusBadRequest)
		return
	}
	user.Miles += distance
	user.ActivityLog = append(user.ActivityLog, fmt.Sprintf("%s: %.2f miles (%s)", time.Now().Format("2006-01-02 15:04:05"), distance, activity))

	// Calculate progress percentages
	user.WalkPct = (user.WalkMiles / mileGoal) * 100
	user.RunPct = (user.RunMiles / mileGoal) * 100

	// Update database
	activityLog := strings.Join(user.ActivityLog, ",")
	_, err = db.Exec("UPDATE users SET miles = $1, runs = $2, walks = $3, walk_miles = $4, run_miles = $5, walk_pct = $6, run_pct = $7, daily_avg_required = $8, activity_log = $9 WHERE name = $10",
		user.Miles, user.Runs, user.Walks, user.WalkMiles, user.RunMiles, user.WalkPct, user.RunPct, user.DailyAvgRequired, activityLog, user.Name)
	if err != nil {
		log.Printf("Error updating database: %v", err)
		http.Error(w, "Error updating database", http.StatusInternalServerError)
		return
	}

	log.Println("Successfully updated user data in the database")

	// Render the updated progress section
	if err := renderProgressSection(w); err != nil {
		log.Printf("Error rendering progress section: %v", err)
		http.Error(w, "Error rendering progress section", http.StatusInternalServerError)
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
	users, err := getUsersFromDB()
	if err != nil {
		log.Printf("Error getting users from database: %v", err)
		http.Error(w, "Error getting users from database", http.StatusInternalServerError)
		return
	}
	for _, user := range users {
		calculateDailyAverage(user)
	}
	if err := tmpl.Execute(w, users); err != nil {
		log.Printf("Error rendering templates: %v", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

func getUsersFromDB() (map[string]*User, error) {
	rows, err := db.Query("SELECT id, name, miles, runs, walks, walk_miles, run_miles, walk_pct, run_pct, daily_avg_required, activity_log FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make(map[string]*User)
	for rows.Next() {
		var user User
		var activityLog string
		if err := rows.Scan(&user.ID, &user.Name, &user.Miles, &user.Runs, &user.Walks, &user.WalkMiles, &user.RunMiles, &user.WalkPct, &user.RunPct, &user.DailyAvgRequired, &activityLog); err != nil {
			return nil, err
		}
		user.ActivityLog = strings.Split(activityLog, ",")
		users[user.Name] = &user
	}
	return users, nil
}

func getUserFromDB(name string) (*User, error) {
	var user User
	var activityLog string
	err := db.QueryRow("SELECT id, name, miles, runs, walks, walk_miles, run_miles, walk_pct, run_pct, daily_avg_required, activity_log FROM users WHERE name = $1", name).Scan(
		&user.ID, &user.Name, &user.Miles, &user.Runs, &user.Walks, &user.WalkMiles, &user.RunMiles, &user.WalkPct, &user.RunPct, &user.DailyAvgRequired, &activityLog)
	if err != nil {
		return nil, err
	}
	user.ActivityLog = strings.Split(activityLog, ",")
	return &user, nil
}

func main() {
	if err := initDB(); err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}

	// Test the database connection
	testDBConnection()

	// Serve static files like CSS
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Handle the main page
	http.HandleFunc("/", getProgress)

	// Handle logging distance
	http.HandleFunc("/log", logDistance)

	fmt.Println("Server starting on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
