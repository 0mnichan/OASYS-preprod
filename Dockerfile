# Stage 1: Build the Go application
FROM golang:1.23 AS builder

# Install necessary build tools
RUN apt-get update && apt-get install -y git build-essential && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy only go.mod and go.sum initially to cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the Go binary
RUN go build -o main .
