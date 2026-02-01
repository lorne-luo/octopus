#!/bin/bash

rm -rf build
bash scripts/build.sh build linux arm64
mkdir -p build/docker/linux/arm64/
mv build/bin/octopus-linux-arm64 build/docker/linux/arm64/octopus

docker compose down
docker rmi lorne/octopus
docker compose -f docker-compose-lorne.yml up -d
git co internal/price/presets.go static/out/README.md