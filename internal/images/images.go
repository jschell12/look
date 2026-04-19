// Package images tracks screenshots on ~/Desktop via a JSON index stored
// in ~/.xmuggle/images.json.  Images are never copied — they stay on the
// Desktop and are referenced by their original path.
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

func indexPath() string {
	return filepath.Join(config.GetPaths().ConfigDir, "images.json")
}

// ImageEntry is a single tracked image.
type ImageEntry struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"` // "pending" or "done"
	FirstSeen   time.Time `json:"first_seen"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

// imageIndex is the on-disk format.
type imageIndex struct {
	Images map[string]*ImageEntry `json:"images"` // key = absolute path
}

func loadIndex() (*imageIndex, error) {
	idx := &imageIndex{Images: make(map[string]*ImageEntry)}
	data, err := os.ReadFile(indexPath())
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

func saveIndex(idx *imageIndex) error {
	if err := config.EnsureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(), data, 0o644)
}

// ────────────────────────────────────────────────────────────────────
// Desktop scanning
// ────────────────────────────────────────────────────────────────────

func desktopDir() string {
	return filepath.Join(config.GetPaths().Home, "Desktop")
}

// desktopImages returns all image files on ~/Desktop sorted newest-first.
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
		// Accept regular files and ModeIrregular (iCloud-evicted "dataless"
		// files on macOS with Desktop & Documents sync).
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
// Sync: merge Desktop state into the index
// ────────────────────────────────────────────────────────────────────

// Sync discovers images on ~/Desktop and adds new ones to the index.
// Existing entries are preserved.  Returns count of newly added images.
func Sync() (int, error) {
	idx, err := loadIndex()
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

	// Prune entries whose files no longer exist on Desktop
	for p := range idx.Images {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			delete(idx.Images, p)
		}
	}

	if err := saveIndex(idx); err != nil {
		return 0, err
	}
	return count, nil
}

// ────────────────────────────────────────────────────────────────────
// Public API
// ────────────────────────────────────────────────────────────────────

// Image describes an entry visible to callers.
type Image struct {
	Path        string
	Name        string
	IsProcessed bool
	ModTime     time.Time
}

// ListAll syncs and returns all tracked images (newest first).
func ListAll() ([]Image, error) {
	if _, err := Sync(); err != nil {
		return nil, err
	}
	return indexedImages()
}

func indexedImages() ([]Image, error) {
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	var out []Image
	for p, entry := range idx.Images {
		fi, err := os.Stat(p)
		if err != nil {
			continue // file gone
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

// Latest returns the newest unprocessed image after syncing.
func Latest() (*Image, error) {
	if _, err := Sync(); err != nil {
		return nil, err
	}
	imgs, err := indexedImages()
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
	if _, err := Sync(); err != nil {
		return nil, err
	}
	imgs, err := indexedImages()
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

// FindByName resolves a fuzzy query to a single image after syncing.
func FindByName(query string) (*Image, error) {
	if _, err := Sync(); err != nil {
		return nil, err
	}
	imgs, err := indexedImages()
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

// MarkProcessed marks an image as done in the index.
func MarkProcessed(absPath string) error {
	idx, err := loadIndex()
	if err != nil {
		return err
	}
	entry, ok := idx.Images[absPath]
	if !ok {
		// Image not tracked yet — add it
		now := time.Now()
		entry = &ImageEntry{
			Name:      filepath.Base(absPath),
			Status:    "done",
			FirstSeen: now,
		}
		entry.ProcessedAt = &now
		idx.Images[absPath] = entry
	} else {
		now := time.Now()
		entry.Status = "done"
		entry.ProcessedAt = &now
	}
	return saveIndex(idx)
}

// Remove removes an image from the tracking index (does NOT delete the
// file from Desktop).
func Remove(absPath string) error {
	idx, err := loadIndex()
	if err != nil {
		return err
	}
	delete(idx.Images, absPath)
	return saveIndex(idx)
}

// RemoveByName fuzzy-matches and removes a single image from tracking.
// Returns the filename that was removed.
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

// RemoveAllDone removes every processed image from tracking.
// Returns the list of removed filenames.
func RemoveAllDone() ([]string, error) {
	idx, err := loadIndex()
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
	if err := saveIndex(idx); err != nil {
		return nil, err
	}
	return removed, nil
}
