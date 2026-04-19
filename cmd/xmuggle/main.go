// Command xmuggle is the primary CLI — runs a Claude Code agent against a
// screenshot to fix code in a target repo.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jschell12/xmuggle/internal/config"
	"github.com/jschell12/xmuggle/internal/daemoninstall"
	"github.com/jschell12/xmuggle/internal/gitops"
	"github.com/jschell12/xmuggle/internal/images"
	"github.com/jschell12/xmuggle/internal/prompt"
	"github.com/jschell12/xmuggle/internal/queue"
	"github.com/jschell12/xmuggle/internal/record"
	"github.com/jschell12/xmuggle/internal/spawn"
)

const usage = `xmuggle — screenshot-driven code fixes

Usage:
  xmuggle send [--screenshots] [--img <name>]... [--all] [--msg "context"]
  xmuggle list [--json]
  xmuggle <subcommand> [args]

Send (submit screenshots for fixing):
  send                     Send screenshot(s) for fixing
    --screenshots          Interactive picker: choose 1+ images from a list
    --img   <name>         Select image by fuzzy match (repeatable for multi-image)
    --all                  Process ALL unprocessed images
    --msg   <msg>          Context to guide the agent (what's wrong, what to fix)

List (view available images):
  list                     Show tracked Desktop images and their status
  list --json              Machine-readable JSON output (for AI agents)

Subcommands:
  init               Set up xmuggle in the current git repo
                     Creates .xmuggle/ directory, registers peer, installs daemon
    --no-daemon        Skip daemon installation

  peers              Show registered peers for this repo

  rec                Record screen at 1fps, optionally auto-submit
    --duration <dur>   Recording length (e.g. 30s, 2m). Default: Ctrl+C
    --fps <N>          Frames per second (default: 1)
    --format <fmt>     jpg (default) or png

  rm <name>...       Remove images from tracking
    --all-done         Remove all processed images

Image detection:
  Screenshots are auto-detected from ~/Desktop on every run.
  Tracked images are stored in .xmuggle/images.json (per-repo).

Examples:

  One-time setup (run from inside the repo on each machine):
    cd ~/dev/my-app
    xmuggle init

  Send screenshots:
    xmuggle send --msg "fix the button"                    # latest screenshot
    xmuggle send --screenshots                             # pick from list
    xmuggle send --img bug1 --img bug2                     # multi-image
    xmuggle send --all                                     # all pending

  Other:
    xmuggle list                                           # see pending
    xmuggle peers                                          # who's registered
    xmuggle rm "Screenshot 2026-04-12"                     # remove by name
    xmuggle rm --all-done                                  # remove all processed
`

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func mustRepoRoot() string {
	root, err := gitops.FindRepoRoot()
	if err != nil {
		die("not in a git repo; cd into a repo first")
	}
	return root
}

func mustHostname() string {
	h, _ := os.Hostname()
	return config.NormalizeHostname(h)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "-h", "--help":
		fmt.Print(usage)
	case "send":
		cmdSend(os.Args[2:])
	case "list":
		cmdList(os.Args[2:])
	case "init":
		cmdInit(os.Args[2:])
	case "peers":
		cmdPeers()
	case "rm":
		cmdRm(os.Args[2:])
	case "rec":
		cmdRec(os.Args[2:])
	default:
		// Try as send with implicit args
		cmdSend(os.Args[1:])
	}
}

// ────────────────────────────────────────────────────────────────────
// init
// ────────────────────────────────────────────────────────────────────

func cmdInit(args []string) {
	noDaemon := false
	for _, a := range args {
		if a == "--no-daemon" {
			noDaemon = true
		}
	}

	repoRoot := mustRepoRoot()
	hostname := mustHostname()

	// Create .xmuggle/ directories
	if err := config.EnsureRepoDirs(repoRoot); err != nil {
		die("create .xmuggle/: %v", err)
	}

	// Write config
	cfg := &config.Config{Version: 1, Hostname: hostname}
	if err := config.Save(repoRoot, cfg); err != nil {
		die("save config: %v", err)
	}

	// Write peer marker
	p := config.GetRepoPaths(repoRoot)
	peerFile := filepath.Join(p.PeersDir, hostname)
	if err := os.WriteFile(peerFile, []byte(""), 0o644); err != nil {
		die("write peer marker: %v", err)
	}

	// Write .xmuggle/.gitignore
	gitignore := filepath.Join(p.Root, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("images.json\nconfig.json\n"), 0o644); err != nil {
		die("write .gitignore: %v", err)
	}

	// Commit and push
	fmt.Println("Registering peer:", hostname)
	err := gitops.CommitAndPush(repoRoot, []string{".xmuggle/"}, fmt.Sprintf("xmuggle: register %s", hostname))
	if err != nil {
		fmt.Fprintf(os.Stderr, "note: could not push: %v\n", err)
	}

	// Register repo with daemon
	if err := config.RegisterRepo(repoRoot); err != nil {
		die("register repo: %v", err)
	}

	if noDaemon {
		fmt.Println("Skipping daemon install (--no-daemon)")
	} else {
		fmt.Println("Installing daemon...")
		if err := daemoninstall.Install(); err != nil {
			fmt.Fprintf(os.Stderr, "note: daemon install failed: %v\n", err)
			fmt.Fprintln(os.Stderr, "Fallback: run `make daemon-install` from the xmuggle repo.")
		} else {
			fmt.Println("Daemon installed and running.")
		}
	}

	fmt.Println()
	fmt.Println("Done! Run `xmuggle init` from any other machine that has this repo cloned.")
}

