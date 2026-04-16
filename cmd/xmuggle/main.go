// Command xmuggle is the primary CLI — runs a Claude Code agent against a
// screenshot to fix code in a target repo.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jschell12/xmuggle/internal/ageutil"
	"github.com/jschell12/xmuggle/internal/config"
	"github.com/jschell12/xmuggle/internal/discover"
	"github.com/jschell12/xmuggle/internal/gitqueue"
	"github.com/jschell12/xmuggle/internal/images"
	"github.com/jschell12/xmuggle/internal/prompt"
	"github.com/jschell12/xmuggle/internal/queue"
	"github.com/jschell12/xmuggle/internal/remote"
	"github.com/jschell12/xmuggle/internal/spawn"
)

const usage = `Usage: xmuggle [<subcommand>] [flags]

Main flags:
  --repo  <repo>   GitHub repo (owner/name or URL) or local path
  --img   <name>   Select image by fuzzy match (repeatable)
  --all            Process ALL unprocessed images
  --msg   <msg>    Optional context
  --list           Show images in ~/.xmuggle/ and status
  --scan           Ingest ALL images from ~/Desktop (not just screenshots)

Transports:
  (default)        process locally
  --remote         SSH/rsync to a Mac on the LAN
    --host <host>  specific hostname (otherwise dns-sd discovery)
    --user <user>  SSH user (default: $USER)
  --remote --git   age-encrypted via private GitHub queue repo
    --to <host>    recipient hostname (overrides default_recipient)

Subcommands (for --git setup):
  xmuggle queue-init <owner/repo>
  xmuggle init-keys
  xmuggle add-recipient <host> [--pubkey age1...] [--default]
  xmuggle list-recipients

Examples:
  xmuggle --repo jschell12/my-app                              # latest screenshot locally
  xmuggle --repo jschell12/my-app --all --msg "fix alignment"  # all pending
  xmuggle --repo jschell12/my-app --remote --git               # encrypted via git
  xmuggle --list
`

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		// No args means: use default behavior (like --list-ish?). Keep it explicit.
		fmt.Print(usage)
		os.Exit(0)
	}

	first := os.Args[1]
	switch first {
	case "-h", "--help":
		fmt.Print(usage)
		return
	case "queue-init":
		cmdQueueInit(os.Args[2:])
		return
	case "init-keys":
		cmdInitKeys()
		return
	case "add-recipient":
		cmdAddRecipient(os.Args[2:])
		return
	case "list-recipients":
		cmdListRecipients()
		return
	}

	runMain(os.Args[1:])
}

type mainArgs struct {
	repo, message, host, user, to string
	remote, useGit, list, scan, all bool
	imgs []string
}

func parseMainArgs(args []string) *mainArgs {
	a := &mainArgs{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			i++
			if i < len(args) {
				a.repo = args[i]
			}
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
		case "--host":
			i++
			if i < len(args) {
				a.host = args[i]
			}
		case "--user":
			i++
			if i < len(args) {
				a.user = args[i]
			}
		case "--to":
			i++
			if i < len(args) {
				a.to = args[i]
			}
		case "--remote":
			a.remote = true
		case "--git":
			a.useGit = true
		case "--list":
			a.list = true
		case "--scan":
			a.scan = true
		case "--all":
			a.all = true
		}
	}
	return a
}

