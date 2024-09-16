# Use the official Golang image as the base image
FROM golang:1.23.1-alpine3.20 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy the Go module files
COPY go.mod go.sum ./

# Download the Go dependencies
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go application
RUN go build -o inmate .

# Use a minimal Alpine image as the base image for the final image
FROM alpine:latest

# Set the working directory inside the container
WORKDIR /app

# Copy the built executable from the builder stage
COPY --from=builder /app/inmate .

# Expose the port that the application listens on
EXPOSE 8080

# Set the command to run the executable
CMD ["./inmate"]