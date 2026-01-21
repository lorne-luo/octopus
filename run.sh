#!/bin/bash

cd web && pnpm install && pnpm run build && cd ..
rm -rf static/out
mv web/out static/

## Open a new terminal, start the backend service
go run main.go start