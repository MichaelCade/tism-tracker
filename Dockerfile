# Stage 1 - Build the Go application
FROM golang:1.18-alpine AS build
# Set working directory
WORKDIR /app
# Copy go.mod and go.sum, install dependencies
COPY go.mod go.sum ./
RUN go mod download
# Copy the source code
COPY . .
# Build the application
RUN go build -o tism-tracker main.go
# Stage 2 - Create the final container
FROM alpine:latest
# Copy the built binary from the builder stage
COPY --from=build /app/tism-tracker /tism-tracker
COPY --from=build /app/templates /templates
COPY --from=build /app/static /static
# Expose the port
EXPOSE 8080
# Run the application
CMD ["/tism-tracker"]