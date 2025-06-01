package main

import (
	"database/sql"
	"encoding/json"
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

type RowingUser struct {
	ID               int
	Name             string
	Kilometers       float64  // Total distance in kilometers
	KilometersGoal   float64  // The goal in kilometers (100km)
	Sessions         int      // Number of rowing sessions
	SessionsDuration float64  // Total duration in minutes (for backward compatibility)
	SessionMinutes   int      // Minutes part of the duration
	SessionSeconds   int      // Seconds part of the duration
	DailyAvgRequired float64  // Required daily average to reach goal
	DaysRemaining    int      // Days remaining in the challenge
	ProgressPct      float64  // Progress percentage toward goal
	ActivityLog      []string // Log of rowing activities
}

var (
	db       *sql.DB
	mileGoal = 100.0 // The goal in miles
	kmGoal   = 100.0 // The rowing goal in kilometers
	mu       sync.Mutex
)

func initDB() error {
	var err error
	connStr := os.Getenv("DATABASE_URL")
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return err
	}

	// Initialize rowing table if it doesn't exist
	if err := initRowingTable(); err != nil {
		log.Printf("Error initializing rowing table: %v", err)
		return err
	}

	return db.Ping()
}

// Initialize the rowing_users table if it doesn't exist
func initRowingTable() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS rowing_users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			kilometers FLOAT NOT NULL DEFAULT 0,
			kilometers_goal FLOAT NOT NULL DEFAULT 100,
			sessions INT NOT NULL DEFAULT 0,
			sessions_duration FLOAT NOT NULL DEFAULT 0,
			session_minutes INT NOT NULL DEFAULT 0,
			session_seconds INT NOT NULL DEFAULT 0,
			daily_avg_required FLOAT NOT NULL DEFAULT 0,
			days_remaining INT NOT NULL DEFAULT 30,
			progress_pct FLOAT NOT NULL DEFAULT 0,
			activity_log TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return err
	}

	// Check if users exist in the rowing_users table, if not, add default users
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM rowing_users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Add default users
		_, err = db.Exec("INSERT INTO rowing_users (name, activity_log) VALUES ('Michael', '')")
		if err != nil {
			return err
		}
	}

	return nil
}

