// Package images manages the ~/.xmuggle image store, auto-ingesting screenshots
// from ~/Desktop via macOS Spotlight (kMDItemIsScreenCapture).
package images

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jschell12/xmuggle/internal/config"
)

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true,
}

func trackedFile() string { return filepath.Join(config.GetPaths().ConfigDir, ".tracked") }
func seenFile() string    { return filepath.Join(config.GetPaths().ConfigDir, ".seen") }

// ensureDirAndFiles makes sure .tracked and .seen exist.
func ensureDirAndFiles() error {
	if err := config.EnsureDirs(); err != nil {
		return err
	}
	for _, f := range []string{trackedFile(), seenFile()} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			if err := os.WriteFile(f, []byte("# Managed by xmuggle\n"), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadSet(path string) (map[string]struct{}, error) {
	set := map[string]struct{}{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return set, nil
		}
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[line] = struct{}{}
	}
	return set, s.Err()
}

func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

func isImage(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

// Image describes an entry in the store.
type Image struct {
	Path        string
	Name        string
	IsProcessed bool
	ModTime     time.Time
}

func listDirImages(dir string) ([]Image, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Image
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !isImage(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Image{
			Path:    filepath.Join(dir, e.Name()),
			Name:    e.Name(),
			ModTime: fi.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// MarkProcessed appends the filename to .tracked.
func MarkProcessed(absPath string) error {
	if err := ensureDirAndFiles(); err != nil {
		return err
	}
	return appendLine(trackedFile(), filepath.Base(absPath))
}

// markSeen appends a source path to .seen to prevent re-ingesting.
func markSeen(src string) error {
	return appendLine(seenFile(), src)
}

// ingestCopy copies src into ~/.xmuggle/, deduping on name collision.
func ingestCopy(src string) (string, error) {
	if err := ensureDirAndFiles(); err != nil {
		return "", err
	}
	root := config.GetPaths().ConfigDir
	name := filepath.Base(src)
	dest := filepath.Join(root, name)
	if _, err := os.Stat(dest); err == nil {
		ext := filepath.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		dest = filepath.Join(root, fmt.Sprintf("%s-%d%s", stem, time.Now().UnixMilli(), ext))
	}
	if err := copyFile(src, dest); err != nil {
		return "", err
	}
	if err := markSeen(src); err != nil {
		return "", err
	}
	return dest, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// findNewScreenshots uses Spotlight to list screenshots not yet in .seen.
func findNewScreenshots() ([]string, error) {
	seen, err := loadSet(seenFile())
	if err != nil {
		return nil, err
	}

	home := config.GetPaths().Home
	cmd := exec.Command(
		"mdfind",
		"kMDItemIsScreenCapture = 1",
		"-onlyin", filepath.Join(home, "Desktop"),
	)
	out, err := cmd.Output()
	if err != nil {
		// Fallback: scan ~/Desktop for Screenshot*.png
		imgs, _ := listDirImages(filepath.Join(home, "Desktop"))
		var cand []string
		for _, img := range imgs {
			if strings.HasPrefix(img.Name, "Screenshot") {
				cand = append(cand, img.Path)
			}
		}
		return filterUnseen(cand, seen), nil
	}

	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}
	return filterUnseen(lines, seen), nil
}

func filterUnseen(paths []string, seen map[string]struct{}) []string {
	var out []string
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// AutoIngest pulls new screenshots from ~/Desktop into ~/.xmuggle. Returns count.
func AutoIngest() (int, error) {
	newShots, err := findNewScreenshots()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, p := range newShots {
		if _, err := ingestCopy(p); err == nil {
			count++
		}
	}
	return count, nil
}

// IngestAll copies ALL images from ~/Desktop (not just screenshots).
func IngestAll() (int, error) {
	seen, err := loadSet(seenFile())
	if err != nil {
		return 0, err
	}
	imgs, err := listDirImages(filepath.Join(config.GetPaths().Home, "Desktop"))
	if err != nil {
		return 0, err
	}
	count := 0
	for _, img := range imgs {
		if _, ok := seen[img.Path]; ok {
			continue
		}
		if _, err := ingestCopy(img.Path); err == nil {
			count++
		}
	}
	return count, nil
}

// storeImages lists images currently in ~/.xmuggle with status.
func storeImages() ([]Image, error) {
	if err := ensureDirAndFiles(); err != nil {
		return nil, err
	}
	tracked, err := loadSet(trackedFile())
	if err != nil {
		return nil, err
	}
	imgs, err := listDirImages(config.GetPaths().ConfigDir)
	if err != nil {
		return nil, err
	}
	for i := range imgs {
		_, ok := tracked[imgs[i].Name]
		imgs[i].IsProcessed = ok
	}
	return imgs, nil
}

// ListAll returns every image in the store.
func ListAll() ([]Image, error) { return storeImages() }

// Latest returns the newest unprocessed image, auto-ingesting first.
func Latest() (*Image, error) {
	if _, err := AutoIngest(); err != nil {
		return nil, err
	}
	imgs, err := storeImages()
	if err != nil {
		return nil, err
	}
	for _, img := range imgs {
		if !img.IsProcessed {
			return &img, nil
		}
	}
	return nil, nil
}

// AllUnprocessed returns every unprocessed image, newest first.
func AllUnprocessed() ([]Image, error) {
	if _, err := AutoIngest(); err != nil {
		return nil, err
	}
	imgs, err := storeImages()
	if err != nil {
		return nil, err
	}
	var out []Image
	for _, img := range imgs {
		if !img.IsProcessed {
			out = append(out, img)
		}
	}
	return out, nil
}

// FindByName resolves a fuzzy query to a single image, auto-ingesting first.
func FindByName(query string) (*Image, error) {
	if _, err := AutoIngest(); err != nil {
		return nil, err
	}
	imgs, err := storeImages()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)

	// exact
	for _, img := range imgs {
		if img.Name == query {
			return &img, nil
		}
	}
	// prefix
	var prefix []Image
	for _, img := range imgs {
		if strings.HasPrefix(strings.ToLower(img.Name), q) {
			prefix = append(prefix, img)
		}
	}
	if len(prefix) == 1 {
		return &prefix[0], nil
	}
	// substring, newest wins
	for _, img := range imgs {
		if strings.Contains(strings.ToLower(img.Name), q) {
			return &img, nil
		}
	}
	return nil, nil
}
