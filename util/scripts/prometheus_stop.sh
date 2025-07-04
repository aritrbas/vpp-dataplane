#!/bin/bash

# Check if Prometheus container is running
if ! docker ps --filter "name=prometheus" --format "{{.Names}}" | grep -q "^prometheus$"; then
    echo "✓ Prometheus server is not running"
    exit 0
fi

# Stop the Prometheus container
echo "Stopping Prometheus server..."
docker stop prometheus > /dev/null

# Wait for shutdown
echo "Waiting for Prometheus server to shut down..."
timeout=5
elapsed=0

while [ $elapsed -lt $timeout ]; do
    if docker logs prometheus 2>&1 | grep -q "See you next time!"; then
        echo "✓ Prometheus server has been shut down"
        break
    fi

    sleep 1
    elapsed=$((elapsed + 1))
done

if [ $elapsed -ge $timeout ]; then
    echo "⚠ Warning: Prometheus server may not have shut down"
fi

# Remove the container
echo "Removing Prometheus server container..."
docker rm prometheus > /dev/null

# Verify the container was successfully removed
if ! docker ps -a --filter "name=prometheus" --format "{{.Names}}" | grep -q "^prometheus$"; then
    echo "✓ Prometheus server container has been stopped"
    exit 0
else
    echo "✗ Error: Failed to remove Prometheus server container"
    exit 1
fi