// ────────────────────────────────────────────────────────────────────
// send
// ────────────────────────────────────────────────────────────────────

type sendArgs struct {
	message     string
	imgs        []string
	all         bool
	screenshots bool
}

func parseSendArgs(args []string) *sendArgs {
	a := &sendArgs{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--msg":
			i++
			if i < len(args) {
				a.message = args[i]
			}
		case "--img":
			i++
			if i < len(args) {
				a.imgs = append(a.imgs, args[i])
			}
		case "--all":
			a.all = true
		case "--screenshots":
			a.screenshots = true
		case "--list":
			cmdList(nil)
			os.Exit(0)
		}
	}
	return a
}

func cmdSend(rawArgs []string) {
	a := parseSendArgs(rawArgs)
	repoRoot := mustRepoRoot()
	hostname := mustHostname()
	p := config.GetRepoPaths(repoRoot)

	// Select screenshots
	var shotPaths []string
	switch {
	case a.screenshots:
		shotPaths = promptSelectScreenshots()
	case len(a.imgs) > 0:
		for _, q := range a.imgs {
			img, err := images.FindByName(q)
			if err != nil || img == nil {
				die("no image matching %q (run 'xmuggle list')", q)
			}
			shotPaths = append(shotPaths, img.Path)
		}
	case a.all:
		ups, err := images.AllUnprocessed()
		if err != nil {
			die("find unprocessed: %v", err)
		}
		if len(ups) == 0 {
			die("No unprocessed images. Take a screenshot on ~/Desktop.")
		}
		for _, img := range ups {
			shotPaths = append(shotPaths, img.Path)
		}
		fmt.Printf("Found %d unprocessed image(s)\n", len(shotPaths))
	default:
		img, err := images.Latest()
		if err != nil {
			die("find latest: %v", err)
		}
		if img == nil {
			die("No unprocessed images. Take a screenshot on ~/Desktop.")
		}
		shotPaths = []string{img.Path}
	}

	var names []string
	for _, sp := range shotPaths {
		names = append(names, filepath.Base(sp))
	}
	fmt.Printf("Screenshot(s): %s\n", strings.Join(names, ", "))

	// Find peers
	target := selectPeer(p.PeersDir, hostname)
	fmt.Printf("Target peer: %s\n", target)
	if a.message != "" {
		fmt.Printf("Context: %s\n", a.message)
	}

	// Detect repo slug from remote
	remoteURL := gitops.RemoteURL(repoRoot)
	repoSlug := repoRoot
	if remoteURL != "" {
		repoSlug = remoteToSlug(remoteURL)
	}

	// Write task
	taskID := queue.NewTaskID()
	t := queue.Task{
		Repo:      repoSlug,
		Message:   a.message,
		Timestamp: time.Now().UnixMilli(),
		Status:    queue.StatusPending,
		Target:    target,
		Sender:    hostname,
	}

	var taskDir string
	var err error
	if len(shotPaths) == 1 {
		taskDir, err = queue.WriteTask(p.QueueDir, taskID, t, shotPaths[0])
	} else {
		taskDir, err = queue.WriteTaskMulti(p.QueueDir, taskID, t, shotPaths)
	}
	if err != nil {
		die("write task: %v", err)
	}
	_ = taskDir

	// Commit and push
	fmt.Printf("Pushing task %s (%d image(s))...\n", taskID, len(shotPaths))
	rel := filepath.Join(".xmuggle", "queue", taskID)
	if err := gitops.CommitAndPush(repoRoot, []string{rel}, fmt.Sprintf("xmuggle: task %s", taskID)); err != nil {
		die("push task: %v", err)
	}

	// Mark screenshots as processed (delete from Desktop)
	for _, sp := range shotPaths {
		_ = images.MarkProcessed(sp)
	}

	fmt.Printf("Task %s sent. The daemon will process it.\n", taskID)
}

