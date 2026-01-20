#!/bin/bash

docker compose down
docker rmi lorne/octopus
docker compose up -d
