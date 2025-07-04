#!/bin/bash

# Set up directories and registry variables
REGISTRY_DIR="/tmp/docker_registry"
REGISTRY_URL="localhost:5000"

# Check if local registry container is already running
if docker ps --filter "name=local-registry" --format "{{.Names}}" | grep -q "^local-registry$"; then
    echo "✓ Local docker registry is already running"
    exit 0
fi

# Check if local registry container exists but is stopped
if docker ps -a --filter "name=local-registry" --format "{{.Names}}" | grep -q "^local-registry$"; then
    echo "Local docker registry container exists but is stopped. Starting it..."
    docker start local-registry > /dev/null
else
    # Create registry data directory
    mkdir -p "$REGISTRY_DIR"

    # Start local docker registry in the background
    echo "Starting local docker registry on $REGISTRY_URL..."
    docker run -d --name local-registry \
        -p "5000:5000" \
        -e REGISTRY_STORAGE_DELETE_ENABLED=true \
        -v "$REGISTRY_DIR:/var/lib/registry" \
        registry:2 > /dev/null
fi

# Wait for local docker registry to be ready
echo "Waiting for local docker registry to start..."
timeout=10
elapsed=0

while [ $elapsed -lt $timeout ]; do
        if docker logs local-registry 2>&1 | grep -q "listening on"; then
                echo "✓ local docker registry is ready at http://$REGISTRY_URL"

                # Connect to kind network if kind cluster exists
                if docker network ls | grep -q "kind"; then
                    echo "Connecting registry to kind network..."
                    docker network connect kind local-registry 2>/dev/null || true
                    echo "✓ Registry connected to kind network"
                fi
                exit 0
        fi
        sleep 1
        elapsed=$((elapsed + 1))
done

echo "✗ Error: local docker registry failed to start"
exit 1
