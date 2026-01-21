#!/bin/bash

# Trap SIGINT and SIGTERM to kill background processes
cleanup() {
    echo "Stopping services..."
    kill $FRONTEND_PID $BACKEND_PID 2>/dev/null
    wait $FRONTEND_PID $BACKEND_PID 2>/dev/null
    exit
}
trap cleanup SIGINT SIGTERM

cd web && pnpm install && pnpm run build && cd ..
rm -rf static/out
mv web/out static/


cd web && pnpm install 
pnpm run dev -p 9100 &
FRONTEND_PID=$!

cd ..
## Open a new terminal, start the backend service
go run main.go start &
BACKEND_PID=$!

# Wait for both processes
wait $BACKEND_PID $FRONTEND_PID