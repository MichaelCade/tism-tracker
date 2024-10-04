# TISM Tracker

TISM Tracker is a web application for tracking walking and running activities. The application allows users to log their distances and view their progress towards a goal.

## Table of Contents

- Features
- Project Structure
- Setup and Installation
- Running the Application
- Using Docker
- Environment Variables
- Database Setup
- Contributing
- License

## Features

- Log walking and running distances
- View progress towards a goal
- Supports both in-memory and external database storage

## Project Structure

.
├── Dockerfile
├── go.mod
├── go.sum
├── main.go
├── static/
│   └── style.css
├── templates/
│   ├── index.html
│   └── progress.html
└── README.md

## Setup and Installation

### Prerequisites

- Go 1.18 or later
- Docker (optional, for containerized deployment)
- PostgreSQL (for external database storage)

### Installation

1. Clone the repository:

   git clone https://github.com/yourusername/tism-tracker.git
   cd tism-tracker

2. Install dependencies:

   go mod download

## Running the Application

### In-Memory Version

To run the in-memory version of the application:

1. Comment out the database-related code in main.go.
2. Run the application:

   go run main.go

### External Database Version

To run the version of the application that uses an external PostgreSQL database:

1. Set up the PostgreSQL database (see Database Setup).
2. Ensure the DATABASE_URL environment variable is set.
3. Run the application:

   go run main.go

## Using Docker

### Building the Docker Image

To build the Docker image:

   docker build -t tism-tracker .

### Running the Docker Container

To run the Docker container:

   docker run -d -p 8080:8080 --name tism-tracker -e DATABASE_URL="postgres://username:password@host:port/dbname?sslmode=disable" tism-tracker

### Multi-Platform Build and Push

To build and push the Docker image for multiple platforms:

   docker buildx build --platform linux/amd64,linux/arm64 -t yourusername/tism-tracker:latest --push .

## Environment Variables

- DATABASE_URL: The connection string for the PostgreSQL database.

Example:

   export DATABASE_URL="postgres://username:password@host:port/dbname?sslmode=disable"

## Database Setup

1. Install PostgreSQL.
2. Create a new database and user.
3. Update the DATABASE_URL environment variable with the connection string.

Example:

   psql -U postgres
   CREATE DATABASE tism_tracker;
   CREATE USER tism_user WITH ENCRYPTED PASSWORD 'yourpassword';
   GRANT ALL PRIVILEGES ON DATABASE tism_tracker TO tism_user;

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## License

This project is licensed under the MIT License.