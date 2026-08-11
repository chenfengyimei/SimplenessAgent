// Package skill loads workspace-local, declarative instruction packages.
package skill

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/xm/simplenessagent/internal/workspace"
	"github.com/xm/simplenessagent/pkg/contracts"
)

const (
	skillsDirectory      = ".simpleness/skills"
	manifestName         = "skill.json"
	instructionsName     = "SKILL.md"
	maxManifestBytes     = 64 * 1024
	maxInstructionsBytes = 128 * 1024
)

var skillName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// Discover reads only manifests, enabling a caller to expose a compact Skill
// index without injecting instruction bodies into a model context.
func Discover(root string, definitions []contracts.ToolDefinition) ([]contracts.SkillManifest, error) {
	root, err := workspace.NormalizeRoot(root)
	if err != nil {
		return nil, err
	}
	directory, err := workspace.ResolveWithin(root, skillsDirectory)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []contracts.SkillManifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]contracts.SkillManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, contracts.NewError(contracts.ErrPathDenied, "skill directories cannot be symbolic links")
		}
		item, readErr := readManifest(root, entry.Name(), definitions)
		if readErr != nil {
			return nil, readErr
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

// Load reads one validated skill body on demand. It never executes scripts or
// expands the declared tool set.
func Load(root, name string, definitions []contracts.ToolDefinition) (contracts.Skill, error) {
	manifest, err := readManifest(root, name, definitions)
	if err != nil {
		return contracts.Skill{}, err
	}
	path, err := workspace.ResolveWithin(root, filepath.ToSlash(filepath.Join(skillsDirectory, name, instructionsName)))
	if err != nil {
		return contracts.Skill{}, err
	}
	contents, err := readRegularFile(path, maxInstructionsBytes)
	if err != nil {
		return contracts.Skill{}, err
	}
	if strings.TrimSpace(string(contents)) == "" {
		return contracts.Skill{}, contracts.NewError(contracts.ErrInvalidInput, "skill instructions cannot be empty")
	}
	return contracts.Skill{Manifest: manifest, Instructions: string(contents)}, nil
}

func readManifest(root, name string, definitions []contracts.ToolDefinition) (contracts.SkillManifest, error) {
	if !skillName.MatchString(name) {
		return contracts.SkillManifest{}, contracts.NewError(contracts.ErrInvalidInput, "skill name is invalid")
	}
	path, err := workspace.ResolveWithin(root, filepath.ToSlash(filepath.Join(skillsDirectory, name, manifestName)))
	if err != nil {
		return contracts.SkillManifest{}, err
	}
	contents, err := readRegularFile(path, maxManifestBytes)
	if err != nil {
		return contracts.SkillManifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var manifest contracts.SkillManifest
	if err = decoder.Decode(&manifest); err != nil {
		return contracts.SkillManifest{}, contracts.NewError(contracts.ErrInvalidInput, "skill manifest is not valid JSON")
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return contracts.SkillManifest{}, contracts.NewError(contracts.ErrInvalidInput, "skill manifest has trailing JSON values")
	}
	if err = validateManifest(root, name, manifest, definitions); err != nil {
		return contracts.SkillManifest{}, err
	}
	return manifest, nil
}

func validateManifest(root, directory string, manifest contracts.SkillManifest, definitions []contracts.ToolDefinition) error {
	if manifest.Version != contracts.SchemaVersion || manifest.Name != directory || !skillName.MatchString(manifest.Name) || strings.TrimSpace(manifest.SkillVersion) == "" || strings.TrimSpace(manifest.Description) == "" || len(manifest.WorkspaceScopes) == 0 {
		return contracts.NewError(contracts.ErrInvalidInput, "skill manifest is incomplete or invalid")
	}
	available := map[string]contracts.ToolDefinition{}
	for _, definition := range definitions {
		available[definition.Name] = definition
	}
	seen := map[string]bool{}
	for _, toolName := range manifest.AllowedTools {
		if seen[toolName] {
			return contracts.NewError(contracts.ErrInvalidInput, "skill manifest has duplicate allowed tool")
		}
		seen[toolName] = true
		if _, ok := available[toolName]; !ok {
			return contracts.NewError(contracts.ErrToolNotAllowed, "skill requests an unavailable tool")
		}
	}
	for _, scope := range manifest.WorkspaceScopes {
		if _, err := workspace.ResolveWithin(root, scope); err != nil {
			return err
		}
	}
	return nil
}

func readRegularFile(path string, maximum int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, contracts.NewError(contracts.ErrPathDenied, "skill files must be regular files")
	}
	if info.Size() > int64(maximum) {
		return nil, contracts.NewError(contracts.ErrOutputLimitReached, "skill file exceeds the size limit")
	}
	return os.ReadFile(path)
}
