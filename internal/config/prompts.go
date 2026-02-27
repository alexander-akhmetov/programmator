// Package config provides unified configuration management for programmator.
package config

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

//go:embed defaults/prompts/*.md
var promptsFS embed.FS

// Prompts holds all loaded prompt templates.
// Each prompt is a Go text/template string with named variables.
type Prompts struct {
	Phased      string // Template for phased execution (has checkboxed tasks)
	Phaseless   string // Template for phaseless execution (single task)
	ReviewFirst string // Template for review fix prompt
}

// promptLoader handles loading prompts with fallback chain.
type promptLoader struct {
	embedFS embed.FS
}

// newPromptLoader creates a new prompt loader with the embedded filesystem.
func newPromptLoader(embedFS embed.FS) *promptLoader {
	return &promptLoader{embedFS: embedFS}
}

// LoadPrompts loads all prompt templates with fallback chain: local → global → embedded.
// localDir can be empty to skip local lookup.
func LoadPrompts(globalDir, localDir string) (*Prompts, error) {
	loader := newPromptLoader(promptsFS)
	return loader.Load(globalDir, localDir)
}

// Load loads all prompt files with fallback chain: local → global → embedded.
func (p *promptLoader) Load(globalDir, localDir string) (*Prompts, error) {
	var prompts Prompts
	var err error

	prompts.Phased, err = p.loadPromptWithLocalFallback(localDir, globalDir, "phased.md")
	if err != nil {
		return nil, fmt.Errorf("load phased prompt: %w", err)
	}

	prompts.Phaseless, err = p.loadPromptWithLocalFallback(localDir, globalDir, "phaseless.md")
	if err != nil {
		return nil, fmt.Errorf("load phaseless prompt: %w", err)
	}

	prompts.ReviewFirst, err = p.loadPromptWithLocalFallback(localDir, globalDir, "review_first.md")
	if err != nil {
		return nil, fmt.Errorf("load review_first prompt: %w", err)
	}

	return &prompts, nil
}

// loadPromptWithLocalFallback loads a prompt file with fallback chain: local → global → embedded.
// localDir can be empty to skip local lookup.
func (p *promptLoader) loadPromptWithLocalFallback(localDir, globalDir, filename string) (string, error) {
	// Try local first (.programmator/prompts/)
	if localDir != "" {
		content, err := p.loadPromptFile(filepath.Join(localDir, "prompts", filename))
		if err != nil {
			// Log non-fatal local prompt errors and fall through to global/embedded
			log.Printf("warning: failed to load local prompt %s: %v (falling back to global/embedded)", filename, err)
		} else if content != "" {
			return content, nil
		}
	}

	// Fall back to global (~/.config/programmator/prompts/) → embedded
	return p.loadPromptWithFallback(
		filepath.Join(globalDir, "prompts", filename),
		"defaults/prompts/"+filename,
	)
}

// loadPromptWithFallback tries to load a prompt from a user file first,
// falling back to the embedded filesystem if the user file doesn't exist or is empty.
func (p *promptLoader) loadPromptWithFallback(userPath, embedPath string) (string, error) {
	content, err := p.loadPromptFile(userPath)
	if err != nil {
		return "", err
	}
	if content != "" {
		return content, nil
	}
	return p.loadPromptFromEmbedFS(embedPath)
}

// loadPromptFile reads a prompt file from disk.
// Returns empty string (not error) if file doesn't exist.
// Comment lines (starting with #) are stripped.
func (p *promptLoader) loadPromptFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed internally
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read prompt file %s: %w", path, err)
	}
	return strings.TrimSpace(stripComments(string(data))), nil
}

// loadPromptFromEmbedFS reads a prompt file from the embedded filesystem.
// Returns empty string (not error) if file doesn't exist.
// Comment lines (starting with #) are stripped.
func (p *promptLoader) loadPromptFromEmbedFS(path string) (string, error) {
	data, err := p.embedFS.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read embedded prompt %s: %w", path, err)
	}
	return strings.TrimSpace(stripComments(string(data))), nil
}

// stripComments removes lines starting with # (comment lines) from content.
// Empty lines are preserved, inline comments are not supported.
// Handles both Unix (LF) and Windows (CRLF) line endings.
func stripComments(content string) string {
	// Normalize line endings: convert CRLF to LF
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Pre-allocate with estimated capacity
	lines := make([]string, 0, strings.Count(content, "\n")+1)
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
