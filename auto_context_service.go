package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/schema"

	"shotgun_code/internal/llm/provider"
)

//go:embed design/prompts/contextPreparation.md
var embeddedPromptFS embed.FS

const (
	autoContextTemplatePath = "design/prompts/contextPreparation.md"
	maxAutoContextTreeChars = 15_000
)

var errAutoContextTreeTooLarge = errors.New("auto context file tree exceeds the allowed size")

type autoContextParser struct{}

type AutoContextResult struct {
	Files     []string `json:"files"`
	Reasoning string   `json:"reasoning,omitempty"`
}

func (autoContextParser) Parse(text string) (AutoContextResult, error) {
	return parseAutoContextJSON(text)
}

func (autoContextParser) ParseWithPrompt(text string, _ schema.PromptValue) (AutoContextResult, error) {
	return parseAutoContextJSON(text)
}

func (autoContextParser) GetFormatInstructions() string {
	return "Respond ONLY with a JSON object that matches this schema:\n" +
		"```\n{\n  \"files\": [\"relative/path/from/project/root\"],\n  \"reasoning\": \"optional short description\"\n}\n```\n" +
		"No code fences, commentary, or explanations outside the JSON object."
}

func (autoContextParser) Type() string {
	return "auto_context_json_parser"
}

type AutoContextService struct {
	parser         autoContextParser
	templateMu     sync.Mutex
	template       prompts.PromptTemplate
	templateLoaded bool
}

func NewAutoContextService() *AutoContextService {
	return &AutoContextService{}
}

func (s *AutoContextService) ensureTemplate() error {
	s.templateMu.Lock()
	defer s.templateMu.Unlock()
	if s.templateLoaded {
		return nil
	}

	var templateBody string
	if bytes, err := os.ReadFile(autoContextTemplatePath); err == nil {
		templateBody = string(bytes)
	} else {
		content, readErr := embeddedPromptFS.ReadFile(autoContextTemplatePath)
		if readErr != nil {
			return fmt.Errorf("failed to load auto-context prompt template: %w", readErr)
		}
		templateBody = string(content)
	}

	s.template = prompts.NewPromptTemplate(
		templateBody,
		[]string{"FILE_TREE", "USER_TASK", "CURRENT_UNDERSTANDING"},
	)
	s.templateLoaded = true
	return nil
}

func (s *AutoContextService) BuildPrompt(fileTree, userTask, understanding string) (string, error) {
	if err := s.ensureTemplate(); err != nil {
		return "", err
	}

	formatted, err := s.template.Format(map[string]any{
		"FILE_TREE":             fileTree,
		"USER_TASK":             userTask,
		"CURRENT_UNDERSTANDING": understanding,
	})
	if err != nil {
		return "", fmt.Errorf("failed to render auto-context prompt: %w", err)
	}

	return strings.TrimSpace(formatted) + "\n\n" + s.parser.GetFormatInstructions(), nil
}

func (s *AutoContextService) ParseResponse(text string) (AutoContextResult, error) {
	return s.parser.Parse(text)
}

func parseAutoContextJSON(text string) (AutoContextResult, error) {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return AutoContextResult{}, errors.New("empty response from LLM")
	}

	// Strip markdown fences if present.
	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```JSON")
		cleaned = strings.TrimPrefix(cleaned, "```")
		if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
	}
	cleaned = strings.TrimSpace(cleaned)

	var result AutoContextResult
	decoder := json.NewDecoder(strings.NewReader(cleaned))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return AutoContextResult{}, fmt.Errorf("failed to decode auto-context response: %w", err)
	}

	normalized := make([]string, 0, len(result.Files))
	for _, f := range result.Files {
		f = normalizeRelativePath(f)
		if f != "" {
			normalized = append(normalized, f)
		}
	}
	if len(normalized) == 0 {
		return AutoContextResult{}, errors.New("response did not include any valid files")
	}
	result.Files = normalized
	return result, nil
}

