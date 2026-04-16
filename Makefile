.PHONY: build setup i-wm i-pm daemon-start daemon-stop daemon-logs watcher-start watcher-stop watcher-logs clean

DAEMON_PLIST := $(HOME)/Library/LaunchAgents/com.screenshot-agent.daemon.plist
WATCHER_PLIST := $(HOME)/Library/LaunchAgents/com.screenshot-agent.watcher.plist

build:
	pnpm build

setup:
	bash scripts/setup.sh

# Install on work machine (sender + file watcher)
i-wm: build
	bash scripts/install-wm.sh

# Install on personal machine (receiver/processor daemon)
i-pm: build
	bash scripts/install-pm.sh

# Daemon controls (personal machine)
daemon-start:
	launchctl load $(DAEMON_PLIST)

daemon-stop:
	launchctl unload $(DAEMON_PLIST)

daemon-logs:
	tail -f ~/.screenshot-agent/logs/daemon.stdout.log

# Watcher controls (work machine)
watcher-start:
	launchctl load $(WATCHER_PLIST)

watcher-stop:
	launchctl unload $(WATCHER_PLIST)

watcher-logs:
	tail -f ~/.screenshot-agent/logs/watcher.log

clean:
	rm -rf dist
