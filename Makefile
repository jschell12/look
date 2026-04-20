.PHONY: install start daemon daemon-stop daemon-status daemon-log

install:
	npm install

start:
	npm start

daemon:
	node daemon.js start

daemon-stop:
	node daemon.js stop

daemon-status:
	node daemon.js status

daemon-log:
	node daemon.js log 50