func writeAutoContextTree(builder *strings.Builder, rootDir, rootLabel string, excludedMap map[string]bool) error {
	if builder == nil {
		return errors.New("tree builder is required")
	}
	if strings.TrimSpace(rootDir) == "" {
		return errors.New("project root is required")
	}
	label := strings.TrimSpace(rootLabel)
	if label == "" {
		label = filepath.Base(rootDir)
	}
	if label == "" || label == "." {
		return errors.New("project root label is required")
	}

	appendLine := func(line string) error {
		builder.WriteString(line)
		if builder.Len() > maxAutoContextTreeChars {
			return errAutoContextTreeTooLarge
		}
		return nil
	}

	if err := appendLine(filepath.ToSlash(label) + "/\n"); err != nil {
		return err
	}

	var walk func(string, string) error
	walk = func(currentPath, prefix string) error {
		entries, err := os.ReadDir(currentPath)
		if err != nil {
			return fmt.Errorf("failed to read directory %s: %w", currentPath, err)
		}
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].IsDir() && !entries[j].IsDir() {
				return true
			}
			if !entries[i].IsDir() && entries[j].IsDir() {
				return false
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})

		visibleEntries := make([]os.DirEntry, 0, len(entries))
		for _, entry := range entries {
			relPath, _ := filepath.Rel(rootDir, filepath.Join(currentPath, entry.Name()))
			if excludedMap[normalizeRelativePath(relPath)] {
				continue
			}
			visibleEntries = append(visibleEntries, entry)
		}

		for idx, entry := range visibleEntries {
			branch := "├── "
			nextPrefix := prefix + "│   "
			if idx == len(visibleEntries)-1 {
				branch = "└── "
				nextPrefix = prefix + "    "
			}
			if err := appendLine(prefix + branch + entry.Name() + "\n"); err != nil {
				return err
			}

			if entry.IsDir() {
				if err := walk(filepath.Join(currentPath, entry.Name()), nextPrefix); err != nil {
					return err
				}
			}
		}
		return nil
	}

	return walk(rootDir, "")
}

func buildAutoContextTree(rootDir string, excludedMap map[string]bool) (string, error) {
	var builder strings.Builder
	if err := writeAutoContextTree(&builder, rootDir, filepath.Base(rootDir), excludedMap); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func buildAutoContextTreeForRoots(rootDirs []string, rootLabels map[string]string, excludedByRoot map[string]map[string]bool) (string, error) {
	if len(rootDirs) == 0 {
		return "", errors.New("project root is required")
	}
	rootOrder := make([]string, len(rootDirs))
	copy(rootOrder, rootDirs)
	sort.SliceStable(rootOrder, func(i, j int) bool {
		return rootLabels[rootOrder[i]] < rootLabels[rootOrder[j]]
	})

	var builder strings.Builder
	for idx, rootDir := range rootOrder {
		label := strings.TrimSpace(rootLabels[rootDir])
		if label == "" {
			return "", fmt.Errorf("missing label for root: %s", rootDir)
		}
		excludedMap, ok := excludedByRoot[rootDir]
		if !ok {
			return "", fmt.Errorf("excluded paths missing for root: %s", rootDir)
		}
		if idx > 0 {
			builder.WriteString("\n")
			if builder.Len() > maxAutoContextTreeChars {
				return "", errAutoContextTreeTooLarge
			}
		}
		if err := writeAutoContextTree(&builder, rootDir, label, excludedMap); err != nil {
			return "", err
		}
	}
	if builder.Len() > maxAutoContextTreeChars {
		return "", errAutoContextTreeTooLarge
	}
	return builder.String(), nil
}

func normalizeRelativePath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return ""
	}
	rel = strings.TrimPrefix(rel, "./")
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "/")
	return rel
}

// normalizeCandidateForRoot brings an LLM-returned path into the canonical
// "relative to rootDir" form. It accepts either strictly relative paths like
// "frontend/src/..." or paths prefixed with the project root label/name, e.g.:
//   "shotgun_code/frontend/src/..." when rootDir == ".../shotgun_code".
func normalizeCandidateForRoot(rootDir, rootLabel, candidate string) string {
	candidate = normalizeRelativePath(candidate)
	if candidate == "" {
		return ""
	}
	prefixes := make([]string, 0, 2)
	if label := strings.TrimSpace(rootLabel); label != "" && label != "." {
		prefixes = append(prefixes, filepath.ToSlash(label))
	}
	rootBase := filepath.Base(rootDir)
	if rootBase != "" && rootBase != "." && rootBase != rootLabel {
		prefixes = append(prefixes, filepath.ToSlash(rootBase))
	}

	for _, prefix := range prefixes {
		if candidate == prefix {
			return "."
		}
		prefixWithSlash := prefix + "/"
		if strings.HasPrefix(candidate, prefixWithSlash) {
			trimmed := strings.TrimPrefix(candidate, prefixWithSlash)
			if trimmed == "" {
				return "."
			}
			return trimmed
		}
		dotPrefix := "./" + prefixWithSlash
		if strings.HasPrefix(candidate, dotPrefix) {
			trimmed := strings.TrimPrefix(candidate, dotPrefix)
			if trimmed == "" {
				return "."
			}
			return trimmed
		}
	}

	return candidate
}

