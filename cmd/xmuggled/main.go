// Command xmuggled is the xmuggle daemon: watches registered repos' .xmuggle/queue/
// directories, enqueues tasks to agent-queue, and spawns workers.
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jschell12/xmuggle/internal/aq"
	"github.com/jschell12/xmuggle/internal/config"
	"github.com/jschell12/xmuggle/internal/gitops"
	"github.com/jschell12/xmuggle/internal/prompt"
	"github.com/jschell12/xmuggle/internal/queue"
	"github.com/jschell12/xmuggle/internal/spawn"
)

var (
	prURLRE  = regexp.MustCompile(`https://github\.com/[^\s]+/pull/\d+`)
	branchRE = regexp.MustCompile(`xmuggle-fix/\d+`)
)

func projectName(repo string) string {
	s := strings.TrimPrefix(repo, "https://github.com/")
	s = strings.TrimPrefix(s, "git@github.com:")
	s = strings.TrimSuffix(s, ".git")
	return filepath.Base(s)
}

func repoURL(repo string) string {
	if strings.HasPrefix(repo, "http") || strings.HasPrefix(repo, "git@") {
		return repo
	}
	if m, _ := regexp.MatchString(`^[\w-]+/[\w.-]+$`, repo); m {
		return "https://github.com/" + repo + ".git"
	}
	return repo
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// repoWatcher watches a single repo's .xmuggle/queue/ for tasks.
type repoWatcher struct {
	repoRoot   string
	hostname   string
	maxWorkers int
	interval   time.Duration
	mu         sync.Mutex
}

func (w *repoWatcher) logf(format string, a ...any) {
	prefix := filepath.Base(w.repoRoot)
	log.Printf("[%s] "+format, append([]any{prefix}, a...)...)
}

func (w *repoWatcher) paths() config.RepoPaths {
	return config.GetRepoPaths(w.repoRoot)
}

// tick pulls the repo, finds tasks addressed to us, enqueues and spawns workers.
func (w *repoWatcher) tick() {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Pull latest
	if err := gitops.PullRebase(w.repoRoot); err != nil {
		w.logf("pull: %v", err)
	}

	p := w.paths()
	pending := queue.ListPending(p.QueueDir)
	if len(pending) == 0 {
		return
	}

	groups := map[string][2]string{} // project → [project, repo]
	enqueuedByProject := map[string]int{}

	for _, taskDir := range pending {
		info, ok := w.enqueueTask(taskDir)
		if !ok {
			continue
		}
		project := info[0]
		groups[project] = info
		enqueuedByProject[project]++
	}

	for project, info := range groups {
		repo := info[1]
		active, err := aq.ActiveWorkerCount(project)
		if err != nil {
			w.logf("active count for %s: %v", project, err)
			active = 0
		}
		needed := enqueuedByProject[project]
		if needed > w.maxWorkers {
			needed = w.maxWorkers
		}
		toSpawn := needed - active
		for i := 0; i < toSpawn; i++ {
			w.spawnWorker(project, repo)
		}
		w.logf("project %s: %d enqueued, %d workers active, %d spawned", project, enqueuedByProject[project], active, max(0, toSpawn))
	}
}

func (w *repoWatcher) enqueueTask(taskDir string) (projectAndRepo [2]string, ok bool) {
	taskID := filepath.Base(taskDir)
	t, err := queue.ReadTask(taskDir)
	if err != nil {
		w.logf("read %s: %v", taskDir, err)
		_ = queue.UpdateTaskStatus(taskDir, queue.StatusError)
		return
	}

	// Only process tasks addressed to us
	if t.Target != "" && t.Target != w.hostname {
		return
	}

	shots := queue.FindScreenshots(taskDir)
	if len(shots) == 0 {
		w.logf("no screenshots in %s", taskDir)
		_ = queue.UpdateTaskStatus(taskDir, queue.StatusError)
		return
	}

	project := projectName(t.Repo)
	if err := aq.Init(project); err != nil {
		w.logf("aq init %s: %v", project, err)
		return
	}
	title := "xmuggle-fix:" + taskID
	desc := t.Message
	if desc == "" {
		desc = "Screenshot-driven fix"
	}
	if err := aq.Add(project, title, desc, []string{"screenshot"}); err != nil {
		w.logf("aq add %s: %v", taskID, err)
		return
	}
	w.logf("enqueued task %s to agent-queue project %s", taskID, project)
	_ = queue.UpdateTaskStatus(taskDir, queue.StatusProcessing)
	return [2]string{project, t.Repo}, true
}

func (w *repoWatcher) spawnWorker(project, repo string) {
	agentID, err := aq.NextAgentID(project)
	if err != nil {
		w.logf("next agent id: %v", err)
		return
	}

	info, err := aq.Clone(repoURL(repo), agentID, os.TempDir())
	if err != nil {
		w.logf("aq clone for %s: %v", agentID, err)
		return
	}

	p := w.paths()
	w.logf("spawning worker %s in %s on branch %s", agentID, info.CloneDir, info.Branch)

	wp := prompt.BuildWorker(prompt.WorkerOptions{
		AgentID:            agentID,
		Project:            project,
		RepoURL:            repoURL(repo),
		CloneDir:           info.CloneDir,
		Branch:             info.Branch,
		AQScripts:          aq.ScriptsDir(),
		ScreenshotQueueDir: p.QueueDir,
		ResultsDir:         p.ResultsDir,
	})

	repoRoot := w.repoRoot

	go func() {
		result, err := spawn.Capture(wp, info.CloneDir)
		if err != nil {
			w.logf("worker %s error: %v", agentID, err)
			return
		}
		if result.ExitCode != 0 {
			w.logf("worker %s exited with code %d", agentID, result.ExitCode)
			return
		}
		combined := result.Stdout + "\n" + result.Stderr
		prURL := prURLRE.FindString(combined)
		branch := branchRE.FindString(combined)
		w.logf("worker %s completed. PR: %s, branch: %s", agentID, prURL, branch)

		// Publish result and clean up task via git
		w.publishResult(repoRoot)
	}()
}

// publishResult commits any new results and cleans up processed tasks.
func (w *repoWatcher) publishResult(repoRoot string) {
	p := config.GetRepoPaths(repoRoot)

	// Check for result dirs
	resultEntries, err := os.ReadDir(p.ResultsDir)
	if err != nil {
		return
	}

	var paths []string
	for _, e := range resultEntries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		resultFile := filepath.Join(p.ResultsDir, e.Name(), "result.json")
		if _, err := os.Stat(resultFile); err != nil {
			continue
		}
		// Add result
		paths = append(paths, filepath.Join(".xmuggle", "results", e.Name()))

		// Remove task from queue
		taskDir := filepath.Join(p.QueueDir, e.Name())
		if _, err := os.Stat(taskDir); err == nil {
			_ = gitops.GitRm(repoRoot, filepath.Join(".xmuggle", "queue", e.Name()))
			paths = append(paths, filepath.Join(".xmuggle", "queue", e.Name()))
		}
	}

	if len(paths) == 0 {
		return
	}

	if err := gitops.CommitAndPush(repoRoot, paths, "xmuggle: publish results"); err != nil {
		w.logf("publish results: %v", err)
	} else {
		w.logf("published results")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	if err := config.EnsureDaemonDirs(); err != nil {
		fmt.Fprintln(os.Stderr, "ensure dirs:", err)
		os.Exit(1)
	}

	daemonCfg, err := config.LoadDaemonConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load daemon config:", err)
		os.Exit(1)
	}

	if len(daemonCfg.Repos) == 0 {
		fmt.Fprintln(os.Stderr, "No repos registered. Run 'xmuggle init' in a git repo first.")
		os.Exit(1)
	}

	hostname, _ := os.Hostname()
	hostname = config.NormalizeHostname(hostname)
	maxWorkers := envInt("MAX_WORKERS", 3)
	interval := time.Duration(envInt("POLL_INTERVAL_MS", 10000)) * time.Millisecond

	log.SetFlags(log.LstdFlags | log.LUTC)
	log.Printf("Daemon started. Hostname: %s, max workers: %d, poll: %s", hostname, maxWorkers, interval)
	log.Printf("Agent-queue scripts: %s", aq.ScriptsDir())
	log.Printf("Watching %d repo(s):", len(daemonCfg.Repos))
	for _, r := range daemonCfg.Repos {
		log.Printf("  %s", r)
	}

	// Create watchers
	var watchers []*repoWatcher
	for _, repoRoot := range daemonCfg.Repos {
		if _, err := os.Stat(filepath.Join(repoRoot, ".xmuggle")); os.IsNotExist(err) {
			log.Printf("skipping %s: no .xmuggle/ directory", repoRoot)
			continue
		}
		watchers = append(watchers, &repoWatcher{
			repoRoot:   repoRoot,
			hostname:   hostname,
			maxWorkers: maxWorkers,
			interval:   interval,
		})
	}

	if len(watchers) == 0 {
		fmt.Fprintln(os.Stderr, "No valid repos to watch.")
		os.Exit(1)
	}

	// Initial tick
	for _, w := range watchers {
		w.tick()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			for _, w := range watchers {
				w.tick()
			}
		case s := <-sig:
			log.Printf("Received %s, shutting down", s)
			return
		}
	}
}
