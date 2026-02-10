#!/bin/bash
set -e

# Build Frontend
echo "Building Frontend..."
cd web
pnpm install
pnpm build
cd ..

# Start Backend
echo "Starting Backend..."
go run main.go start