func resolveLLMSelection(rootDir, rootLabel string, candidates []string, allowEmpty bool) ([]string, error) {
	if len(candidates) == 0 {
		return nil, errors.New("no candidate paths provided")
	}
	selected := make(map[string]struct{})
	for _, candidate := range candidates {
		candidate = normalizeCandidateForRoot(rootDir, rootLabel, candidate)
		if candidate == "" {
			continue
		}
		absPath := filepath.Join(rootDir, filepath.FromSlash(candidate))
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		if info.IsDir() {
			filepath.WalkDir(absPath, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					return nil
				}
				rel, relErr := filepath.Rel(rootDir, path)
				if relErr != nil {
					return nil
				}
				rel = normalizeRelativePath(rel)
				if rel != "" {
					selected[rel] = struct{}{}
				}
				return nil
			})
		} else {
			selected[filepath.ToSlash(candidate)] = struct{}{}
		}
	}

	if len(selected) == 0 {
		if allowEmpty {
			return []string{}, nil
		}
		return nil, errors.New("no existing files matched the LLM selection")
	}

	sorted := make([]string, 0, len(selected))
	for rel := range selected {
		sorted = append(sorted, rel)
	}
	sort.Strings(sorted)
	return sorted, nil
}

func resolveLLMSelectionForRoots(rootDirs []string, rootLabels map[string]string, candidates []string) (map[string][]string, error) {
	if len(rootDirs) == 0 {
		return nil, errors.New("project root is required")
	}
	labelToRoot := make(map[string]string, len(rootDirs))
	for _, rootDir := range rootDirs {
		label := strings.TrimSpace(rootLabels[rootDir])
		if label == "" {
			return nil, fmt.Errorf("missing label for root: %s", rootDir)
		}
		if _, exists := labelToRoot[label]; exists {
			return nil, fmt.Errorf("duplicate root label: %s", label)
		}
		labelToRoot[label] = rootDir
	}

	normalized := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if normalizedCandidate := normalizeRelativePath(candidate); normalizedCandidate != "" {
			normalized = append(normalized, normalizedCandidate)
		}
	}
	if len(normalized) == 0 {
		return nil, errors.New("no candidate paths provided")
	}

	grouped := make(map[string][]string, len(rootDirs))
	for _, rootDir := range rootDirs {
		grouped[rootDir] = []string{}
	}

	if len(rootDirs) == 1 {
		rootDir := rootDirs[0]
		grouped[rootDir] = append(grouped[rootDir], normalized...)
	} else {
		for _, candidate := range normalized {
			parts := strings.SplitN(candidate, "/", 2)
			label := parts[0]
			rootDir, ok := labelToRoot[label]
			if !ok {
				return nil, fmt.Errorf("path %q does not include a known root label", candidate)
			}
			grouped[rootDir] = append(grouped[rootDir], candidate)
		}
	}

	selectedByRoot := make(map[string][]string)
	totalSelected := 0
	for _, rootDir := range rootDirs {
		rootCandidates := grouped[rootDir]
		if len(rootCandidates) == 0 {
			continue
		}
		selected, err := resolveLLMSelection(rootDir, rootLabels[rootDir], rootCandidates, true)
		if err != nil {
			return nil, err
		}
		if len(selected) == 0 {
			continue
		}
		selectedByRoot[rootDir] = selected
		totalSelected += len(selected)
	}

	if totalSelected == 0 {
		return nil, errors.New("no existing files matched the LLM selection")
	}

	return selectedByRoot, nil
}

func buildProviderConfig(settings LLMSettings) provider.Config {
	return provider.Config{
		Provider: settings.ActiveProvider,
		Model:    fallbackModel(settings),
		APIKey:   settings.keyForProvider(settings.ActiveProvider),
		BaseURL:  strings.TrimSpace(settings.BaseURL),
	}
}

func fallbackModel(settings LLMSettings) string {
	model := strings.TrimSpace(settings.Model)
	if model != "" {
		return model
	}
	return defaultModelForProvider(settings.ActiveProvider)
}
