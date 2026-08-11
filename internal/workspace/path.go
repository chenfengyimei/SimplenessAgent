// Package workspace protects the filesystem boundary selected by the user.
package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func NormalizeRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", contracts.NewError(contracts.ErrInvalidInput, "workspace path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", contracts.NewError(contracts.ErrInvalidInput, "workspace path must be an existing directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", contracts.NewError(contracts.ErrPathDenied, "workspace root cannot be a symbolic link")
	}
	return filepath.Clean(abs), nil
}

func ResolveWithin(root, candidate string) (string, error) {
	if strings.TrimSpace(candidate) == "" {
		candidate = "."
	}
	root, err := NormalizeRoot(root)
	if err != nil {
		return "", err
	}
	target := candidate
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", contracts.NewError(contracts.ErrPathDenied, "target is outside the authorized workspace")
	}
	// Symbolic links and Windows reparse points are denied in the foundation
	// runtime rather than resolved optimistically. This is fail-closed and
	// prevents a link inside a workspace from crossing its authorization root.
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			break
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", contracts.NewError(contracts.ErrPathDenied, "symbolic links are not allowed in workspace tool paths")
		}
	}
	return target, nil
}