// Function to calculate the daily average distance needed for the remaining days of June 2025
func calculateRowingDailyAverage(user *RowingUser) {
	// Since the date is 1 June 2025, we'll use that for calculations
	// If we were in development mode, we'd use the current date
	currentDate := time.Date(2025, time.June, 1, 0, 0, 0, 0, time.Local)
	endOfMonth := time.Date(2025, time.June+1, 0, 0, 0, 0, 0, time.Local)

	user.DaysRemaining = endOfMonth.Day() - currentDate.Day() + 1

	if user.DaysRemaining > 0 {
		user.DailyAvgRequired = (kmGoal - user.Kilometers) / float64(user.DaysRemaining)
		if user.DailyAvgRequired < 0 {
			user.DailyAvgRequired = 0
		}
	} else {
		user.DailyAvgRequired = 0
	}
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

// Get rowing users from the database
func getRowingUsersFromDB() (map[string]*RowingUser, error) {
	rows, err := db.Query("SELECT id, name, kilometers, sessions, sessions_duration, daily_avg_required, progress_pct, activity_log FROM rowing_users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make(map[string]*RowingUser)
	for rows.Next() {
		var user RowingUser
		var activityLog string
		if err := rows.Scan(&user.ID, &user.Name, &user.Kilometers, &user.Sessions, &user.SessionsDuration, &user.DailyAvgRequired, &user.ProgressPct, &activityLog); err != nil {
			return nil, err
		}
		user.KilometersGoal = kmGoal
		if activityLog != "" {
			user.ActivityLog = strings.Split(activityLog, ",")
		} else {
			user.ActivityLog = []string{}
		}
		calculateRowingDailyAverage(&user)
		users[user.Name] = &user
	}
	return users, nil
}

// Get a specific rowing user from the database
func getRowingUserFromDB(name string) (*RowingUser, error) {
	var user RowingUser
	var activityLog string
	err := db.QueryRow("SELECT id, name, kilometers, sessions, sessions_duration, daily_avg_required, progress_pct, activity_log FROM rowing_users WHERE name = $1", name).Scan(
		&user.ID, &user.Name, &user.Kilometers, &user.Sessions, &user.SessionsDuration, &user.DailyAvgRequired, &user.ProgressPct, &activityLog)
	if err != nil {
		return nil, err
	}
	user.KilometersGoal = kmGoal
	if activityLog != "" {
		user.ActivityLog = strings.Split(activityLog, ",")
	} else {
		user.ActivityLog = []string{}
	}
	calculateRowingDailyAverage(&user)
	return &user, nil
}

// Function to log rowing distance for the 100km challenge
func logRowingDistance(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	log.Println("Received request to log rowing distance")

	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	distanceStr := r.FormValue("distance")
	durationStr := r.FormValue("duration")

	log.Printf("Form values - Name: %s, Distance: %s, Duration: %s", name, distanceStr, durationStr)

	// Parse distance
	distance, err := strconv.ParseFloat(distanceStr, 64)
	if err != nil {
		log.Printf("Invalid distance value: %v", err)
		http.Error(w, "Invalid distance value", http.StatusBadRequest)
		return
	}

	// Parse duration
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		log.Printf("Invalid duration value: %v", err)
		http.Error(w, "Invalid duration value", http.StatusBadRequest)
		return
	}

	user, err := getRowingUserFromDB(name)
	if err != nil {
		log.Printf("User not found: %v", err)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Update user data
	user.Kilometers += distance
	user.Sessions++
	user.SessionsDuration += duration
	user.ProgressPct = (user.Kilometers / kmGoal) * 100

	// Calculate the new daily average
	calculateRowingDailyAverage(user)

	// Add to activity log
	user.ActivityLog = append(user.ActivityLog, fmt.Sprintf("%s: %.2f km (%.1f min)", time.Now().Format("2006-01-02 15:04:05"), distance, duration))

	// Update database
	activityLog := strings.Join(user.ActivityLog, ",")
	_, err = db.Exec("UPDATE rowing_users SET kilometers = $1, sessions = $2, sessions_duration = $3, daily_avg_required = $4, progress_pct = $5, activity_log = $6 WHERE name = $7",
		user.Kilometers, user.Sessions, user.SessionsDuration, user.DailyAvgRequired, user.ProgressPct, activityLog, user.Name)
	if err != nil {
		log.Printf("Error updating database: %v", err)
		http.Error(w, "Error updating database", http.StatusInternalServerError)
		return
	}

	log.Println("Successfully updated rowing user data in the database")

	// Render the updated rowing progress section
	if err := renderRowingProgressSection(w); err != nil {
		log.Printf("Error rendering rowing progress section: %v", err)
		http.Error(w, "Error rendering rowing progress section", http.StatusInternalServerError)
	}
}

// Function to render only the rowing progress section
func renderRowingProgressSection(w http.ResponseWriter) error {
	// Create template functions map
	funcMap := template.FuncMap{
		"sub": func(a, b float64) float64 {
			return a - b
		},
		"extractDates": func(logs []string) string {
			// Extract dates from activity logs for chart
			if len(logs) == 0 {
				return "[]"
			}

			dates := make([]string, 0, len(logs))
			for _, log := range logs {
				parts := strings.Split(log, ":")
				if len(parts) > 0 {
					// Get just the date part
					datePart := strings.Split(parts[0], " ")[0]
					dates = append(dates, datePart)
				}
			}

			// Reverse the order to show oldest to newest
			for i, j := 0, len(dates)-1; i < j; i, j = i+1, j-1 {
				dates[i], dates[j] = dates[j], dates[i]
			}

			// Convert to JSON
			jsonDates, err := json.Marshal(dates)
			if err != nil {
				return "[]"
			}
			return string(jsonDates)
		},
		"extractDistances": func(logs []string) string {
			// Extract distances from activity logs for chart
			if len(logs) == 0 {
				return "[]"
			}

			distances := make([]float64, 0, len(logs))
			for _, log := range logs {
				parts := strings.Split(log, ":")
				if len(parts) > 1 {
					// Extract the distance part
					distanceStr := strings.Split(parts[1], " km")[0]
					distance, err := strconv.ParseFloat(strings.TrimSpace(distanceStr), 64)
					if err == nil {
						distances = append(distances, distance)
					}
				}
			}

			// Reverse the order to match dates
			for i, j := 0, len(distances)-1; i < j; i, j = i+1, j-1 {
				distances[i], distances[j] = distances[j], distances[i]
			}

			// Convert to JSON
			jsonDistances, err := json.Marshal(distances)
			if err != nil {
				return "[]"
			}
			return string(jsonDistances)
		},
	}

	tmpl, err := template.New("rowing_progress.html").Funcs(funcMap).ParseFiles("templates/rowing_progress.html")
	if err != nil {
		log.Printf("Error loading rowing progress template: %v", err)
		return err
	}
	users, err := getRowingUsersFromDB()
	if err != nil {
		log.Printf("Error getting rowing users from database: %v", err)
		return err
	}
	if err := tmpl.Execute(w, users); err != nil {
		log.Printf("Error rendering rowing progress template: %v", err)
		return err
	}
	return nil
}

// Handle the request for the rowing progress page
func getRowingProgress(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	// Create template functions map
	funcMap := template.FuncMap{
		"sub": func(a, b float64) float64 {
			return a - b
		},
		"extractDates": func(logs []string) string {
			// Extract dates from activity logs for chart
			if len(logs) == 0 {
				return "[]"
			}

			dates := make([]string, 0, len(logs))
			for _, log := range logs {
				parts := strings.Split(log, ":")
				if len(parts) > 0 {
					// Get just the date part
					datePart := strings.Split(parts[0], " ")[0]
					dates = append(dates, datePart)
				}
			}

			// Reverse the order to show oldest to newest
			for i, j := 0, len(dates)-1; i < j; i, j = i+1, j-1 {
				dates[i], dates[j] = dates[j], dates[i]
			}

			// Convert to JSON
			jsonDates, err := json.Marshal(dates)
			if err != nil {
				return "[]"
			}
			return string(jsonDates)
		},
		"extractDistances": func(logs []string) string {
			// Extract distances from activity logs for chart
			if len(logs) == 0 {
				return "[]"
			}

			distances := make([]float64, 0, len(logs))
			for _, log := range logs {
				parts := strings.Split(log, ":")
				if len(parts) > 1 {
					// Extract the distance part
					distanceStr := strings.Split(parts[1], " km")[0]
					distance, err := strconv.ParseFloat(strings.TrimSpace(distanceStr), 64)
					if err == nil {
						distances = append(distances, distance)
					}
				}
			}

			// Reverse the order to match dates
			for i, j := 0, len(distances)-1; i < j; i, j = i+1, j-1 {
				distances[i], distances[j] = distances[j], distances[i]
			}

			// Convert to JSON
			jsonDistances, err := json.Marshal(distances)
			if err != nil {
				return "[]"
			}
			return string(jsonDistances)
		},
	}

	// Render the full rowing page with funcMap
	tmpl, err := template.New("rowing.html").Funcs(funcMap).ParseFiles("templates/rowing.html", "templates/rowing_progress.html")
	if err != nil {
		log.Printf("Error loading rowing templates: %v", err)
		http.Error(w, "Error loading rowing template", http.StatusInternalServerError)
		return
	}
	users, err := getRowingUsersFromDB()
	if err != nil {
		log.Printf("Error getting rowing users from database: %v", err)
		http.Error(w, "Error getting rowing users from database", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, users); err != nil {
		log.Printf("Error rendering rowing templates: %v", err)
		http.Error(w, "Error rendering rowing template", http.StatusInternalServerError)
	}
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

	// Handle the rowing page
	http.HandleFunc("/rowing", getRowingProgress)

	// Handle logging distance
	http.HandleFunc("/log", logDistance)

	// Handle logging rowing distance
	http.HandleFunc("/log-rowing", logRowingDistance)

	fmt.Println("Server starting on port 8181...")
	log.Fatal(http.ListenAndServe(":8181", nil))
}
