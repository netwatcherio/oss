#!/bin/bash
git pull

docker build -t netwatcher-controller ./controller/
docker build -t netwatcher-panel ./panel/

docker compose up -d