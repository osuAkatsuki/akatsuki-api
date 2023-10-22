#!/usr/bin/make

build:
	docker build -t akatsuki-api:latest .

run-api:
	docker run \
		--env APP_COMPONENT=api \
		--network=host \
		--env-file=.env \
		-it akatsuki-api:latest