func selectPeer(peersDir, selfHostname string) string {
	entries, err := os.ReadDir(peersDir)
	if err != nil {
		die("read peers: %v (run 'xmuggle init' first)", err)
	}

	var peers []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.Name() == selfHostname {
			continue
		}
		peers = append(peers, e.Name())
	}

	if len(peers) == 0 {
		die("No peers found. Run 'xmuggle init' from another machine that has this repo cloned.")
	}
	if len(peers) == 1 {
		return peers[0]
	}

	fmt.Println("\nSelect a peer to send to:")
	for i, p := range peers {
		fmt.Printf("  [%d] %s\n", i+1, p)
	}
	fmt.Print("Choice: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(peers) {
		die("invalid selection")
	}
	return peers[n-1]
}

func remoteToSlug(remote string) string {
	s := strings.TrimPrefix(remote, "https://github.com/")
	s = strings.TrimPrefix(s, "git@github.com:")
	s = strings.TrimSuffix(s, ".git")
	return s
}

// ────────────────────────────────────────────────────────────────────
// list
// ────────────────────────────────────────────────────────────────────

func cmdList(args []string) {
	useJSON := false
	for _, a := range args {
		if a == "--json" {
			useJSON = true
		}
	}

	imgs, err := images.ListAll()
	if err != nil {
		die("list: %v", err)
	}

	if useJSON {
		type jsonImage struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			Status      string `json:"status"`
			ModTime     string `json:"mod_time"`
			ModTimeUnix int64  `json:"mod_time_unix"`
		}
		var out []jsonImage
		for _, img := range imgs {
			status := "pending"
			if img.IsProcessed {
				status = "done"
			}
			out = append(out, jsonImage{
				Name:        img.Name,
				Path:        img.Path,
				Status:      status,
				ModTime:     img.ModTime.Format(time.RFC3339),
				ModTimeUnix: img.ModTime.Unix(),
			})
		}
		if out == nil {
			out = []jsonImage{}
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return
	}

	if len(imgs) == 0 {
		fmt.Println("No screenshots found on ~/Desktop.")
		return
	}
	unprocessed := 0
	for _, img := range imgs {
		if !img.IsProcessed {
			unprocessed++
		}
	}
	fmt.Printf("%d image(s) tracked (%d unprocessed):\n\n", len(imgs), unprocessed)
	for _, img := range imgs {
		status := "pending"
		if img.IsProcessed {
			status = "done"
		}
		fmt.Printf("  [%s] %s\n", status, img.Name)
	}
}

// ────────────────────────────────────────────────────────────────────
// peers
// ────────────────────────────────────────────────────────────────────

func cmdPeers() {
	repoRoot := mustRepoRoot()
	hostname := mustHostname()
	p := config.GetRepoPaths(repoRoot)

	_ = gitops.PullRebase(repoRoot)

	entries, err := os.ReadDir(p.PeersDir)
	if err != nil {
		die("read peers: %v (run 'xmuggle init' first)", err)
	}

	fmt.Println("Peers:")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		marker := ""
		if e.Name() == hostname {
			marker = "  ← this machine"
		}
		fmt.Printf("  %s%s\n", e.Name(), marker)
	}
}

// ────────────────────────────────────────────────────────────────────
// screenshots picker
// ────────────────────────────────────────────────────────────────────

func promptSelectScreenshots() []string {
	imgs, err := images.ListAll()
	if err != nil {
		die("list: %v", err)
	}
	if len(imgs) == 0 {
		die("No screenshots found on ~/Desktop.")
	}

	fmt.Printf("Select screenshot(s) to send (newest first):\n\n")
	for i, img := range imgs {
		status := "pending"
		if img.IsProcessed {
			status = "done"
		}
		age := time.Since(img.ModTime).Truncate(time.Second)
		fmt.Printf("  [%d] %-7s %s  (%s ago)\n", i+1, status, img.Name, age)
	}
	fmt.Println()
	fmt.Print("Enter numbers (e.g. 1 3 5 or 1,3,5 or 1-3): ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		die("read input: %v", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		die("No selection made.")
	}

	selected := parseNumberSelection(line, len(imgs))
	if len(selected) == 0 {
		die("No valid selection.")
	}

	var paths []string
	for _, idx := range selected {
		paths = append(paths, imgs[idx].Path)
	}
	return paths
}

func parseNumberSelection(input string, max int) []int {
	input = strings.ReplaceAll(input, ",", " ")
	parts := strings.Fields(input)
	seen := map[int]bool{}
	var result []int
	for _, part := range parts {
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil {
				continue
			}
			for n := lo; n <= hi; n++ {
				idx := n - 1
				if idx >= 0 && idx < max && !seen[idx] {
					seen[idx] = true
					result = append(result, idx)
				}
			}
		} else {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				continue
			}
			idx := n - 1
			if idx >= 0 && idx < max && !seen[idx] {
				seen[idx] = true
				result = append(result, idx)
			}
		}
	}
	return result
}

