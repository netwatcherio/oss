# Use an official Go runtime as a parent image
FROM golang:1.21.1-alpine

# Set the working directory inside the container
WORKDIR /app

# Copy the local package files to the container's workspace
COPY . /app

# Download dependencies
RUN go mod download

# Build the Go app
RUN go build -o main .

# Expose port 8080 to the outside world
EXPOSE 8080

# Declare environment variables with sensible defaults.
# The actual values will be injected by docker-compose or docker run
ENV DEBUG=true
ENV LISTEN=0.0.0.0:8080
ENV KEY=${NW_GUARDIAN_KEY}

ENV POSTGRES_USER=${POSTGRES_USER}
ENV POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
ENV POSTGRES_DB=${POSTGRES_DB}

# Optional extras if you need them:
# ENV MONGO_URI=${NW_GUARDIAN_MONGO_URI}
# ENV MAIN_DB=${NW_GUARDIAN_MAIN_DB}

# Command to run the executable
CMD ["/app/main"]