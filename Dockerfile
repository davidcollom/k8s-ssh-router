# Use the official Go image as the build environment
FROM golang:1.24 as builder

# Set the working directory inside the container
WORKDIR /app

# Copy the Go Modules manifests
COPY go.mod go.sum ./

# Download the dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN go build -o k8s-ssh-router ./cmd

# Use a minimal base image for the final image
FROM alpine:latest

# Set the working directory inside the container
WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/k8s-ssh-router .

# Command to run the binary
CMD ["./k8s-ssh-router"]
