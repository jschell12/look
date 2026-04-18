.PHONY: build install install-skill uninstall-tool daemon-install daemon-uninstall daemon-start daemon-stop daemon-logs daemon-wake-schedule daemon-wake-unschedule link clean

DAEMON_PLIST := $(HOME)/Library/LaunchAgents/com.xmuggle.daemon.plist
INSTALL_PREFIX ?= /usr/local

# Sleep behavior for the daemon.
#   default — daemon sleeps when the Mac sleeps (normal)
#   awake   — wrap the daemon in `caffeinate -i` to prevent idle sleep
SLEEP_MODE ?= default

# Wake-from-sleep schedule (via `pmset repeat wakeorpoweron`). Used by
# `daemon-wake-schedule`. WAKE_TIMES must be HH:MM:SS; WAKE_DAYS is the pmset
# weekday mask (MTWRFSU = every day, MTWRF = weekdays only, SU = weekends).
WAKE_DAYS ?= MTWRFSU
WAKE_TIMES ?=

build:
	@mkdir -p bin
	go build -o bin/xmuggle ./cmd/xmuggle
	go build -o bin/xmuggled ./cmd/xmuggled

# Install binaries + /xmuggle skill for Claude/Cursor
install: build
	bash scripts/install-skill.sh

# Install just the /xmuggle skill files (no build, no binary install).
# Useful for updating skill text without rebuilding.
install-skill:
	@if [ -d $(HOME)/.claude ]; then \
		mkdir -p $(HOME)/.claude/skills/xmuggle; \
		cp skills/claude/SKILL.md $(HOME)/.claude/skills/xmuggle/SKILL.md; \
		echo "  ~/.claude/skills/xmuggle/SKILL.md"; \
	fi
	@if [ -d $(HOME)/.cursor ]; then \
		mkdir -p $(HOME)/.cursor/commands; \
		cp skills/cursor/command.md $(HOME)/.cursor/commands/xmuggle.md; \
		echo "  ~/.cursor/commands/xmuggle.md"; \
	fi

# Install the queue-processing daemon (launchd).
# Pass SLEEP_MODE=awake to wrap the daemon in `caffeinate -i`.
daemon-install: build
	SLEEP_MODE=$(SLEEP_MODE) bash scripts/install-daemon.sh

daemon-uninstall:
	launchctl unload $(DAEMON_PLIST) 2>/dev/null || true
	rm -f $(DAEMON_PLIST)

daemon-start:
	launchctl load $(DAEMON_PLIST)

daemon-stop:
	launchctl unload $(DAEMON_PLIST)

daemon-logs:
	tail -f ~/.xmuggle/logs/daemon.stdout.log

# Schedule a daily wake-from-sleep so the daemon can process queued tasks even
# when the lid is closed / machine is asleep. Uses `pmset repeat wakeorpoweron`,
# which requires sudo and supports one recurring time per day.
#
# Usage: sudo make daemon-wake-schedule WAKE_TIMES=09:00:00 [WAKE_DAYS=MTWRFSU]
daemon-wake-schedule:
	@if [ -z "$(WAKE_TIMES)" ]; then \
		echo "Usage: sudo make daemon-wake-schedule WAKE_TIMES=HH:MM:SS [WAKE_DAYS=MTWRFSU]"; \
		echo ""; \
		echo "  WAKE_TIMES   single time of day in 24h HH:MM:SS (pmset repeat limitation)"; \
		echo "  WAKE_DAYS    weekday mask: MTWRFSU=every day, MTWRF=weekdays, SU=weekends"; \
		exit 1; \
	fi
	sudo pmset repeat wakeorpoweron $(WAKE_DAYS) $(WAKE_TIMES)

daemon-wake-unschedule:
	sudo pmset repeat cancel

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
