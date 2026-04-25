.PHONY: install run run-daemon build pull daemon-stop daemon-restart daemon-status daemon-log

INSTALL_DIR := $(HOME)/.local/bin
LAUNCHD_LABEL := com.xmuggle.daemon

install: pull build
	npm install
	install -d $(INSTALL_DIR)
	install -m 0755 xmuggled $(INSTALL_DIR)/xmuggled
	launchctl kill SIGTERM gui/$(shell id -u)/$(LAUNCHD_LABEL) 2>/dev/null || true

pull:
	git pull --rebase

run:
	npm start

run-daemon:
	$(INSTALL_DIR)/xmuggled start

build:
	go build -o xmuggled ./cmd/xmuggled/

daemon-stop:
	$(INSTALL_DIR)/xmuggled stop

daemon-restart:
	-$(INSTALL_DIR)/xmuggled stop 2>/dev/null
	-rm -f $(HOME)/.xmuggle/daemon.pid
	$(INSTALL_DIR)/xmuggled start

daemon-status:
	$(INSTALL_DIR)/xmuggled status

log:
	bash scripts/tail-all.sh

daemon-log:
	tail -f $(HOME)/.xmuggle/daemon.log

claude-log:
	@latest=$$(ls -t $(HOME)/.xmuggle/claude-*.log 2>/dev/null | head -1); \
	if [ -z "$$latest" ]; then echo "No claude logs found"; exit 1; fi; \
	echo "Tailing $$latest"; \
	tail -f "$$latest" | python3 scripts/claude-log-filter.py
