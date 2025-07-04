#!/bin/bash

# Check if local registry container is running
if ! docker ps --filter "name=local-registry" --format "{{.Names}}" | grep -q "^local-registry$"; then
    echo "✓ Local docker registry is not running"
    exit 0
fi

# Stop the local registry container
echo "Stopping local docker registry..."
docker stop local-registry > /dev/null

# Wait for shutdown
echo "Waiting for local docker registry to shut down..."
timeout=5
elapsed=0

while [ $elapsed -lt $timeout ]; do
    if ! docker ps --filter "name=local-registry" --format "{{.Names}}" | grep -q "^local-registry$"; then
        echo "✓ Local docker registry has been shut down"
        break
    fi

    sleep 1
    elapsed=$((elapsed + 1))
done

if [ $elapsed -ge $timeout ]; then
    echo "⚠ Warning: Local docker registry may not have shut down"
fi

# Remove the container
echo "Removing local docker registry container..."
docker rm local-registry > /dev/null

# Verify the container was successfully removed
if ! docker ps -a --filter "name=local-registry" --format "{{.Names}}" | grep -q "^local-registry$"; then
    echo "✓ Local docker registry container has been stopped"
    exit 0
else
    echo "✗ Error: Failed to remove local docker registry container"
    exit 1
fi
