.PHONY: build docker-build docker-compose-up

build:
	go build .

docker-build:
	docker build .

docker-compose-up:
	docker-compose up --build --force-recreate

docker-compose-up-v2:
	docker-compose -f docker-compose-v2.yaml up --build --force-recreate
