.PHONY: build install uninstall-tool daemon-install daemon-uninstall daemon-start daemon-stop daemon-logs link clean

DAEMON_PLIST := $(HOME)/Library/LaunchAgents/com.xmuggle.daemon.plist
INSTALL_PREFIX ?= /usr/local

build:
	@mkdir -p bin
	go build -o bin/xmuggle ./cmd/xmuggle
	go build -o bin/xmuggled ./cmd/xmuggled

# Install binaries + /xmuggle skill for Claude/Cursor
install: build
	bash scripts/install-skill.sh

# Install the queue-processing daemon (launchd)
daemon-install: build
	bash scripts/install-daemon.sh

daemon-uninstall:
	launchctl unload $(DAEMON_PLIST) 2>/dev/null || true
	rm -f $(DAEMON_PLIST)

daemon-start:
	launchctl load $(DAEMON_PLIST)

daemon-stop:
	launchctl unload $(DAEMON_PLIST)

daemon-logs:
	tail -f ~/.xmuggle/logs/daemon.stdout.log

# Interactive LAN discovery + tunnel/push/pull
link:
	bash scripts/mac-link.sh

clean:
	rm -rf bin

# TEMP: one-shot cleanup of both old 'look' and current 'xmuggle' installs.
# Remove once the old install is gone.
uninstall-tool:
	@echo "Stopping any running daemons..."
	-launchctl unload $(HOME)/Library/LaunchAgents/com.look.daemon.plist 2>/dev/null
	-launchctl unload $(HOME)/Library/LaunchAgents/com.xmuggle.daemon.plist 2>/dev/null
	@echo "Removing launchd plists..."
	-rm -f $(HOME)/Library/LaunchAgents/com.look.daemon.plist
	-rm -f $(HOME)/Library/LaunchAgents/com.xmuggle.daemon.plist
	@echo "Removing binaries from common locations..."
	-rm -f $(HOME)/.local/bin/look $(HOME)/.local/bin/lookd
	-rm -f $(HOME)/.local/bin/xmuggle $(HOME)/.local/bin/xmuggled
	-rm -f $(HOME)/bin/look $(HOME)/bin/lookd
	-rm -f $(HOME)/bin/xmuggle $(HOME)/bin/xmuggled
	-rm -f /opt/homebrew/bin/look /opt/homebrew/bin/lookd
	-rm -f /opt/homebrew/bin/xmuggle /opt/homebrew/bin/xmuggled
	-rm -f /usr/local/bin/look /usr/local/bin/lookd
	-rm -f /usr/local/bin/xmuggle /usr/local/bin/xmuggled
	@echo "Removing Claude/Cursor skills..."
	-rm -rf $(HOME)/.claude/skills/look $(HOME)/.claude/skills/xmuggle
	-rm -f $(HOME)/.cursor/commands/look.md $(HOME)/.cursor/commands/xmuggle.md
	@echo ""
	@echo "Done. Data directory preserved at ~/.look and/or ~/.xmuggle."
	@echo "To also remove all data: rm -rf ~/.look ~/.xmuggle"
