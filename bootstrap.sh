#!/bin/bash
set -e

CONFIG_FILE="config.yaml"
COMPOSE_FILE="docker/docker-compose.yml"

echo "=== homeGPT Bootstrap ==="
echo "Reading config from $CONFIG_FILE..."
echo ""

# Function to wait for vLLM health endpoint
wait_for_health() {
    local container=$1
    local max_attempts=180  # 180 * 5s = 15 minutes max
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
# Get models by startup_mode
ACTIVE_MODEL=$(yq -r '.models[] | select(.startup_mode == "active") | .id' "$CONFIG_FILE")
ACTIVE_CONTAINER=$(yq -r '.models[] | select(.startup_mode == "active") | .container_name' "$CONFIG_FILE")

# Get sleep and disabled models
SLEEP_MODELS=$(yq -r '.models[] | select(.startup_mode == "sleep") | .id' "$CONFIG_FILE")
SLEEP_CONTAINERS=$(yq -r '.models[] | select(.startup_mode == "sleep") | .container_name' "$CONFIG_FILE")

echo "Active model: $ACTIVE_MODEL ($ACTIVE_CONTAINER)"
echo "Sleep models: $SLEEP_MODELS"
echo ""

# Clean up
echo "Cleaning up existing containers..."
docker compose -f "$COMPOSE_FILE" down

# Check if there are any sleep models
if [ -z "$SLEEP_CONTAINERS" ]; then
    # No sleep models - just start the active model directly
    echo ""
    echo "=== Starting active model (no sleep models to cache) ==="
    echo "Starting $ACTIVE_CONTAINER..."
    docker compose -f "$COMPOSE_FILE" up -d "$ACTIVE_CONTAINER"
    wait_for_health "$ACTIVE_CONTAINER"
else
    # Multiple models - load and cache all, then wake active
    # Collect all containers that need to be started (active + sleep)
    ALL_CONTAINERS="$ACTIVE_CONTAINER $SLEEP_CONTAINERS"

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
            curl -s -X POST "http://localhost:$host_port/sleep" -H "Content-Type: application/json" -d '{"level": 1}' > /dev/null || echo "   (sleep command sent)"
            sleep 3
        fi
        
        step=$((step + 1))
    done

    # Step 2: Wake up only the active model
    echo ""
    echo "=== Phase 2: Activating default model ==="
    ACTIVE_PORT=$(docker compose -f "$COMPOSE_FILE" port "$ACTIVE_CONTAINER" 8000 2>/dev/null | cut -d: -f2 || echo "")
    if [ -n "$ACTIVE_PORT" ]; then
        echo "   Waking up $ACTIVE_CONTAINER (port $ACTIVE_PORT)..."
        curl -s -X POST "http://localhost:$ACTIVE_PORT/wake_up" > /dev/null || echo "   (wake_up command sent)"
        sleep 3
        wait_for_health "$ACTIVE_CONTAINER"
    fi
fi

# Start model manager (will resync state)
echo ""
echo "=== Starting management services ==="
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
echo "Active model: $ACTIVE_MODEL ($ACTIVE_CONTAINER)"
