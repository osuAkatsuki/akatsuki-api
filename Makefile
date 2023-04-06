#!/usr/bin/make

build:
	docker build -t akatsuki-api:latest -t registry.digitalocean.com/akatsuki/akatsuki-api:latest .

push:
	docker push registry.digitalocean.com/akatsuki/akatsuki-api:latest

install:
	helm install --values chart/values.yaml akatsuki-api-staging ../common-helm-charts/microservice-base/

uninstall:
	helm uninstall akatsuki-api-staging

diff-upgrade:
	helm diff upgrade --allow-unreleased --values chart/values.yaml akatsuki-api-staging ../common-helm-charts/microservice-base/

upgrade:
	helm upgrade --atomic --values chart/values.yaml akatsuki-api-staging ../common-helm-charts/microservice-base/
