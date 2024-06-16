# FROM golang:alpine as builder

# # RUN mkdir /build
# WORKDIR /build
# # ADD . /build/
# COPY . .

# RUN apk add git
# RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -a -installsuffix cgo -ldflags '-extldflags "-static"' -o blocproxy ./cmd

# FROM scratch

# COPY --from=builder /build/blocproxy /app/
# WORKDIR /app
# EXPOSE 62081
# CMD ["./blocproxy"]


# First stage: build the Go application
FROM golang:alpine as builder

# Set the working directory
WORKDIR /build

# Copy the source code
COPY . .

# Install git and iproute2
RUN apk add --no-cache git iproute2

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -a -installsuffix cgo -ldflags '-extldflags "-static"' -o blocproxy ./cmd

# Second stage: create a minimal image with the built Go binary and necessary dependencies
FROM alpine

# Copy the built binary from the builder stage
COPY --from=builder /build/blocproxy /app/blocproxy

# Install iproute2 in the final stage to get ss command
RUN apk add --no-cache iproute2

# Set the working directory
WORKDIR /app

# Expose the necessary port
EXPOSE 62081

# Run the Go application
CMD ["./blocproxy"]