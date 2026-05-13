# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/agent ./cmd/agent

# Runtime stage
FROM ubuntu:24.04

# Install common development tools
RUN apt-get update && apt-get install -y -- no-install-recommends \
    git \
    curl \
    wget \
    make \
    gcc \
    g++ \
    python3 \
    python3-pip \
    python3-venv \
    nodejs \
    npm \
    ca-certificates \
    jq \
    ripgrep \
    && rm -rf /var/lib/apt/lists/*

# Install Go
RUN curl -fsSL https://go.dev/dl/go1.22.5.linux-amd64.tar.gz | tar -C /usr/local -xzf -
ENV PATH="/usr/local/go/bin:${PATH}"

# Copy the agent binary
COPY --from=builder /build/agent /usr/local/bin/agent

# Create workspace and skills directories
RUN mkdir -p /workspace /skills

# Set workspace as working directory
WORKDIR /workspace

# Default environment variables
ENV WORKSPACE_PATH=/workspace
ENV SKILLS_PATH=/skills
ENV TASK_FILE=/workspace/.task.json
ENV MAX_ITERATIONS=50
ENV TOOL_TIMEOUT=60
# GIT_TOKEN, REPO_URL, BASE_BRANCH set at runtime (never bake secrets into images)

ENTRYPOINT ["/usr/local/bin/agent"]