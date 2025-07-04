#!/bin/bash

# Check if Prometheus container is already running
if docker ps --filter "name=prometheus" --format "{{.Names}}" | grep -q "^prometheus$"; then
    echo "✓ Prometheus server is already running"
    exit 0
fi

# Start Prometheus in the background
echo "Starting Prometheus server..."
docker run --name prometheus --network host -p 9090:9090 -v ~/calico/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus 2>&1 &

# Wait for Prometheus to be ready
echo "Waiting for Prometheus server to start..."
timeout=5
elapsed=0

while [ $elapsed -lt $timeout ]; do
        if docker logs prometheus 2>&1 | grep -q "Server is ready to receive web requests."; then
                echo "✓ Prometheus server is ready"
                exit 0
        fi

        sleep 1
        elapsed=$((elapsed + 1))
done

echo "✗ Error: Prometheus server failed to start"
exit 1
