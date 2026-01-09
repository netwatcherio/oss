# Build stage
FROM node:22-alpine AS builder

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm ci

# Copy source
COPY . .

# Build-time configuration for Vite (optional, runtime injection preferred)
ARG CONTROLLER_ENDPOINT
ENV CONTROLLER_ENDPOINT=${CONTROLLER_ENDPOINT}

# Build production bundle
RUN npm run build

# Runtime stage - serve with lightweight server
FROM node:22-alpine

WORKDIR /app

# Install serve globally
RUN npm install -g serve

# Copy built assets from builder
COPY --from=builder /app/dist ./dist

# Copy the entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

EXPOSE 3000

# Use entrypoint for runtime config injection
ENTRYPOINT ["/docker-entrypoint.sh"]
