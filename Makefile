.PHONY: install start build daemon daemon-stop daemon-status daemon-log

install:
	npm install

start:
	npm start

build:
	go build -o xmuggled ./cmd/xmuggled/

daemon: build
	./xmuggled start

daemon-stop:
	./xmuggled stop

daemon-status:
	./xmuggled status

daemon-log:
	./xmuggled log 50
