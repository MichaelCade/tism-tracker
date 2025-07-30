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

// WeightTrainingType represents the category of weight training
type WeightTrainingType string

const (
	Push WeightTrainingType = "push"
	Pull WeightTrainingType = "pull"
	Legs WeightTrainingType = "legs"
)

// WeightTrainingSet represents a single set in a weight training exercise
type WeightTrainingSet struct {
	Weight float64 // Weight used in kg
	Reps   int     // Number of repetitions
}

// WeightTrainingExercise represents a specific exercise within a training category
type WeightTrainingExercise struct {
	Name       string              // Name of the exercise
	Sets       []WeightTrainingSet // Sets performed for this exercise
	Date       time.Time           // Date when the exercise was performed
	ExerciseID int                 // Database ID for the exercise
}

// WeightTrainingUser represents a user's weight training data
type WeightTrainingUser struct {
	ID            int
	Name          string
	PushSessions  int                                             // Number of push training sessions
	PullSessions  int                                             // Number of pull training sessions
	LegsSessions  int                                             // Number of legs training sessions
	TotalSessions int                                             // Total number of training sessions
	Exercises     map[WeightTrainingType][]WeightTrainingExercise // All exercises performed by the user
	ActivityLog   []string                                        // Log of weight training activities
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

	// Check if connection string is empty
	if connStr == "" {
		log.Println("Warning: DATABASE_URL environment variable is not set")
		connStr = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
		log.Printf("Using default connection string: %s", connStr)
	}

	// Fix common connection string issues
	// Replace any 'cd' without '=' that might be causing the error
	if strings.Contains(connStr, " cd ") {
		connStr = strings.Replace(connStr, " cd ", "=", -1)
		log.Println("Fixed malformed connection string by replacing 'cd' with '='")
	}

	// Ensure connection string has proper format
	if !strings.HasPrefix(connStr, "postgres://") {
		// If it doesn't have the postgres:// prefix, try to format it properly
		if !strings.Contains(connStr, "=") {
			// Assume it's just a host:port/database format
			connStr = fmt.Sprintf("postgres://postgres:postgres@%s?sslmode=disable", connStr)
		} else {
			// Assume it's a key=value format
			connStr = "postgres://" + connStr
		}
		log.Printf("Reformatted connection string: %s", connStr)
	}

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Error opening database connection: %v", err)
		return err
	}

	// Initialize users table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			miles FLOAT NOT NULL DEFAULT 0,
			runs INT NOT NULL DEFAULT 0,
			walks INT NOT NULL DEFAULT 0,
			walk_miles FLOAT NOT NULL DEFAULT 0,
			run_miles FLOAT NOT NULL DEFAULT 0,
			walk_pct FLOAT NOT NULL DEFAULT 0,
			run_pct FLOAT NOT NULL DEFAULT 0,
			daily_avg_required FLOAT NOT NULL DEFAULT 0,
			activity_log TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		log.Printf("Error creating users table: %v", err)
		return err
	}

	// Check if users exist in the users table, if not, add default users
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		log.Printf("Error checking user count: %v", err)
		return err
	}

	if count == 0 {
		// Add default users
		_, err = db.Exec("INSERT INTO users (name, activity_log) VALUES ('Cadey', '')")
		if err != nil {
			log.Printf("Error inserting default user Cadey: %v", err)
			return err
		}
		_, err = db.Exec("INSERT INTO users (name, activity_log) VALUES ('Gemma', '')")
		if err != nil {
			log.Printf("Error inserting default user Gemma: %v", err)
			return err
		}
	}

	// Initialize rowing table if it doesn't exist
	if err := initRowingTable(); err != nil {
		log.Printf("Error initializing rowing table: %v", err)
		return err
	}

	// Initialize weight training tables
	if err := initWeightTrainingTables(); err != nil {
		log.Printf("Error initializing weight training tables: %v", err)
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

// Initialize the weight_training tables if they don't exist
func initWeightTrainingTables() error {
	// Create the weight_training_users table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS weight_training_users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			push_sessions INT NOT NULL DEFAULT 0,
			pull_sessions INT NOT NULL DEFAULT 0,
			legs_sessions INT NOT NULL DEFAULT 0,
			total_sessions INT NOT NULL DEFAULT 0,
			activity_log TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return err
	}

	// Create the weight_training_exercises table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS weight_training_exercises (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL REFERENCES weight_training_users(id),
			name TEXT NOT NULL,
			training_type TEXT NOT NULL,
			date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Create the weight_training_sets table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS weight_training_sets (
			id SERIAL PRIMARY KEY,
			exercise_id INT NOT NULL REFERENCES weight_training_exercises(id),
			weight FLOAT NOT NULL,
			reps INT NOT NULL,
			set_number INT NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	// Check if users exist in the weight_training_users table, if not, add default users
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM weight_training_users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Add default users
		_, err = db.Exec("INSERT INTO weight_training_users (name, activity_log) VALUES ('Michael', '')")
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
	log.Println("Testing database connection...")

	// Check if the database connection is properly initialized
	if db == nil {
		log.Fatal("Database connection is nil, initialization failed")
	}

	// Test the connection with a ping
	err := db.Ping()
	if err != nil {
		log.Fatalf("Error pinging database: %v", err)
	}
	log.Println("Successfully connected to the database!")

	// Try to query the users table (a real operation)
	log.Println("Testing query to users table...")
	rows, err := db.Query("SELECT id, name FROM users")
	if err != nil {
		log.Fatalf("Error querying users table: %v", err)
	}
	defer rows.Close()

	log.Println("Users in the database:")
	userCount := 0
	for rows.Next() {
		userCount++
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Fatalf("Error scanning row: %v", err)
		}
		log.Printf("ID: %d, Name: %s", id, name)
	}
	if userCount == 0 {
		log.Println("No users found in the database, but query executed successfully")
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating over rows: %v", err)
	}

	log.Println("Database connection and query tests completed successfully")
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

// Get all weight training users from the database
func getWeightTrainingUsersFromDB() ([]*WeightTrainingUser, error) {
	rows, err := db.Query("SELECT id, name, push_sessions, pull_sessions, legs_sessions, total_sessions, activity_log FROM weight_training_users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*WeightTrainingUser
	for rows.Next() {
		user := &WeightTrainingUser{
			Exercises: make(map[WeightTrainingType][]WeightTrainingExercise),
		}
		var activityLog string
		err := rows.Scan(&user.ID, &user.Name, &user.PushSessions, &user.PullSessions, &user.LegsSessions, &user.TotalSessions, &activityLog)
		if err != nil {
			return nil, err
		}
		if activityLog != "" {
			user.ActivityLog = strings.Split(activityLog, ",")
		}
		users = append(users, user)
	}

	// Load exercises for each user
	for _, user := range users {
		// Load exercises for push training
		if err := loadExercisesForUser(user, string(Push)); err != nil {
			return nil, err
		}

		// Load exercises for pull training
		if err := loadExercisesForUser(user, string(Pull)); err != nil {
			return nil, err
		}

		// Load exercises for legs training
		if err := loadExercisesForUser(user, string(Legs)); err != nil {
			return nil, err
		}
	}

	return users, nil
}

// Get a specific weight training user from the database
func getWeightTrainingUserFromDB(name string) (*WeightTrainingUser, error) {
	user := &WeightTrainingUser{
		Exercises: make(map[WeightTrainingType][]WeightTrainingExercise),
	}
	var activityLog string
	err := db.QueryRow("SELECT id, name, push_sessions, pull_sessions, legs_sessions, total_sessions, activity_log FROM weight_training_users WHERE name = $1", name).Scan(
		&user.ID, &user.Name, &user.PushSessions, &user.PullSessions, &user.LegsSessions, &user.TotalSessions, &activityLog)
	if err != nil {
		return nil, err
	}
	if activityLog != "" {
		user.ActivityLog = strings.Split(activityLog, ",")
	}

	// Load exercises for push training
	if err := loadExercisesForUser(user, string(Push)); err != nil {
		return nil, err
	}

	// Load exercises for pull training
	if err := loadExercisesForUser(user, string(Pull)); err != nil {
		return nil, err
	}

	// Load exercises for legs training
	if err := loadExercisesForUser(user, string(Legs)); err != nil {
		return nil, err
	}

	return user, nil
}

// Load exercises of a specific type for a user
func loadExercisesForUser(user *WeightTrainingUser, trainingType string) error {
	rows, err := db.Query(`
		SELECT e.id, e.name, e.date 
		FROM weight_training_exercises e
		WHERE e.user_id = $1 AND e.training_type = $2
		ORDER BY e.date DESC
	`, user.ID, trainingType)
	if err != nil {
		return err
	}
	defer rows.Close()

	exercises := []WeightTrainingExercise{}
	for rows.Next() {
		exercise := WeightTrainingExercise{}
		err := rows.Scan(&exercise.ExerciseID, &exercise.Name, &exercise.Date)
		if err != nil {
			return err
		}

		// Load sets for this exercise
		exercise.Sets, err = loadSetsForExercise(exercise.ExerciseID)
		if err != nil {
			return err
		}

		exercises = append(exercises, exercise)
	}

	user.Exercises[WeightTrainingType(trainingType)] = exercises
	return nil
}

// Load all sets for a specific exercise
func loadSetsForExercise(exerciseID int) ([]WeightTrainingSet, error) {
	rows, err := db.Query(`
		SELECT weight, reps 
		FROM weight_training_sets
		WHERE exercise_id = $1
		ORDER BY set_number ASC
	`, exerciseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sets := []WeightTrainingSet{}
	for rows.Next() {
		set := WeightTrainingSet{}
		err := rows.Scan(&set.Weight, &set.Reps)
		if err != nil {
			return nil, err
		}
		sets = append(sets, set)
	}

	return sets, nil
}

// Log a weight training exercise with its sets
func logWeightTrainingExercise(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	log.Println("Received request to log weight training exercise")

	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	exerciseName := r.FormValue("exercise_name")
	trainingType := r.FormValue("training_type")

	// Validate training type
	if trainingType != string(Push) && trainingType != string(Pull) && trainingType != string(Legs) {
		log.Printf("Invalid training type: %s", trainingType)
		http.Error(w, "Invalid training type", http.StatusBadRequest)
		return
	} // Get weights for each set (always 8 reps)
	weights := []float64{}
	reps := []int{8, 8, 8} // Fixed at 8 reps per set

	// Get the selected exercise
	selectedExercise := r.FormValue("selected_exercise")
	if selectedExercise != "" {
		exerciseName = selectedExercise
	}

	for i := 1; i <= 3; i++ {
		weightStr := r.FormValue(fmt.Sprintf("set%d_weight", i))

		weight, err := strconv.ParseFloat(weightStr, 64)
		if err != nil {
			log.Printf("Invalid weight value for set %d: %v", i, err)
			http.Error(w, fmt.Sprintf("Invalid weight value for set %d", i), http.StatusBadRequest)
			return
		}

		weights = append(weights, weight)
	}

	// Get user data
	user, err := getWeightTrainingUserFromDB(name)
	if err != nil {
		log.Printf("User not found: %v", err)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Begin transaction for inserting exercise and sets
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Error starting transaction: %v", err)
		http.Error(w, "Error starting database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Insert exercise
	var exerciseID int
	err = tx.QueryRow(`
		INSERT INTO weight_training_exercises (user_id, name, training_type, date)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		RETURNING id
	`, user.ID, exerciseName, trainingType).Scan(&exerciseID)
	if err != nil {
		log.Printf("Error inserting exercise: %v", err)
		http.Error(w, "Error inserting exercise", http.StatusInternalServerError)
		return
	}

	// Insert sets
	for i := 0; i < len(weights); i++ {
		_, err = tx.Exec(`
			INSERT INTO weight_training_sets (exercise_id, weight, reps, set_number)
			VALUES ($1, $2, $3, $4)
		`, exerciseID, weights[i], reps[i], i+1)
		if err != nil {
			log.Printf("Error inserting set %d: %v", i+1, err)
			http.Error(w, fmt.Sprintf("Error inserting set %d", i+1), http.StatusInternalServerError)
			return
		}
	}

	// Update user stats
	switch trainingType {
	case string(Push):
		user.PushSessions++
	case string(Pull):
		user.PullSessions++
	case string(Legs):
		user.LegsSessions++
	}
	user.TotalSessions++

	// Format sets for activity log
	setsStr := ""
	for i := 0; i < len(weights); i++ {
		if i > 0 {
			setsStr += ", "
		}
		setsStr += fmt.Sprintf("%.1fkg×%d", weights[i], reps[i])
	}

	// Add to activity log
	activityEntry := fmt.Sprintf("%s: %s (%s) - Sets: %s", time.Now().Format("2006-01-02 15:04:05"), exerciseName, trainingType, setsStr)
	user.ActivityLog = append(user.ActivityLog, activityEntry)

	// Update user in database
	activityLog := strings.Join(user.ActivityLog, ",")
	_, err = tx.Exec(`
		UPDATE weight_training_users 
		SET push_sessions = $1, pull_sessions = $2, legs_sessions = $3, total_sessions = $4, activity_log = $5
		WHERE id = $6
	`, user.PushSessions, user.PullSessions, user.LegsSessions, user.TotalSessions, activityLog, user.ID)
	if err != nil {
		log.Printf("Error updating user: %v", err)
		http.Error(w, "Error updating user", http.StatusInternalServerError)
		return
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		log.Printf("Error committing transaction: %v", err)
		http.Error(w, "Error committing changes to database", http.StatusInternalServerError)
		return
	}

	// Render the updated weight training progress section
	renderWeightTrainingProgressSection(w, trainingType)
}

// Render the weight training page
func getWeightTrainingProgress(w http.ResponseWriter, r *http.Request) {
	// Determine the training type from the URL
	trainingType := r.URL.Query().Get("type")
	if trainingType == "" {
		trainingType = string(Push) // Default to push training if not specified
	}

	// Validate training type
	if trainingType != string(Push) && trainingType != string(Pull) && trainingType != string(Legs) {
		log.Printf("Invalid training type: %s", trainingType)
		http.Error(w, "Invalid training type", http.StatusBadRequest)
		return
	}

	// Get users from database
	users, err := getWeightTrainingUsersFromDB()
	if err != nil {
		log.Printf("Error getting weight training users: %v", err)
		http.Error(w, "Error getting weight training data", http.StatusInternalServerError)
		return
	}

	// Create a data structure for the template
	data := struct {
		Users        []*WeightTrainingUser
		TrainingType string
		Exercises    map[string][]string // Map of training type to exercise names
	}{
		Users:        users,
		TrainingType: trainingType,
		Exercises: map[string][]string{
			string(Push): {
				"Flat Dumbbells",
				"Flat Flys",
				"Seated Dumbbell front raises",
				"Seated Dumbbell side raises",
				"Seated Dumbbell shoulder press",
				"Tricep Pushdowns",
				"Incline Smith",
				"Close Grip Incline Smith",
				"Overhead Rope (Cables)",
				"Assisted Dips/Dip machine",
			},
			string(Pull): {
				"Deadlifts",
				"Bent Over Rows (Underhand)",
				"Shrugs (Barbell or dumbbell)",
				"Lat Pulldown",
				"Upright Rows (Barbell or Rope)",
				"Rear Delt Raises (Dumbbell)",
				"Single Preacher Dumbbell Curls",
				"EZ Bar Standing Curls",
				"Double Dumbbell Hammer Curls",
			},
			string(Legs): {
				"Barbell Squat",
				"Straight leg deadlifts",
				"Front squat",
				"Leg Press",
				"Calf Raises on Leg Press",
				"Leg Extensions",
				"Hamstring curls (Machine)",
				"Dumbbell lunges",
				"Ab/Crunch Machine",
				"Captains Chair Leg or Knee Raises",
			},
		},
	}
	// Define template functions
	funcMap := template.FuncMap{
		"makeExerciseMap": func(exercises []WeightTrainingExercise) map[string][]WeightTrainingExercise {
			result := make(map[string][]WeightTrainingExercise)
			for _, ex := range exercises {
				result[ex.Name] = append(result[ex.Name], ex)
			}
			return result
		},
		"toWeightTrainingType": func(s string) WeightTrainingType {
			return WeightTrainingType(s)
		},
	}

	// Parse and execute the template
	tmpl, err := template.New("weight_training.html").Funcs(funcMap).ParseFiles("templates/weight_training.html", "templates/weight_training_progress.html")
	if err != nil {
		log.Printf("Error loading weight training templates: %v", err)
		http.Error(w, "Error loading templates", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error rendering weight training template: %v", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// Render only the weight training progress section for a specific training type
func renderWeightTrainingProgressSection(w http.ResponseWriter, trainingType string) {
	// Validate training type
	if trainingType != string(Push) && trainingType != string(Pull) && trainingType != string(Legs) {
		log.Printf("Invalid training type: %s", trainingType)
		http.Error(w, "Invalid training type", http.StatusBadRequest)
		return
	}

	// Get users from database
	users, err := getWeightTrainingUsersFromDB()
	if err != nil {
		log.Printf("Error getting weight training users: %v", err)
		http.Error(w, "Error getting weight training data", http.StatusInternalServerError)
		return
	}

	// Create a data structure for the template
	data := struct {
		Users        []*WeightTrainingUser
		TrainingType string
	}{
		Users:        users,
		TrainingType: trainingType,
	}
	// Define template functions
	funcMap := template.FuncMap{
		"makeExerciseMap": func(exercises []WeightTrainingExercise) map[string][]WeightTrainingExercise {
			result := make(map[string][]WeightTrainingExercise)
			for _, ex := range exercises {
				result[ex.Name] = append(result[ex.Name], ex)
			}
			return result
		},
		"toWeightTrainingType": func(s string) WeightTrainingType {
			return WeightTrainingType(s)
		},
	}

	// Parse and execute the template
	tmpl, err := template.New("weight_training_progress.html").Funcs(funcMap).ParseFiles("templates/weight_training_progress.html")
	if err != nil {
		log.Printf("Error loading weight training progress template: %v", err)
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error rendering weight training progress template: %v", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
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

	// Handle weight training exercise logging
	http.HandleFunc("/log-weight-training", logWeightTrainingExercise)

	// Handle the weight training progress page
	http.HandleFunc("/weight-training", getWeightTrainingProgress)

	fmt.Println("Server starting on port 8181...")
	log.Fatal(http.ListenAndServe(":8181", nil))
}
