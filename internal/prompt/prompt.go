// Package prompt builds the text prompts fed to the Claude Code agent.
package prompt

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Options struct {
	ScreenshotPaths []string
	Repo            string
	Message         string
}

var shortSlugRE = regexp.MustCompile(`^[\w-]+/[\w.-]+$`)

// Build returns the prompt for direct local execution (single-agent mode).
func Build(opts Options) string {
	isURL := strings.HasPrefix(opts.Repo, "http") ||
		strings.HasPrefix(opts.Repo, "git@") ||
		shortSlugRE.MatchString(opts.Repo)

	ts := time.Now().UnixMilli()
	branch := fmt.Sprintf("xmuggle-fix/%d", ts)

	var repoSetup string
	if isURL {
		slug := strings.TrimPrefix(opts.Repo, "https://github.com/")
		repoSetup = fmt.Sprintf(
			"Clone the repo: git clone https://github.com/%s /tmp/xmuggle-%d\ncd /tmp/xmuggle-%d",
			slug, ts, ts,
		)
	} else {
		repoSetup = "cd " + opts.Repo
	}

	userContext := ""
	if opts.Message != "" {
		userContext = fmt.Sprintf("\n\nAdditional context from the user: %q", opts.Message)
	}

	var shots string
	if len(opts.ScreenshotPaths) == 1 {
		shots = fmt.Sprintf(
			"Read the file at %s using the Read tool. This is a screenshot showing a bug, UI issue, error, or desired change.",
			opts.ScreenshotPaths[0],
		)
	} else {
		var sb strings.Builder
		sb.WriteString("Read each of the following screenshots using the Read tool. Together they show a bug, UI issue, error, or desired change:\n\n")
		for i, p := range opts.ScreenshotPaths {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, p)
		}
		shots = sb.String()
	}

	return fmt.Sprintf(`You are a screenshot-driven code fix agent. Your job is to analyze screenshot(s), understand the problem, and fix it in a target repo.

## Step 1: Analyze the screenshot(s)

%s%s

Describe what you see and identify exactly what needs to change.

## Step 2: Set up the repo

%s

Create a new branch:
git checkout -b %s

## Step 3: Find and fix the issue

Based on your analysis of the screenshot, explore the codebase to find the relevant source files. Make the necessary code changes to fix the identified issue. Be surgical — only change what is needed.

## Step 4: Commit and push

Stage the changed files (by name, not git add -A), write a clear commit message describing what the screenshot showed and what you fixed, and push the branch:
git push -u origin %s

## Step 5: Create and merge PR

Create a pull request:
gh pr create --title "<concise description of the fix>" --body "## Screenshot analysis
<what you saw in the screenshot>

## Changes made
<what you changed and why>

---
Automated fix by xmuggle"

Then merge it:
gh pr merge --squash --auto

If the merge fails (e.g. merge conflicts, required checks), report the PR URL so the user can handle it manually.

## Important

- Do NOT hallucinate file contents — always read files before editing them.
- If the screenshot is unclear or you cannot determine what to fix, explain what you see and stop.
- If the repo requires a build step, run it after your changes to verify they compile.
`, shots, userContext, repoSetup, branch, branch)
}

// WorkerOptions is input for the agent-queue worker prompt.
type WorkerOptions struct {
	AgentID            string
	Project            string
	RepoURL            string
	CloneDir           string
	Branch             string
	AQScripts          string
	ScreenshotQueueDir string
	ResultsDir         string
}

// BuildWorker returns the prompt used by daemon-spawned workers that follow
// the agent-queue claim → fix → merge loop.
func BuildWorker(o WorkerOptions) string {
	return fmt.Sprintf(`You are a screenshot-driven code fix worker agent (%s).
You process screenshot tasks from the agent-queue, fixing code issues shown in screenshots.

## Environment

- Agent ID: %s
- Project: %s
- Working directory: %s
- Branch: %s
- Agent-queue scripts: %s
- Screenshot tasks dir: %s
- Results dir: %s

## Worker loop

Run this loop until there are no more items to claim:

### 1. Sync with main

`+"```bash\n%s/agent-queue sync --dir %s\n```"+`

### 2. Claim next item

`+"```bash\nITEM=$(%s/agent-queue claim -p %s --agent %s)\n```"+`

If claim returns empty or fails, exit successfully.

Parse the JSON to get the item ID and title. Title format: `+"`xmuggle-fix:<task-id>`"+`.

### 3. Read the screenshot

Extract the task-id from the title. Then:
- Read %s/<task-id>/task.json (contains repo, message)
- Read %s/<task-id>/screenshot.* (use Read tool — images render natively)

### 4. Fix the issue in the cloned repo

You are already in %s on branch %s. Explore, find files, make surgical changes.

### 5. Commit and merge

`+"```bash\ngit add <files>\ngit commit -m \"fix: <what you fixed>\"\n%s/agent-merge %s --delete-branch\n```"+`

On merge failure:
`+"```bash\n%s/agent-queue fail -p %s <item-id> --reason \"merge conflict\"\ngit checkout main && git pull --ff-only origin main\ngit checkout -b %s\n```"+`

### 6. Complete the item

`+"```bash\n%s/agent-queue complete -p %s <item-id> --branch %s\n```"+`

### 7. Write result file

`+"```bash\nmkdir -p %s/<task-id>\ncat > %s/<task-id>/result.json <<'RESULT'\n{\n  \"status\": \"success\",\n  \"branch\": \"%s\",\n  \"summary\": \"<brief description>\",\n  \"timestamp\": <unix ms>\n}\nRESULT\n```"+`

### 8. Reset and loop

`+"```bash\ngit checkout main && git pull --ff-only origin main\nBRANCH=\"%s-%s-$(date +%%s)\"\ngit checkout -b \"$BRANCH\"\n```"+`

Then back to step 1.

## Important

- Always read files before editing.
- If a screenshot is unclear, write an error result and continue.
- Always write a result file.
- One task at a time.
`,
		o.AgentID, o.AgentID, o.Project, o.CloneDir, o.Branch,
		o.AQScripts, o.ScreenshotQueueDir, o.ResultsDir,
		o.AQScripts, o.CloneDir,
		o.AQScripts, o.Project, o.AgentID,
		o.ScreenshotQueueDir, o.ScreenshotQueueDir,
		o.CloneDir, o.Branch,
		o.AQScripts, o.Branch,
		o.AQScripts, o.Project, o.Branch,
		o.AQScripts, o.Project, o.Branch,
		o.ResultsDir, o.ResultsDir, o.Branch,
		o.Project, o.AgentID,
	)
}
