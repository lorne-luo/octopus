#!/bin/bash

set -e

cd web && pnpm install && pnpm run build && cd ..
rm -rf static/out
mv -f web/out static/
git co internal/price/presets.go static/out/README.md

## Open a new terminal, start the backend service
go run main.go start