func runMain(rawArgs []string) {
	a := parseMainArgs(rawArgs)

	if a.scan {
		n, err := images.IngestAll()
		if err != nil {
			die("scan: %v", err)
		}
		fmt.Printf("Ingested %d image(s) into ~/.xmuggle/\n", n)
		if a.repo == "" {
			return
		}
	}

	if a.list {
		n, _ := images.AutoIngest()
		if n > 0 {
			fmt.Printf("Auto-ingested %d new screenshot(s)\n\n", n)
		}
		imgs, err := images.ListAll()
		if err != nil {
			die("list: %v", err)
		}
		if len(imgs) == 0 {
			fmt.Println("No images in ~/.xmuggle/")
			fmt.Println("Take a screenshot, or run --scan to ingest all images from ~/Desktop.")
			return
		}
		unprocessed := 0
		for _, img := range imgs {
			if !img.IsProcessed {
				unprocessed++
			}
		}
		fmt.Printf("%d image(s) in ~/.xmuggle/ (%d unprocessed):\n\n", len(imgs), unprocessed)
		for _, img := range imgs {
			status := "pending"
			if img.IsProcessed {
				status = "done"
			}
			fmt.Printf("  [%s] %s\n", status, img.Name)
		}
		return
	}

	if a.repo == "" {
		fmt.Fprintln(os.Stderr, "Error: --repo is required")
		fmt.Println(usage)
		os.Exit(1)
	}

	var shotPaths []string
	switch {
	case len(a.imgs) > 0:
		for _, q := range a.imgs {
			img, err := images.FindByName(q)
			if err != nil || img == nil {
				die("no image matching %q in ~/.xmuggle/ (run --list)", q)
			}
			shotPaths = append(shotPaths, img.Path)
		}
	case a.all:
		ups, err := images.AllUnprocessed()
		if err != nil {
			die("find unprocessed: %v", err)
		}
		if len(ups) == 0 {
			die("No unprocessed images. Take a screenshot or run --scan.")
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
			die("No unprocessed images. Take a screenshot or run --scan.")
		}
		shotPaths = []string{img.Path}
	}

	var names []string
	for _, p := range shotPaths {
		names = append(names, filepath.Base(p))
	}
	fmt.Printf("Screenshot(s): %s\n", strings.Join(names, ", "))
	fmt.Printf("Target repo: %s\n", a.repo)
	if a.message != "" {
		fmt.Printf("Context: %s\n", a.message)
	}
	mode := "local"
	if a.useGit {
		mode = "remote (git)"
	} else if a.remote {
		mode = "remote (ssh)"
	}
	fmt.Printf("Mode: %s\n---\n", mode)

	switch {
	case a.useGit:
		runRemoteGit(shotPaths, a)
	case a.remote:
		runRemoteSSH(shotPaths, a)
	default:
		runLocal(shotPaths, a)
	}
}

