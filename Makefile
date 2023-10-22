#!/usr/bin/make

build:
	docker build -t akatsuki-api:latest .

run-api:
	docker run --network
