#!/bin/bash
set -e

CONFIG_FILE="model-manager/config.yaml"
COMPOSE_FILE="docker/docker-compose.yml"

echo "=== homeGPT Bootstrap ==="
echo "Reading config from $CONFIG_FILE..."
echo ""

# Function to wait for vLLM health endpoint
wait_for_health() {
    local container=$1
    local max_attempts=90  # 90 * 5s = 7.5 minutes max
    local attempt=0
    
    # Get the host port for this container
    local host_port=$(docker compose -f "$COMPOSE_FILE" port "$container" 8000 2>/dev/null | cut -d: -f2 || echo "")
    
    if [ -z "$host_port" ]; then
        echo "   ERROR: Could not determine port for $container"
        return 1
    fi
    
    echo "   Waiting for health check on port $host_port ..."
    while [ $attempt -lt $max_attempts ]; do
        if curl -s http://localhost:$host_port/health > /dev/null 2>&1; then
            echo ""
            echo "   ✓ Model is ready!"
            return 0
        fi
        attempt=$((attempt + 1))
        echo -n "."
        sleep 5  # Check every 5 seconds instead of 2
    done
    echo ""
    echo "   ✗ Timeout waiting for model to be healthy"
    return 1
}

# Parse config using yq (apt version - jq wrapper for YAML)
DEFAULT_MODEL=$(yq -r '.models[] | select(.default == true) | .id' "$CONFIG_FILE")
DEFAULT_CONTAINER=$(yq -r '.models[] | select(.default == true) | .container_name' "$CONFIG_FILE")

# Get all non-default models
NON_DEFAULT_MODELS=$(yq -r '.models[] | select(.default == false) | .id' "$CONFIG_FILE")
NON_DEFAULT_CONTAINERS=$(yq -r '.models[] | select(.default == false) | .container_name' "$CONFIG_FILE")

echo "Default model: $DEFAULT_MODEL ($DEFAULT_CONTAINER)"
echo "Non-default models: $NON_DEFAULT_MODELS"
echo ""

# Clean up
echo "Cleaning up existing containers..."
docker compose -f "$COMPOSE_FILE" down

# Collect all containers (default + non-default)
ALL_CONTAINERS="$DEFAULT_CONTAINER $NON_DEFAULT_CONTAINERS"

# Step 1: Load each model one at a time, then immediately sleep it
echo ""
echo "=== Phase 1: Loading and caching all models (one at a time) ==="
step=1
for container in $ALL_CONTAINERS; do
    echo ""
    echo "$step. Starting $container..."
    docker compose -f "$COMPOSE_FILE" up -d "$container"
    wait_for_health "$container"
    
    # Immediately put it to sleep to free VRAM for next model
    host_port=$(docker compose -f "$COMPOSE_FILE" port "$container" 8000 2>/dev/null | cut -d: -f2 || echo "")
    if [ -n "$host_port" ]; then
        echo "   Putting $container to sleep to free VRAM..."
        curl -s -X POST "http://localhost:$host_port/sleep" > /dev/null || echo "   (sleep command sent)"
        sleep 3
    fi
    
    step=$((step + 1))
done

# Step 2: Wake up only the default model
echo ""
echo "=== Phase 2: Activating default model ==="
DEFAULT_PORT=$(docker compose -f "$COMPOSE_FILE" port "$DEFAULT_CONTAINER" 8000 2>/dev/null | cut -d: -f2 || echo "")
if [ -n "$DEFAULT_PORT" ]; then
    echo "   Waking up $DEFAULT_CONTAINER (port $DEFAULT_PORT)..."
    curl -s -X POST "http://localhost:$DEFAULT_PORT/wake_up" > /dev/null || echo "   (wake_up command sent)"
    sleep 3
    wait_for_health "$DEFAULT_CONTAINER"
fi

# Start model manager (will resync state)
echo ""
echo "=== Phase 3: Starting management services ==="
echo "Starting Model Manager..."
docker compose -f "$COMPOSE_FILE" up -d model-manager
sleep 5

# Start WebUI
echo "Starting WebUI..."
docker compose -f "$COMPOSE_FILE" up -d webui

echo ""
echo "=== Done ==="
echo "WebUI: http://localhost:3000"
echo "Model Manager: http://localhost:9000"
echo ""
echo "Active model: $DEFAULT_MODEL ($DEFAULT_CONTAINER)"