// ────────────────────────────────────────────────────────────────────
// rec
// ────────────────────────────────────────────────────────────────────

func cmdRec(args []string) {
	var (
		durStr  string
		fpsVal  float64 = 1.0
		format  string  = "jpg"
		message string
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--duration":
			i++
			if i < len(args) {
				durStr = args[i]
			}
		case "--fps":
			i++
			if i < len(args) {
				v, err := strconv.ParseFloat(args[i], 64)
				if err == nil {
					fpsVal = v
				}
			}
		case "--format":
			i++
			if i < len(args) {
				format = args[i]
			}
		case "--msg":
			i++
			if i < len(args) {
				message = args[i]
			}
		}
	}

	var dur time.Duration
	if durStr != "" {
		d, err := time.ParseDuration(durStr)
		if err != nil {
			die("invalid duration %q: %v", durStr, err)
		}
		dur = d
	}

	tmpDir := filepath.Join(os.TempDir(), "xmuggle-rec")
	_ = os.MkdirAll(tmpDir, 0o755)

	rec := record.New(record.Options{
		Duration:  dur,
		FPS:       fpsVal,
		Format:    format,
		OutputDir: tmpDir,
	})

	if dur > 0 {
		fmt.Fprintf(os.Stderr, "Recording for %s (%.0f fps, %s)...\n", dur, fpsVal, format)
	} else {
		fmt.Fprintf(os.Stderr, "Recording (%.0f fps, %s)... Press Ctrl+C to stop.\n", fpsVal, format)
	}

	if err := rec.Start(); err != nil {
		die("%v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	if dur > 0 {
		select {
		case <-sigCh:
		case <-time.After(dur + 500*time.Millisecond):
		}
	} else {
		<-sigCh
	}
	signal.Stop(sigCh)

	frames := rec.Stop()
	fmt.Fprintf(os.Stderr, "\nCaptured %d frame(s)\n", len(frames))

	if len(frames) == 0 {
		fmt.Println("No frames captured.")
		return
	}

	// Auto-submit
	repoRoot := mustRepoRoot()
	p := config.GetRepoPaths(repoRoot)
	hostname := mustHostname()

	target := selectPeer(p.PeersDir, hostname)
	remoteURL := gitops.RemoteURL(repoRoot)
	repoSlug := repoRoot
	if remoteURL != "" {
		repoSlug = remoteToSlug(remoteURL)
	}

	taskID := queue.NewTaskID()
	t := queue.Task{
		Repo:      repoSlug,
		Message:   message,
		Timestamp: time.Now().UnixMilli(),
		Status:    queue.StatusPending,
		Target:    target,
		Sender:    hostname,
	}
	_, err := queue.WriteTaskMulti(p.QueueDir, taskID, t, frames)
	if err != nil {
		die("write task: %v", err)
	}

	rel := filepath.Join(".xmuggle", "queue", taskID)
	if err := gitops.CommitAndPush(repoRoot, []string{rel}, fmt.Sprintf("xmuggle: rec %s", taskID)); err != nil {
		die("push: %v", err)
	}
	fmt.Printf("Submitted %d frame(s) as task %s\n", len(frames), taskID)
}

// ────────────────────────────────────────────────────────────────────
// rm
// ────────────────────────────────────────────────────────────────────

func cmdRm(args []string) {
	if len(args) == 0 {
		die("Usage: xmuggle rm <name>... [--all-done]")
	}

	var names []string
	allDone := false
	for _, a := range args {
		if a == "--all-done" {
			allDone = true
			continue
		}
		names = append(names, a)
	}

	var failed bool
	for _, q := range names {
		name, err := images.RemoveByName(q)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			failed = true
			continue
		}
		fmt.Printf("Removed: %s\n", name)
	}

	if allDone {
		removed, err := images.RemoveAllDone()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			failed = true
		}
		if len(removed) == 0 {
			fmt.Println("No processed images to remove.")
		} else {
			fmt.Printf("Removed %d processed image(s):\n", len(removed))
			for _, n := range removed {
				fmt.Println("  " + n)
			}
		}
	}

	if failed {
		os.Exit(1)
	}
}

// interactive returns true if stdin is connected to a terminal.
func interactive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// runLocal runs the claude agent locally (used by rec auto-submit as fallback).
func runLocal(shotPaths []string, repo, message string) {
	p := prompt.Build(prompt.Options{
		ScreenshotPaths: shotPaths,
		Repo:            repo,
		Message:         message,
	})
	code, err := spawn.Interactive(p, "")
	if err != nil {
		die("%v", err)
	}
	for _, sp := range shotPaths {
		_ = images.MarkProcessed(sp)
	}
	os.Exit(code)
}
