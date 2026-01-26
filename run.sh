#!/bin/bash

cd web && pnpm install && pnpm run build && cd ..
mv -f web/out static/

## Open a new terminal, start the backend service
go run main.go start

