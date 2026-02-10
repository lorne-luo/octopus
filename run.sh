#!/bin/bash
set -e

# Build Frontend
echo "Building Frontend..."
cd web
pnpm install
pnpm build
cd ..
rm -rf static/out
mv -f web/out static/
git co static/out/README.md

# Start Backend
echo "Starting Backend..."
go run main.go start
