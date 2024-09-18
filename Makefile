.PHONY: build docker-build docker-compose-up

build:
	go build .

docker-build:
	docker build .

docker-compose-up:
	docker-compose up --build --force-recreate
