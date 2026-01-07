# Build stage
FROM node:22-alpine AS builder

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm ci

# Copy source
COPY . .

# Build production bundle
RUN npm run build

# Runtime stage - serve with lightweight server
FROM node:22-alpine

WORKDIR /app

# Install serve globally
RUN npm install -g serve

# Copy built assets from builder
COPY --from=builder /app/dist ./dist

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

EXPOSE 3000

# Serve static files
CMD ["serve", "-s", "dist", "-l", "3000"]
