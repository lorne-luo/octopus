#!/bin/bash

cd web && pnpm install 
NEXT_PUBLIC_API_BASE_URL="http://127.0.0.1:8080" pnpm run dev&
FRONTEND_PID=$!

cd ..
## Open a new terminal, start the backend service
go run main.go start &
BACKEND_PID=$!

# Wait for both processes
wait $BACKEND_PID $FRONTEND_PID