func runLocal(shotPaths []string, a *mainArgs) {
	p := prompt.Build(prompt.Options{
		ScreenshotPaths: shotPaths,
		Repo:            a.repo,
		Message:         a.message,
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

func runRemoteSSH(shotPaths []string, a *mainArgs) {
	host := a.host
	if host == "" {
		fmt.Println("Discovering Macs on the LAN...")
		svcs, err := discover.DiscoverAll(4 * time.Second)
		if err != nil || len(svcs) == 0 {
			die("no Macs discovered. Pass --host <hostname>")
		}
		fmt.Println("Discovered SSH hosts:")
		for i, s := range svcs {
			fmt.Printf("  [%d] %s -> %s\n", i+1, s.Instance, s.Host)
		}
		fmt.Print("Choose target: ")
		var choice int
		_, _ = fmt.Scanln(&choice)
		if choice < 1 || choice > len(svcs) {
			die("invalid choice")
		}
		host = svcs[choice-1].Host
	}

	target := remote.Target{Host: host, User: a.user}
	fmt.Printf("Remote: %s\n", host)

	var taskIDs []string
	for _, sp := range shotPaths {
		taskID := queue.NewTaskID()
		tmpBase := filepath.Join(os.TempDir(), "xmuggle-tasks")
		_ = os.MkdirAll(tmpBase, 0o755)

		t := queue.Task{
			Repo:      a.repo,
			Message:   a.message,
			Timestamp: time.Now().UnixMilli(),
			Status:    queue.StatusPending,
		}
		taskDir, err := queue.WriteTask(tmpBase, taskID, t, sp)
		if err != nil {
			die("write task: %v", err)
		}
		fmt.Printf("Sending task %s...\n", taskID)
		if err := remote.SendTask(target, taskDir, taskID); err != nil {
			die("send: %v", err)
		}
		taskIDs = append(taskIDs, taskID)
		_ = images.MarkProcessed(sp)
	}

	fmt.Printf("%d task(s) sent. Waiting for results...\n", len(taskIDs))
	pollForResults(taskIDs, func(id string) (*queue.Result, error) {
		return remote.PollForResult(target, id, 10*time.Minute, 5*time.Second)
	}, a.repo)
}

func runRemoteGit(shotPaths []string, a *mainArgs) {
	cfg, err := config.Load()
	if err != nil {
		die("load config: %v", err)
	}
	if cfg.Git == nil {
		die("git transport not configured. Run: xmuggle queue-init <owner/repo>")
	}
	if cfg.Age == nil {
		die("no age keypair. Run: xmuggle init-keys")
	}

	recipient := a.to
	if recipient == "" {
		recipient = cfg.DefaultRecipient
	}
	fmt.Printf("Queue repo: %s\n", cfg.Git.QueueRepo)
	fmt.Printf("Recipient: %s\n", recipient)

	var taskIDs []string
	for _, sp := range shotPaths {
		taskID := queue.NewTaskID()
		fmt.Printf("Encrypting and pushing task %s...\n", taskID)
		err := gitqueue.SendTask(cfg, gitqueue.SendArgs{
			TaskID:         taskID,
			Repo:           a.repo,
			Message:        a.message,
			ScreenshotPath: sp,
			Recipient:      a.to,
		})
		if err != nil {
			die("send (git): %v", err)
		}
		taskIDs = append(taskIDs, taskID)
		_ = images.MarkProcessed(sp)
	}

	fmt.Printf("%d task(s) queued. Waiting for results...\n", len(taskIDs))
	pollForResults(taskIDs, func(id string) (*queue.Result, error) {
		r, err := gitqueue.PollForResult(cfg, id, 10*time.Minute)
		if err != nil {
			return nil, err
		}
		return &queue.Result{
			Status:    r.Status,
			PRUrl:     r.PRUrl,
			Branch:    r.Branch,
			Summary:   r.Summary,
			Timestamp: r.Timestamp,
		}, nil
	}, a.repo)
}

// pollForResults drains results serially and prints summaries, then optionally git-pulls in a local repo.
func pollForResults(taskIDs []string, poll func(string) (*queue.Result, error), repo string) {
	failed := false
	for _, id := range taskIDs {
		r, err := poll(id)
		fmt.Printf("\n--- Task %s ---\n", id)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}
		if r.Status == "success" {
			fmt.Println("Fix applied successfully!")
			if r.PRUrl != "" {
				fmt.Println("PR:", r.PRUrl)
			}
			if r.Branch != "" {
				fmt.Println("Branch:", r.Branch)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Agent reported an error:")
			fmt.Fprintln(os.Stderr, r.Summary)
			failed = true
		}
	}

	if _, err := os.Stat(repo); err == nil {
		fmt.Printf("\nPulling latest in %s...\n", repo)
		cmd := exec.Command("git", "pull")
		cmd.Dir = repo
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
	if failed {
		os.Exit(1)
	}
}

//
// Subcommands
//

func cmdQueueInit(args []string) {
	if len(args) < 1 {
		die("Usage: xmuggle queue-init <owner/repo>")
	}
	slug := args[0]
	if !strings.Contains(slug, "/") {
		die("Invalid slug: expected owner/repo")
	}
	cfg, err := config.Load()
	if err != nil {
		die("load config: %v", err)
	}
	cfg.SetGit(slug)
	if err := config.Save(cfg); err != nil {
		die("save config: %v", err)
	}

	fmt.Printf("Cloning %s into %s...\n", slug, cfg.Git.CloneDir)
	if err := gitqueue.EnsureCloned(slug, cfg.Git.CloneDir, cfg.Git.Branch); err != nil {
		die("clone: %v", err)
	}

	// Scaffold directories
	var touched []string
	for _, d := range []string{"queue", "results", "pubkeys", "processed"} {
		rel := d + "/.gitkeep"
		if !gitqueue.FileExists(cfg.Git.CloneDir, rel) {
			_ = gitqueue.WriteFile(cfg.Git.CloneDir, rel, []byte(""))
			touched = append(touched, rel)
		}
	}
	if len(touched) > 0 {
		if err := gitqueue.CommitAndPush(cfg.Git.CloneDir, touched, "Scaffold queue repo directories", cfg.Git.Branch, cfg.Git.AuthorName, cfg.Git.AuthorEmail); err != nil {
			die("commit: %v", err)
		}
	}

	fmt.Printf("Queue repo ready: %s\n", slug)
	if cfg.Age != nil {
		publishOwnPubkey(cfg)
	} else {
		fmt.Println("\nNext: xmuggle init-keys")
	}
}

func cmdInitKeys() {
	if err := config.EnsureDirs(); err != nil {
		die("ensure dirs: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		die("load config: %v", err)
	}

	identityPath := config.DefaultIdentityPath()
	if cfg.Age != nil && cfg.Age.IdentityFile != "" {
		identityPath = cfg.Age.IdentityFile
	}

	if _, err := os.Stat(identityPath); err == nil {
		pub, err := ageutil.ReadIdentityPubkey(identityPath)
		if err != nil {
			die("read existing identity: %v", err)
		}
		fmt.Printf("Identity already exists at %s\n", identityPath)
		fmt.Printf("Public key: %s\n", pub)
		cfg.SetAge(identityPath, pub)
		_ = config.Save(cfg)
		publishOwnPubkey(cfg)
		return
	}

	fmt.Printf("Generating age keypair at %s...\n", identityPath)
	pub, err := ageutil.GenerateKeypair(identityPath)
	if err != nil {
		die("generate keypair: %v", err)
	}
	cfg.SetAge(identityPath, pub)
	if err := config.Save(cfg); err != nil {
		die("save config: %v", err)
	}

	fmt.Printf("\nPublic key: %s\n", pub)
	fmt.Println("\nShare this pubkey with other machines so they can encrypt to you:")
	fmt.Printf("  xmuggle add-recipient %s --pubkey %s\n", cfg.Hostname, pub)

	publishOwnPubkey(cfg)
}

func publishOwnPubkey(cfg *config.Config) {
	if cfg.Git == nil || cfg.Age == nil {
		return
	}
	if err := gitqueue.EnsureCloned(cfg.Git.QueueRepo, cfg.Git.CloneDir, cfg.Git.Branch); err != nil {
		fmt.Fprintf(os.Stderr, "note: could not reach queue repo: %v\n", err)
		return
	}
	_ = gitqueue.PullRebase(cfg.Git.CloneDir, cfg.Git.Branch)

	rel := fmt.Sprintf("pubkeys/%s.pub", cfg.Hostname)
	existing := ""
	if gitqueue.FileExists(cfg.Git.CloneDir, rel) {
		data, err := gitqueue.ReadFile(cfg.Git.CloneDir, rel)
		if err == nil {
			existing = strings.TrimSpace(string(data))
		}
	}
	if existing == cfg.Age.Pubkey {
		fmt.Printf("Pubkey already published at %s\n", rel)
		return
	}
	if err := gitqueue.WriteFile(cfg.Git.CloneDir, rel, []byte(cfg.Age.Pubkey+"\n")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := gitqueue.CommitAndPush(cfg.Git.CloneDir, []string{rel}, "Publish pubkey for "+cfg.Hostname, cfg.Git.Branch, cfg.Git.AuthorName, cfg.Git.AuthorEmail); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("Published %s to %s\n", rel, cfg.Git.QueueRepo)
}

func cmdAddRecipient(args []string) {
	if len(args) < 1 {
		die("Usage: xmuggle add-recipient <hostname> [--pubkey age1...] [--default]")
	}
	hostname := args[0]
	var pubkey string
	asDefault := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--pubkey":
			i++
			if i < len(args) {
				pubkey = args[i]
			}
		case "--default":
			asDefault = true
		}
	}
	cfg, err := config.Load()
	if err != nil {
		die("load config: %v", err)
	}

	if pubkey == "" {
		if cfg.Git == nil {
			die("no pubkey given and no queue repo. Pass --pubkey or run queue-init first.")
		}
		if err := gitqueue.EnsureCloned(cfg.Git.QueueRepo, cfg.Git.CloneDir, cfg.Git.Branch); err != nil {
			die("reach queue repo: %v", err)
		}
		_ = gitqueue.PullRebase(cfg.Git.CloneDir, cfg.Git.Branch)
		rel := fmt.Sprintf("pubkeys/%s.pub", hostname)
		if !gitqueue.FileExists(cfg.Git.CloneDir, rel) {
			die("no pubkey at %s in %s. Ask that machine's owner to run 'xmuggle init-keys'.", rel, cfg.Git.QueueRepo)
		}
		data, err := gitqueue.ReadFile(cfg.Git.CloneDir, rel)
		if err != nil {
			die("read pubkey: %v", err)
		}
		pubkey = strings.TrimSpace(string(data))
	}

	if !strings.HasPrefix(pubkey, "age1") {
		die("%q doesn't look like an age pubkey", pubkey)
	}

	cfg.UpsertRecipient(config.Recipient{Hostname: hostname, Pubkey: pubkey})
	if asDefault || cfg.DefaultRecipient == "" {
		cfg.DefaultRecipient = hostname
		fmt.Printf("Default recipient: %s\n", hostname)
	}
	if err := config.Save(cfg); err != nil {
		die("save: %v", err)
	}
	fmt.Printf("Added recipient %s\n", hostname)
}

func cmdListRecipients() {
	cfg, err := config.Load()
	if err != nil {
		die("load config: %v", err)
	}
	fmt.Println("Hostname:", cfg.Hostname)
	if cfg.Age != nil {
		fmt.Println("Self pubkey:", cfg.Age.Pubkey)
	}
	if cfg.Git != nil {
		fmt.Println("Queue repo:", cfg.Git.QueueRepo)
	}
	fmt.Println()

	if len(cfg.Recipients) == 0 {
		fmt.Println("No recipients configured.")
		return
	}
	fmt.Println("Recipients:")
	for _, r := range cfg.Recipients {
		marker := ""
		if r.Hostname == cfg.DefaultRecipient {
			marker = " (default)"
		}
		fmt.Printf("  %s%s\n    %s\n", r.Hostname, marker, r.Pubkey)
	}
	_ = json.Marshal // silence unused if imports get cleaned
}
