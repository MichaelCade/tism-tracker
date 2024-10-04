# Stage 1 - Build the Go application
FROM golang:1.18-alpine AS build
# Set working directory
WORKDIR /app
# Copy go.mod, install dependencies
COPY go.mod ./
RUN go mod download
# Copy the source code
COPY . .
# Build the application
RUN go build -o tism-tracker main.go

# Stage 2 - Create the final container
FROM alpine:3.14
# Install necessary runtime dependencies
RUN apk --no-cache add ca-certificates
# Copy the built binary from the builder stage
COPY --from=build /app/tism-tracker /tism-tracker
COPY --from=build /app/templates /templates
COPY --from=build /app/static /static
# Expose the port
EXPOSE 8080
# Run the application
CMD ["/tism-tracker"]