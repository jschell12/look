// Package images tracks screenshots on ~/Desktop via a JSON index stored
// in <repo>/.xmuggle/images.json. Images are never copied — they stay on
// the Desktop and are referenced by their original path.
package images

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jschell12/xmuggle/internal/config"
	"github.com/jschell12/xmuggle/internal/gitops"
)

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true,
}

func isImage(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

// ────────────────────────────────────────────────────────────────────
// JSON index
// ────────────────────────────────────────────────────────────────────

func indexPath(repoRoot string) string {
	return config.GetRepoPaths(repoRoot).ImagesFile
}

// ImageEntry is a single tracked image.
type ImageEntry struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"` // "pending" or "done"
	FirstSeen   time.Time  `json:"first_seen"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

type imageIndex struct {
	Images map[string]*ImageEntry `json:"images"` // key = absolute path
}

func loadIndex(repoRoot string) (*imageIndex, error) {
	idx := &imageIndex{Images: make(map[string]*ImageEntry)}
	data, err := os.ReadFile(indexPath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return idx, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, idx); err != nil {
		return nil, err
	}
	if idx.Images == nil {
		idx.Images = make(map[string]*ImageEntry)
	}
	return idx, nil
}

func saveIndex(repoRoot string, idx *imageIndex) error {
	if err := config.EnsureRepoDirs(repoRoot); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(repoRoot), data, 0o644)
}

// ────────────────────────────────────────────────────────────────────
// Desktop scanning
// ────────────────────────────────────────────────────────────────────

func desktopDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Desktop")
}

func desktopImages() ([]Image, error) {
	return listDirImages(desktopDir())
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
		typ := e.Type()
		if !typ.IsRegular() && typ&os.ModeIrregular == 0 {
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

// ────────────────────────────────────────────────────────────────────
// Sync
// ────────────────────────────────────────────────────────────────────

func Sync(repoRoot string) (int, error) {
	idx, err := loadIndex(repoRoot)
	if err != nil {
		return 0, err
	}
	imgs, err := desktopImages()
	if err != nil {
		return 0, err
	}
	now := time.Now()
	count := 0
	for _, img := range imgs {
		if _, exists := idx.Images[img.Path]; exists {
			continue
		}
		idx.Images[img.Path] = &ImageEntry{
			Name:      img.Name,
			Status:    "pending",
			FirstSeen: now,
		}
		count++
	}
	for p := range idx.Images {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			delete(idx.Images, p)
		}
	}
	if err := saveIndex(repoRoot, idx); err != nil {
		return 0, err
	}
	return count, nil
}

// ────────────────────────────────────────────────────────────────────
// Public API
// ────────────────────────────────────────────────────────────────────

type Image struct {
	Path        string
	Name        string
	IsProcessed bool
	ModTime     time.Time
}

func mustRepoRoot() string {
	root, err := gitops.FindRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "not in a git repo")
		os.Exit(1)
	}
	return root
}

func ListAll() ([]Image, error) {
	root := mustRepoRoot()
	if _, err := Sync(root); err != nil {
		return nil, err
	}
	return indexedImages(root)
}

func indexedImages(repoRoot string) ([]Image, error) {
	idx, err := loadIndex(repoRoot)
	if err != nil {
		return nil, err
	}
	var out []Image
	for p, entry := range idx.Images {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		out = append(out, Image{
			Path:        p,
			Name:        entry.Name,
			IsProcessed: entry.Status == "done",
			ModTime:     fi.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

func Latest() (*Image, error) {
	root := mustRepoRoot()
	if _, err := Sync(root); err != nil {
		return nil, err
	}
	imgs, err := indexedImages(root)
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

func AllUnprocessed() ([]Image, error) {
	root := mustRepoRoot()
	if _, err := Sync(root); err != nil {
		return nil, err
	}
	imgs, err := indexedImages(root)
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

func FindByName(query string) (*Image, error) {
	root := mustRepoRoot()
	if _, err := Sync(root); err != nil {
		return nil, err
	}
	imgs, err := indexedImages(root)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	for _, img := range imgs {
		if img.Name == query {
			return &img, nil
		}
	}
	var prefix []Image
	for _, img := range imgs {
		if strings.HasPrefix(strings.ToLower(img.Name), q) {
			prefix = append(prefix, img)
		}
	}
	if len(prefix) == 1 {
		return &prefix[0], nil
	}
	for _, img := range imgs {
		if strings.Contains(strings.ToLower(img.Name), q) {
			return &img, nil
		}
	}
	return nil, nil
}

func MarkProcessed(absPath string) error {
	root := mustRepoRoot()
	_ = os.Remove(absPath)
	idx, err := loadIndex(root)
	if err != nil {
		return err
	}
	delete(idx.Images, absPath)
	return saveIndex(root, idx)
}

func Remove(absPath string) error {
	root := mustRepoRoot()
	idx, err := loadIndex(root)
	if err != nil {
		return err
	}
	delete(idx.Images, absPath)
	return saveIndex(root, idx)
}

func RemoveByName(query string) (string, error) {
	img, err := FindByName(query)
	if err != nil {
		return "", err
	}
	if img == nil {
		return "", fmt.Errorf("no image matching %q", query)
	}
	if err := Remove(img.Path); err != nil {
		return "", err
	}
	return img.Name, nil
}

func RemoveAllDone() ([]string, error) {
	root := mustRepoRoot()
	idx, err := loadIndex(root)
	if err != nil {
		return nil, err
	}
	var removed []string
	for p, entry := range idx.Images {
		if entry.Status != "done" {
			continue
		}
		removed = append(removed, entry.Name)
		delete(idx.Images, p)
	}
	if err := saveIndex(root, idx); err != nil {
		return nil, err
	}
	return removed, nil
}
