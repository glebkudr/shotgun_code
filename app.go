package main

import (
	"context"
	"crypto/sha1"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/fsnotify/fsnotify"
	gitignore "github.com/sabhiram/go-gitignore"
	"github.com/tmc/tokencount/openaitokenizer"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"shotgun_code/internal/labgradient"
)

const defaultMaxOutputSizeBytes = 50_000_000 // 50MB
var ErrContextTooLong = errors.New("context is too long")

//go:embed ignore.glob
var defaultCustomIgnoreRulesContent string

const defaultCustomPromptRulesContent = "пиши по-русски"
const legacyDefaultCustomPromptRulesContent = "no additional rules"

const (
	LLMProviderOpenAI     = "openai"
	LLMProviderOpenRouter = "openrouter"
	LLMProviderGemini     = "gemini"
)

type LLMSettings struct {
	ActiveProvider string `json:"activeProvider"`
	Model          string `json:"model"`
	OpenAIKey      string `json:"openAIKey"`
	OpenRouterKey  string `json:"openRouterKey"`
	GeminiKey      string `json:"geminiKey"`
	BaseURL        string `json:"baseURL"`
}

type AppSettings struct {
	CustomIgnoreRules string      `json:"customIgnoreRules"`
	CustomPromptRules string      `json:"customPromptRules"`
	LLMSettings       LLMSettings `json:"llmSettings"`
	ProjectRoots      []string    `json:"projectRoots"`
	MaxContextBytes   int64       `json:"maxContextBytes"`
}

type App struct {
	ctx                         context.Context
	contextGenerator            *ContextGenerator
	fileWatcher                 *Watchman
	settings                    AppSettings
	currentCustomIgnorePatterns *gitignore.GitIgnore
	configPath                  string
	useGitignore                bool
	useCustomIgnore             bool
	projectGitignores           map[string]*gitignore.GitIgnore // Compiled .gitignore per project root
	autoContextService          *AutoContextService
	historyManager              *HistoryManager
	llmCache                    cachedProvider
	autoContextButtonTexture    string
	tokenEncoder                *openaitokenizer.Encoder
	tokenEncoderErr             error
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.contextGenerator = NewContextGenerator(a)
	a.autoContextService = NewAutoContextService()
	a.historyManager = NewHistoryManager(a)
	a.fileWatcher = NewWatchman(a)
	a.useGitignore = true    // Default to true, matching frontend
	a.useCustomIgnore = true // Default to true, matching frontend

	configFilePath, err := xdg.ConfigFile("shotgun-code/settings.json")
	if err != nil {
		runtime.LogErrorf(a.ctx, "Error getting config file path: %v. Using defaults and will attempt to save later if rules are modified.", err)
		// configPath will be empty, loadSettings will handle this by using defaults
		// and saveSettings will fail gracefully if configPath remains empty and saving is attempted.
	}
	a.configPath = configFilePath

	a.loadSettings()
	// Initialize history after config path is set
	if err := a.historyManager.LoadHistory(); err != nil {
		runtime.LogWarningf(a.ctx, "Failed to load prompt history: %v", err)
	}

	// Ensure CustomPromptRules has a default if it's empty after loading
	if strings.TrimSpace(a.settings.CustomPromptRules) == "" {
		a.settings.CustomPromptRules = defaultCustomPromptRulesContent
	}

	a.initAutoContextButtonTexture()
	a.initTokenEncoder()
}

func (a *App) initAutoContextButtonTexture() {
	params := labgradient.SliceParams{
		L:               80.0,
		Radius:          60.0,
		VerticalSpanDeg: 30.0,
		// CenterAngleDeg and HorizontalSpanDeg are ignored by GeneratePanoramicTexture
	}

	// Generate a seamless 360-degree texture.
	// Width 3072 provides ~8.5 pixels per degree (matching previous 512px/60deg).
	texture, err := labgradient.GeneratePanoramicTexture(3072, 64, params)
	if err != nil {
		runtime.LogErrorf(a.ctx, "failed to generate auto-context LAB texture: %v", err)
		return
	}

	a.autoContextButtonTexture = texture
	runtime.LogDebug(a.ctx, "auto-context LAB texture generated successfully")
}

func (a *App) initTokenEncoder() {
	enc, err := openaitokenizer.NewEncoder("o200k_base")
	if err != nil {
		a.tokenEncoderErr = err
		runtime.LogErrorf(a.ctx, "failed to initialize tokenizer (o200k_base): %v", err)
		return
	}
	a.tokenEncoder = enc
}

// GetAutoContextButtonTexture returns a data URL with the LAB gradient texture for the Auto context button.
func (a *App) GetAutoContextButtonTexture() string {
	return a.autoContextButtonTexture
}

type FileNode struct {
	Name            string      `json:"name"`
	Path            string      `json:"path"`    // Full path
	RelPath         string      `json:"relPath"` // Path relative to selected root
	RootPath        string      `json:"rootPath"`
	RootLabel       string      `json:"rootLabel"`
	SizeBytes       int64       `json:"sizeBytes"`
	TokensApprox    int64       `json:"tokensApprox"`
	IsDir           bool        `json:"isDir"`
	Children        []*FileNode `json:"children,omitempty"`
	IsGitignored    bool        `json:"isGitignored"`    // True if path matches a .gitignore rule
	IsCustomIgnored bool        `json:"isCustomIgnored"` // True if path matches a ignore.glob rule
}

type ProjectRoot struct {
	Path  string `json:"path"`
	Label string `json:"label"`
}

// SelectDirectory opens a dialog to select a directory and returns the chosen path
func (a *App) SelectDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{})
}

func normalizeAndValidateRoots(input []string, requireExists bool) ([]string, error) {
	if len(input) == 0 {
		return []string{}, nil
	}
	seen := make(map[string]bool, len(input))
	normalized := make([]string, 0, len(input))
	for _, raw := range input {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, errors.New("project root is required")
		}
		absPath, err := filepath.Abs(trimmed)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve project root %q: %w", trimmed, err)
		}
		absPath = filepath.Clean(absPath)
		if seen[absPath] {
			return nil, fmt.Errorf("duplicate project root: %s", absPath)
		}
		if requireExists {
			info, err := os.Stat(absPath)
			if err != nil {
				return nil, fmt.Errorf("project root not accessible: %w", err)
			}
			if !info.IsDir() {
				return nil, fmt.Errorf("project root is not a directory: %s", absPath)
			}
		}
		seen[absPath] = true
		normalized = append(normalized, absPath)
	}
	return normalized, nil
}

func computeRootLabels(rootDirs []string) map[string]string {
	labels := make(map[string]string, len(rootDirs))
	baseCounts := make(map[string]int, len(rootDirs))
	for _, rootDir := range rootDirs {
		baseCounts[filepath.Base(rootDir)]++
	}
	for _, rootDir := range rootDirs {
		base := filepath.Base(rootDir)
		if baseCounts[base] == 1 {
			labels[rootDir] = base
			continue
		}
		sum := sha1.Sum([]byte(rootDir))
		hash := fmt.Sprintf("%x", sum)
		labels[rootDir] = fmt.Sprintf("%s~%s", base, hash[:6])
	}
	return labels
}

func buildProjectRootList(rootDirs []string) []ProjectRoot {
	labels := computeRootLabels(rootDirs)
	result := make([]ProjectRoot, 0, len(rootDirs))
	for _, rootDir := range rootDirs {
		result = append(result, ProjectRoot{
			Path:  rootDir,
			Label: labels[rootDir],
		})
	}
	return result
}

func sumChildSizes(children []*FileNode) int64 {
	var total int64
	for _, child := range children {
		if child == nil {
			continue
		}
		total += child.SizeBytes
	}
	return total
}
func sumChildTokens(children []*FileNode) int64 {
	var total int64
	for _, child := range children {
		if child == nil {
			continue
		}
		total += child.TokensApprox
	}
	return total
}

func resolveDirPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
func (a *App) getTokenEncoder() (*openaitokenizer.Encoder, error) {
	if a.tokenEncoderErr != nil {
		return nil, fmt.Errorf("tokenizer initialization failed: %w", a.tokenEncoderErr)
	}
	if a.tokenEncoder == nil {
		return nil, errors.New("tokenizer is not initialized")
	}
	return a.tokenEncoder, nil
}

func (a *App) CountTokens(text string) (int64, error) {
	encoder, err := a.getTokenEncoder()
	if err != nil {
		return 0, err
	}
	if text == "" {
		return 0, nil
	}
	return int64(encoder.Count(text)), nil
}

func (a *App) contextLimitBytes() int64 {
	if a.settings.MaxContextBytes > 0 {
		return a.settings.MaxContextBytes
	}
	return defaultMaxOutputSizeBytes
}

func computeDirStats(encoder *openaitokenizer.Encoder, rootPath string, visited map[string]bool) (int64, int64, error) {
	if encoder == nil {
		return 0, 0, errors.New("tokenizer is not initialized")
	}
	if visited == nil {
		return 0, 0, errors.New("visited map is required")
	}
	rootResolved, err := resolveDirPath(rootPath)
	if err != nil {
		return 0, 0, err
	}
	if visited[rootResolved] {
		return 0, 0, nil
	}
	visited[rootResolved] = true
	defer delete(visited, rootResolved)

	var totalBytes int64
	var totalTokens int64
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return 0, 0, err
	}
	for _, entry := range entries {
		entryPath := filepath.Join(rootPath, entry.Name())
		entryIsDir := entry.IsDir()
		isSymlink := entry.Type()&os.ModeSymlink != 0
		if isSymlink && !entryIsDir {
			info, statErr := os.Stat(entryPath)
			if statErr != nil {
				return 0, 0, statErr
			}
			if info.IsDir() {
				entryIsDir = true
			}
		}
		if entryIsDir {
			resolved, resolveErr := resolveDirPath(entryPath)
			if resolveErr != nil {
				return 0, 0, resolveErr
			}
			if visited[resolved] {
				continue
			}
			subBytes, subTokens, subErr := computeDirStats(encoder, entryPath, visited)
			if subErr != nil {
				return 0, 0, subErr
			}
			totalBytes += subBytes
			totalTokens += subTokens
			continue
		}
		content, readErr := os.ReadFile(entryPath)
		if readErr != nil {
			return 0, 0, readErr
		}
		totalBytes += int64(len(content))
		totalTokens += int64(encoder.Count(string(content)))
	}
	return totalBytes, totalTokens, nil
}

func (a *App) GetProjectRoots() ([]ProjectRoot, error) {
	roots, err := normalizeAndValidateRoots(a.settings.ProjectRoots, false)
	if err != nil {
		return nil, err
	}
	return buildProjectRootList(roots), nil
}

func (a *App) AddProjectRoot(path string) ([]ProjectRoot, error) {
	normalized, err := normalizeAndValidateRoots([]string{path}, true)
	if err != nil {
		return nil, err
	}
	newRoot := normalized[0]
	existing, err := normalizeAndValidateRoots(a.settings.ProjectRoots, false)
	if err != nil {
		return nil, err
	}
	for _, root := range existing {
		if root == newRoot {
			return nil, fmt.Errorf("project root already added: %s", newRoot)
		}
	}
	updated := append(existing, newRoot)
	a.settings.ProjectRoots = updated
	if err := a.saveSettings(); err != nil {
		return nil, err
	}
	return buildProjectRootList(updated), nil
}

func (a *App) RemoveProjectRoot(path string) ([]ProjectRoot, error) {
	normalized, err := normalizeAndValidateRoots([]string{path}, false)
	if err != nil {
		return nil, err
	}
	target := normalized[0]
	existing, err := normalizeAndValidateRoots(a.settings.ProjectRoots, false)
	if err != nil {
		return nil, err
	}
	updated := make([]string, 0, len(existing))
	removed := false
	for _, root := range existing {
		if root == target {
			removed = true
			continue
		}
		updated = append(updated, root)
	}
	if !removed {
		return nil, fmt.Errorf("project root not found: %s", target)
	}
	a.settings.ProjectRoots = updated
	if err := a.saveSettings(); err != nil {
		return nil, err
	}
	return buildProjectRootList(updated), nil
}

// ListFiles lists files and folders in directories, parsing .gitignore if present.
func (a *App) ListFiles(rootDirs []string) ([]*FileNode, error) {
	normalizedRoots, err := normalizeAndValidateRoots(rootDirs, true)
	if err != nil {
		return nil, err
	}
	if len(normalizedRoots) == 0 {
		return []*FileNode{}, nil
	}
	tokenEncoder, err := a.getTokenEncoder()
	if err != nil {
		return nil, err
	}

	rootLabels := computeRootLabels(normalizedRoots)
	a.projectGitignores = make(map[string]*gitignore.GitIgnore, len(normalizedRoots))

	rootNodes := make([]*FileNode, 0, len(normalizedRoots))
	for _, rootDir := range normalizedRoots {
		runtime.LogDebugf(a.ctx, "ListFiles called for directory: %s", rootDir)

		var gitIgn *gitignore.GitIgnore
		gitignorePath := filepath.Join(rootDir, ".gitignore")
		runtime.LogDebugf(a.ctx, "Attempting to find .gitignore at: %s", gitignorePath)
		if _, statErr := os.Stat(gitignorePath); statErr == nil {
			runtime.LogDebugf(a.ctx, ".gitignore found at: %s", gitignorePath)
			compiled, compileErr := gitignore.CompileIgnoreFile(gitignorePath)
			if compileErr != nil {
				runtime.LogWarningf(a.ctx, "Error compiling .gitignore file at %s: %v", gitignorePath, compileErr)
				gitIgn = nil
			} else {
				gitIgn = compiled
				runtime.LogDebug(a.ctx, ".gitignore compiled successfully.")
			}
		} else {
			runtime.LogDebugf(a.ctx, ".gitignore not found at %s (os.Stat error: %v)", gitignorePath, statErr)
			gitIgn = nil
		}
		a.projectGitignores[rootDir] = gitIgn

		rootLabel := rootLabels[rootDir]
		rootNode := &FileNode{
			Name:            rootLabel,
			Path:            rootDir,
			RelPath:         ".",
			RootPath:        rootDir,
			RootLabel:       rootLabel,
			IsDir:           true,
			IsGitignored:    false, // Root itself is not gitignored by default
			IsCustomIgnored: a.currentCustomIgnorePatterns != nil && a.currentCustomIgnorePatterns.MatchesPath("."),
		}

		visited := make(map[string]bool)
		resolvedRoot, resolveErr := resolveDirPath(rootDir)
		if resolveErr != nil {
			return nil, fmt.Errorf("failed to resolve root directory %s: %w", rootDir, resolveErr)
		}
		visited[resolvedRoot] = true
		children, err := buildTreeRecursive(context.TODO(), a.ctx, rootDir, rootDir, rootLabel, gitIgn, a.currentCustomIgnorePatterns, tokenEncoder, visited, 0)
		if err != nil {
			return nil, fmt.Errorf("error building children tree for %s: %w", rootDir, err)
		}
		rootNode.Children = children
		rootNode.SizeBytes = sumChildSizes(children)
		rootNode.TokensApprox = sumChildTokens(children)
		rootNodes = append(rootNodes, rootNode)
	}

	return rootNodes, nil
}

func buildTreeRecursive(ctx context.Context, logCtx context.Context, currentPath, rootPath, rootLabel string, gitIgn *gitignore.GitIgnore, customIgn *gitignore.GitIgnore, tokenEncoder *openaitokenizer.Encoder, visited map[string]bool, depth int) ([]*FileNode, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return nil, err
	}

	var nodes []*FileNode
	for _, entry := range entries {
		nodePath := filepath.Join(currentPath, entry.Name())
		relPath, _ := filepath.Rel(rootPath, nodePath)
		// For gitignore matching, paths should generally be relative to the .gitignore file (rootPath)
		// and use OS-specific separators. go-gitignore handles this.

		isGitignored := false
		isCustomIgnored := false
		entryIsDir := entry.IsDir()
		isSymlink := entry.Type()&os.ModeSymlink != 0
		if isSymlink && !entryIsDir {
			info, statErr := os.Stat(nodePath)
			if statErr != nil {
				runtime.LogWarningf(logCtx, "Failed to stat symlink %s: %v", nodePath, statErr)
			} else if info.IsDir() {
				entryIsDir = true
			}
		}
		var resolvedDirPath string
		if entryIsDir {
			resolved, resolveErr := resolveDirPath(nodePath)
			if resolveErr != nil {
				return nil, fmt.Errorf("failed to resolve directory %s: %w", nodePath, resolveErr)
			}
			resolvedDirPath = resolved
		}

		pathToMatch := relPath
		if entryIsDir {
			if !strings.HasSuffix(pathToMatch, string(os.PathSeparator)) {
				pathToMatch += string(os.PathSeparator)
			}
		}

		if gitIgn != nil {
			isGitignored = gitIgn.MatchesPath(pathToMatch)
		}
		if customIgn != nil {
			isCustomIgnored = customIgn.MatchesPath(pathToMatch)
		}

		if depth < 2 || strings.Contains(relPath, "node_modules") || strings.HasSuffix(relPath, ".log") {
			fmt.Printf("Checking path: '%s' (original relPath: '%s'), IsDir: %v, Gitignored: %v, CustomIgnored: %v\n", pathToMatch, relPath, entryIsDir, isGitignored, isCustomIgnored)
		}
		var sizeBytes int64
		var tokenCount int64
		if entryIsDir {
			if isCustomIgnored {
				dirSize, dirTokens, statsErr := computeDirStats(tokenEncoder, nodePath, visited)
				if statsErr != nil {
					runtime.LogErrorf(logCtx, "Failed to compute stats for directory %s: %v", nodePath, statsErr)
					return nil, statsErr
				}
				sizeBytes = dirSize
				tokenCount = dirTokens
			}
		} else {
			content, readErr := os.ReadFile(nodePath)
			if readErr != nil {
				runtime.LogErrorf(logCtx, "Failed to read file %s: %v", nodePath, readErr)
				return nil, readErr
			}
			sizeBytes = int64(len(content))
			tokenCount = int64(tokenEncoder.Count(string(content)))
		}

		node := &FileNode{
			Name:            entry.Name(),
			Path:            nodePath,
			RelPath:         relPath,
			RootPath:        rootPath,
			RootLabel:       rootLabel,
			SizeBytes:       sizeBytes,
			TokensApprox:    tokenCount,
			IsDir:           entryIsDir,
			IsGitignored:    isGitignored,
			IsCustomIgnored: isCustomIgnored,
		}
		if entryIsDir {
			// If it's a directory, recursively call buildTree.
			// Recursion stops only for folders ignored by custom rules.
			// For folders matched by .gitignore, recursion continues so the UI can show their contents
			// and allow selective inclusion of files inside gitignored folders.
			if !isCustomIgnored {
				if visited[resolvedDirPath] {
					runtime.LogWarningf(logCtx, "Skipping directory %s (resolved to %s) to avoid symlink cycle", nodePath, resolvedDirPath)
				} else {
					visited[resolvedDirPath] = true
					children, err := buildTreeRecursive(ctx, logCtx, nodePath, rootPath, rootLabel, gitIgn, customIgn, tokenEncoder, visited, depth+1)
					delete(visited, resolvedDirPath)
					if err != nil {
						if errors.Is(err, context.Canceled) {
							return nil, err // Propagate cancellation
						}
						// runtime.LogWarnf(ctx, "Error building subtree for %s: %v", nodePath, err) // Use ctx if available
						runtime.LogWarningf(logCtx, "Error building subtree for %s: %v", nodePath, err) // Fallback for now
						// Decide: skip this dir or return error up. For now, skip with log.
					} else {
						node.Children = children
						node.SizeBytes = sumChildSizes(children)
						node.TokensApprox = sumChildTokens(children)
					}
				}
			}
		}
		nodes = append(nodes, node)
	}
	// Sort nodes: directories first, then files, then alphabetically
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].IsDir && !nodes[j].IsDir {
			return true
		}
		if !nodes[i].IsDir && nodes[j].IsDir {
			return false
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	return nodes, nil
}

// ContextGenerator manages the asynchronous generation of shotgun context
type ContextGenerator struct {
	app                *App // To access Wails runtime context for emitting events
	mu                 sync.Mutex
	currentCancelFunc  context.CancelFunc
	currentCancelToken interface{} // Token to identify the current cancel func
}

func NewContextGenerator(app *App) *ContextGenerator {
	return &ContextGenerator{app: app}
}

// RequestShotgunContextGeneration is called by the frontend to start/restart generation.
// This method itself is not bound to Wails directly if it's part of App.
// Instead, a wrapper method in App struct will be bound.
func (cg *ContextGenerator) requestShotgunContextGenerationInternal(rootDirs []string, excludedByRoot map[string][]string) {
	cg.mu.Lock()
	if cg.currentCancelFunc != nil {
		runtime.LogDebug(cg.app.ctx, "Cancelling previous context generation job.")
		cg.currentCancelFunc()
	}

	genCtx, cancel := context.WithCancel(cg.app.ctx)
	myToken := new(struct{}) // Create a unique token for this generation job
	cg.currentCancelFunc = cancel
	cg.currentCancelToken = myToken
	limit := cg.app.contextLimitBytes()
	runtime.LogInfof(cg.app.ctx, "Starting new shotgun context generation for %d roots. Max size: %d bytes.", len(rootDirs), limit)
	cg.mu.Unlock()

	go func(tokenForThisJob interface{}) {
		jobStartTime := time.Now()
		defer func() {
			cg.mu.Lock()
			if cg.currentCancelToken == tokenForThisJob { // Only clear if it's still this job's token
				cg.currentCancelFunc = nil
				cg.currentCancelToken = nil
				runtime.LogDebug(cg.app.ctx, "Cleared currentCancelFunc for completed/cancelled job (token match).")
			} else {
				runtime.LogDebug(cg.app.ctx, "currentCancelFunc was replaced by a newer job (token mismatch); not clearing.")
			}
			cg.mu.Unlock()
			runtime.LogInfof(cg.app.ctx, "Shotgun context generation goroutine finished in %s", time.Since(jobStartTime))
		}()

		if genCtx.Err() != nil { // Check for immediate cancellation
			runtime.LogInfo(cg.app.ctx, fmt.Sprintf("Context generation cancelled before starting: %v", genCtx.Err()))
			return
		}

		output, err := cg.app.generateShotgunOutputWithProgress(genCtx, rootDirs, excludedByRoot)
		var tokenCount int64
		if err == nil {
			tokenCount, err = cg.app.CountTokens(output)
			if err != nil {
				err = fmt.Errorf("failed to count tokens for context: %w", err)
			}
		}

		select {
		case <-genCtx.Done():
			errMsg := fmt.Sprintf("Shotgun context generation cancelled: %v", genCtx.Err())
			runtime.LogInfo(cg.app.ctx, errMsg) // Changed from LogWarn
			runtime.EventsEmit(cg.app.ctx, "shotgunContextError", errMsg)
		default:
			if err != nil {
				errMsg := fmt.Sprintf("Error generating shotgun output: %v", err)
				runtime.LogError(cg.app.ctx, errMsg)
				runtime.EventsEmit(cg.app.ctx, "shotgunContextError", errMsg)
			} else {
				finalSize := len(output)
				successMsg := fmt.Sprintf("Shotgun context generated successfully. Size: %d bytes. Tokens: %d.", finalSize, tokenCount)
				if int64(finalSize) > limit { // Should have been caught by ErrContextTooLong, but as a safeguard
					runtime.LogWarningf(cg.app.ctx, "Warning: Generated context size %d exceeds max %d, but was not caught by ErrContextTooLong.", finalSize, limit)
				}
				runtime.LogInfo(cg.app.ctx, successMsg)
				runtime.EventsEmit(cg.app.ctx, "shotgunContextGenerated", map[string]interface{}{
					"output": output,
					"tokens": tokenCount,
				})
			}
		}
	}(myToken) // Pass the token to the goroutine
}

// RequestShotgunContextGeneration is the method bound to Wails.
func (a *App) RequestShotgunContextGeneration(rootDirs []string, excludedByRoot map[string][]string) {
	if a.contextGenerator == nil {
		// This should not happen if startup initializes it correctly
		runtime.LogError(a.ctx, "ContextGenerator not initialized")
		runtime.EventsEmit(a.ctx, "shotgunContextError", "Internal error: ContextGenerator not initialized")
		return
	}
	a.contextGenerator.requestShotgunContextGenerationInternal(rootDirs, excludedByRoot)
}

func (a *App) RequestAutoContextSelection(rootDirs []string, excludedByRoot map[string][]string, userTask string) (map[string][]string, error) {
	if a.autoContextService == nil {
		return nil, errors.New("auto-context service is not initialized")
	}
	normalizedRoots, err := normalizeAndValidateRoots(rootDirs, true)
	if err != nil {
		return nil, err
	}
	if len(normalizedRoots) == 0 {
		return nil, errors.New("project root is required")
	}
	if !a.HasActiveLlmKey() {
		return nil, errors.New("no active LLM configuration found")
	}

	rootLabels := computeRootLabels(normalizedRoots)
	excludedMaps := make(map[string]map[string]bool, len(normalizedRoots))
	for _, rootDir := range normalizedRoots {
		excludedMaps[rootDir] = make(map[string]bool)
	}
	for rootDir, paths := range excludedByRoot {
		normalizedRoot, err := normalizeAndValidateRoots([]string{rootDir}, false)
		if err != nil {
			return nil, err
		}
		if len(normalizedRoot) == 0 {
			return nil, errors.New("project root is required")
		}
		rootKey := normalizedRoot[0]
		if _, ok := excludedMaps[rootKey]; !ok {
			return nil, fmt.Errorf("excluded paths provided for unknown root: %s", rootKey)
		}
		for _, p := range paths {
			normalized := normalizeRelativePath(p)
			if normalized == "" {
				continue
			}
			excludedMaps[rootKey][normalized] = true
		}
	}

	tree, err := buildAutoContextTreeForRoots(normalizedRoots, rootLabels, excludedMaps)
	if err != nil {
		a.emitAutoContextError(fmt.Sprintf("failed to build project tree: %v", err))
		return nil, err
	}

	task := strings.TrimSpace(userTask)
	// Build LLM prompt for auto-context selection
	prompt, err := a.autoContextService.BuildPrompt(tree, task, "")
	if err != nil {
		a.emitAutoContextError(fmt.Sprintf("failed to render auto-context prompt: %v", err))
		return nil, err
	}

	cfg := buildProviderConfig(a.settings.LLMSettings)
	providerInstance, err := a.getOrCreateProvider(cfg)
	if err != nil {
		a.emitAutoContextError(fmt.Sprintf("failed to configure provider: %v", err))
		return nil, err
	}

	// Execute LLM call
	raw, apiCall, err := providerInstance.Generate(a.ctx, prompt)

	// Log to shared prompt history for diagnostics (Step 3 view).
	if a.historyManager != nil {
		historyLabel := "AUTO CONTEXT"
		if task != "" {
			const maxLabelRunes = 80
			runes := []rune(task)
			if len(runes) > maxLabelRunes {
				historyLabel = "AUTO CONTEXT: " + string(runes[:maxLabelRunes]) + "…"
			} else {
				historyLabel = "AUTO CONTEXT: " + task
			}
		}

		responseForHistory := raw
		if err != nil {
			responseForHistory = fmt.Sprintf("ERROR during auto-context LLM call: %v", err)
		}
		a.historyManager.AddItem(historyLabel, prompt, responseForHistory, apiCall)
	}

	if err != nil {
		a.emitAutoContextError(fmt.Sprintf("provider error: %v", err))
		return nil, err
	}

	parsed, err := a.autoContextService.ParseResponse(raw)
	if err != nil {
		a.emitAutoContextError(fmt.Sprintf("failed to parse LLM response: %v", err))
		return nil, err
	}

	selected, err := resolveLLMSelectionForRoots(normalizedRoots, rootLabels, parsed.Files)
	if err != nil {
		a.emitAutoContextError(fmt.Sprintf("unable to match LLM selection to files: %v", err))
		return nil, err
	}

	selectedCount := 0
	for _, items := range selected {
		selectedCount += len(items)
	}

	runtime.LogInfof(a.ctx, "Auto-context selected %d files via %s (%s)", selectedCount, cfg.Provider, cfg.Model)
	return selected, nil
}

// countProcessableItems estimates the total number of operations for progress tracking.
// Operations: 1 for root dir line, 1 for each dir/file entry in tree, 1 for each file content read.
func (a *App) countProcessableItems(jobCtx context.Context, rootDir string, excludedMap map[string]bool) (int, error) {
	count := 1 // For the root directory line itself

	var counterHelper func(currentPath string) error
	counterHelper = func(currentPath string) error {
		select {
		case <-jobCtx.Done():
			return jobCtx.Err()
		default:
		}

		entries, err := os.ReadDir(currentPath)
		if err != nil {
			runtime.LogWarningf(a.ctx, "countProcessableItems: error reading dir %s: %v", currentPath, err)
			return nil // Continue counting other parts if a subdir is inaccessible
		}

		for _, entry := range entries {
			path := filepath.Join(currentPath, entry.Name())
			relPath, _ := filepath.Rel(rootDir, path)

			if excludedMap[relPath] {
				continue
			}

			count++ // For the tree entry (dir or file)

			if entry.IsDir() {
				err := counterHelper(path)
				if err != nil { // Propagate cancellation or critical errors
					return err
				}
			} else {
				count++ // For reading the file content
			}
		}
		return nil
	}

	err := counterHelper(rootDir)
	if err != nil {
		return 0, err // Return error if counting was interrupted (e.g. context cancelled)
	}
	return count, nil
}

type generationProgressState struct {
	processedItems int
	totalItems     int
}

func (a *App) emitProgress(state *generationProgressState) {
	runtime.EventsEmit(a.ctx, "shotgunContextGenerationProgress", map[string]int{
		"current": state.processedItems,
		"total":   state.totalItems,
	})
}

// generateShotgunOutputWithProgress generates the TXT output with progress reporting and size limits
func (a *App) generateShotgunOutputWithProgress(jobCtx context.Context, rootDirs []string, excludedByRoot map[string][]string) (string, error) {
	if err := jobCtx.Err(); err != nil { // Check for cancellation at the beginning
		return "", err
	}

	normalizedRoots, err := normalizeAndValidateRoots(rootDirs, true)
	if err != nil {
		return "", err
	}
	if len(normalizedRoots) == 0 {
		return "", nil
	}
	limit := a.contextLimitBytes()

	rootLabels := computeRootLabels(normalizedRoots)
	excludedMaps := make(map[string]map[string]bool, len(normalizedRoots))
	for _, rootDir := range normalizedRoots {
		excludedMaps[rootDir] = make(map[string]bool)
	}
	for rootDir, paths := range excludedByRoot {
		normalizedRoot, err := normalizeAndValidateRoots([]string{rootDir}, false)
		if err != nil {
			return "", err
		}
		if len(normalizedRoot) == 0 {
			return "", errors.New("project root is required")
		}
		rootKey := normalizedRoot[0]
		if _, ok := excludedMaps[rootKey]; !ok {
			return "", fmt.Errorf("excluded paths provided for unknown root: %s", rootKey)
		}
		for _, p := range paths {
			if p == "" {
				continue
			}
			excludedMaps[rootKey][p] = true
		}
	}

	totalItems := 0
	for _, rootDir := range normalizedRoots {
		count, err := a.countProcessableItems(jobCtx, rootDir, excludedMaps[rootDir])
		if err != nil {
			return "", fmt.Errorf("failed to count processable items: %w", err)
		}
		totalItems += count
	}
	progressState := &generationProgressState{processedItems: 0, totalItems: totalItems}
	a.emitProgress(progressState) // Initial progress (0 / total)

	rootOrder := make([]string, len(normalizedRoots))
	copy(rootOrder, normalizedRoots)
	sort.SliceStable(rootOrder, func(i, j int) bool {
		return rootLabels[rootOrder[i]] < rootLabels[rootOrder[j]]
	})

	var output strings.Builder
	var fileContents strings.Builder

	var buildShotgunTreeRecursive func(pCtx context.Context, rootDir, rootLabel, currentPath, prefix string, excludedMap map[string]bool) error
	buildShotgunTreeRecursive = func(pCtx context.Context, rootDir, rootLabel, currentPath, prefix string, excludedMap map[string]bool) error {
		select {
		case <-pCtx.Done():
			return pCtx.Err()
		default:
		}

		entries, err := os.ReadDir(currentPath)
		if err != nil {
			runtime.LogWarningf(a.ctx, "buildShotgunTreeRecursive: error reading dir %s: %v", currentPath, err)
			// Decide if this error should halt the entire process or just skip this directory
			// For now, returning nil to skip, but log it. Could also return the error.
			return nil // Or return err if this should stop everything
		}

		// Sort entries like in ListFiles for consistent tree
		sort.SliceStable(entries, func(i, j int) bool {
			entryI := entries[i]
			entryJ := entries[j]
			isDirI := entryI.IsDir()
			isDirJ := entryJ.IsDir()
			if isDirI && !isDirJ {
				return true
			}
			if !isDirI && isDirJ {
				return false
			}
			return strings.ToLower(entryI.Name()) < strings.ToLower(entryJ.Name())
		})

		// Create a temporary slice to hold non-excluded entries for correct prefixing
		var visibleEntries []fs.DirEntry
		for _, entry := range entries {
			path := filepath.Join(currentPath, entry.Name())
			relPath, _ := filepath.Rel(rootDir, path)
			if !excludedMap[relPath] {
				visibleEntries = append(visibleEntries, entry)
			}
		}

		for i, entry := range visibleEntries {
			select {
			case <-pCtx.Done():
				return pCtx.Err()
			default:
			}

			path := filepath.Join(currentPath, entry.Name())
			relPath, _ := filepath.Rel(rootDir, path)

			isLast := i == len(visibleEntries)-1

			branch := "├── "
			nextPrefix := prefix + "│   "
			if isLast {
				branch = "└── "
				nextPrefix = prefix + "    "
			}
			output.WriteString(prefix + branch + entry.Name() + "\n")

			progressState.processedItems++ // For tree entry
			a.emitProgress(progressState)

			if int64(output.Len()+fileContents.Len()) > limit {
				return fmt.Errorf("%w: content limit of %d bytes exceeded during tree generation (size: %d bytes)", ErrContextTooLong, limit, output.Len()+fileContents.Len())
			}

			if entry.IsDir() {
				err := buildShotgunTreeRecursive(pCtx, rootDir, rootLabel, path, nextPrefix, excludedMap)
				if err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						return err
					}
					fmt.Printf("Error processing subdirectory %s: %v\n", path, err)
				}
			} else {
				select { // Check before heavy I/O
				case <-pCtx.Done():
					return pCtx.Err()
				default:
				}
				content, err := os.ReadFile(path)
				if err != nil {
					fmt.Printf("Error reading file %s: %v\n", path, err)
					content = []byte(fmt.Sprintf("Error reading file: %v", err))
				}

				relPathForwardSlash := filepath.ToSlash(relPath)
				filePath := relPathForwardSlash
				if rootLabel != "" {
					filePath = rootLabel + "/" + relPathForwardSlash
				}

				fileContents.WriteString(fmt.Sprintf("<file path=\"%s\">\n", filePath))
				fileContents.WriteString(string(content))
				fileContents.WriteString("\n</file>\n") // Each file block ends with a newline

				progressState.processedItems++ // For file content
				a.emitProgress(progressState)

				if int64(output.Len()+fileContents.Len()) > limit { // Final check after append
					return fmt.Errorf("%w: content limit of %d bytes exceeded after appending file %s (total size: %d bytes)", ErrContextTooLong, limit, relPath, output.Len()+fileContents.Len())
				}
			}
		}
		return nil
	}

	for _, rootDir := range rootOrder {
		if err := jobCtx.Err(); err != nil {
			return "", err
		}
		rootLabel := rootLabels[rootDir]
		output.WriteString(rootLabel + string(os.PathSeparator) + "\n")
		progressState.processedItems++
		a.emitProgress(progressState)
		if int64(output.Len()+fileContents.Len()) > limit {
			return "", fmt.Errorf("%w: content limit of %d bytes exceeded after root dir line (size: %d bytes)", ErrContextTooLong, limit, output.Len()+fileContents.Len())
		}

		err := buildShotgunTreeRecursive(jobCtx, rootDir, rootLabel, rootDir, "", excludedMaps[rootDir])
		if err != nil {
			return "", fmt.Errorf("failed to build tree for shotgun: %w", err)
		}
	}

	if err := jobCtx.Err(); err != nil { // Check for cancellation before final string operations
		return "", err
	}

	// The final output is the tree, a newline, then all concatenated file contents.
	// If fileContents is empty, we still want the newline after the tree.
	// If fileContents is not empty, it already ends with a newline, so an extra one might not be desired
	// depending on how it's structured. Given each <file> block ends with \n, this should be fine.
	return output.String() + "\n" + strings.TrimRight(fileContents.String(), "\n"), nil
}

// --- Watchman Implementation ---

func copyGitignoreMap(src map[string]*gitignore.GitIgnore) map[string]*gitignore.GitIgnore {
	if src == nil {
		return nil
	}
	dst := make(map[string]*gitignore.GitIgnore, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func isRelativeWithinRoot(rel string) bool {
	if rel == "" || rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func findRootForPath(path string, roots []string) (string, string) {
	bestLen := -1
	bestRoot := ""
	bestRel := ""
	for _, root := range roots {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}
		if !isRelativeWithinRoot(rel) {
			continue
		}
		if len(root) > bestLen {
			bestLen = len(root)
			bestRoot = root
			bestRel = rel
		}
	}
	return bestRoot, bestRel
}

type Watchman struct {
	app         *App
	rootDirs    []string
	fsWatcher   *fsnotify.Watcher
	watchedDirs map[string]bool // Tracks directories explicitly added to fsnotify

	// lastKnownState map[string]fileMeta // Removed, fsnotify handles state
	mu         sync.Mutex // Changed to Mutex for simplicity with Start/Stop/Refresh
	cancelFunc context.CancelFunc

	// Store current patterns to be used by scanDirectoryStateInternal
	currentProjectGitignores map[string]*gitignore.GitIgnore
	currentCustomPatterns    *gitignore.GitIgnore
}

func NewWatchman(app *App) *Watchman {
	return &Watchman{
		app:         app,
		watchedDirs: make(map[string]bool),
	}
}

// StartFileWatcher is called by JavaScript to start watching directories.
func (a *App) StartFileWatcher(rootDirs []string) error {
	runtime.LogInfof(a.ctx, "StartFileWatcher called for %d roots.", len(rootDirs))
	if a.fileWatcher == nil {
		return fmt.Errorf("file watcher not initialized")
	}
	return a.fileWatcher.Start(rootDirs)
}

// StopFileWatcher is called by JavaScript to stop the current watcher.
func (a *App) StopFileWatcher() error {
	runtime.LogInfo(a.ctx, "StopFileWatcher called")
	if a.fileWatcher == nil {
		return fmt.Errorf("file watcher not initialized")
	}
	a.fileWatcher.Stop()
	return nil
}

func (w *Watchman) Start(newRootDirs []string) error {
	w.Stop() // Stop any existing watcher

	normalizedRoots, err := normalizeAndValidateRoots(newRootDirs, true)
	if err != nil {
		return err
	}
	if len(normalizedRoots) == 0 {
		runtime.LogInfo(w.app.ctx, "Watchman: Root directory is empty, not starting.")
		return nil
	}

	w.mu.Lock()
	w.rootDirs = normalizedRoots
	if w.app.useGitignore {
		w.currentProjectGitignores = copyGitignoreMap(w.app.projectGitignores)
	} else {
		w.currentProjectGitignores = nil
	}
	if w.app.useCustomIgnore {
		w.currentCustomPatterns = w.app.currentCustomIgnorePatterns
	} else {
		w.currentCustomPatterns = nil
	}
	ctx, cancel := context.WithCancel(w.app.ctx) // Use app's context as parent
	w.cancelFunc = cancel
	w.mu.Unlock()

	w.fsWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		runtime.LogErrorf(w.app.ctx, "Watchman: Error creating fsnotify watcher: %v", err)
		return fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}
	w.watchedDirs = make(map[string]bool) // Initialize/clear

	runtime.LogInfof(w.app.ctx, "Watchman: Starting for %d roots", len(normalizedRoots))
	for _, rootDir := range normalizedRoots {
		w.addPathsToWatcherRecursive(rootDir, rootDir) // Add initial paths
	}

	go w.run(ctx)
	return nil
}

func (w *Watchman) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancelFunc != nil {
		runtime.LogInfo(w.app.ctx, "Watchman: Stopping...")
		w.cancelFunc()
		w.cancelFunc = nil // Allow GC and prevent double-cancel
	}
	if w.fsWatcher != nil {
		err := w.fsWatcher.Close()
		if err != nil {
			runtime.LogWarningf(w.app.ctx, "Watchman: Error closing fsnotify watcher: %v", err)
		}
		w.fsWatcher = nil
	}
	w.rootDirs = nil
	w.watchedDirs = make(map[string]bool) // Clear watched directories
}

func (w *Watchman) run(ctx context.Context) {
	defer func() {
		if w.fsWatcher != nil {
			// This close is a safeguard; Stop() should ideally be called.
			w.fsWatcher.Close()
		}
		runtime.LogInfo(w.app.ctx, "Watchman: Goroutine stopped.")
	}()

	w.mu.Lock()
	currentRootDirs := append([]string{}, w.rootDirs...)
	w.mu.Unlock()
	runtime.LogInfof(w.app.ctx, "Watchman: Monitoring goroutine started for %d roots", len(currentRootDirs))

	for {
		select {
		case <-ctx.Done():
			w.mu.Lock()
			shutdownRoots := append([]string{}, w.rootDirs...)
			w.mu.Unlock()
			runtime.LogInfof(w.app.ctx, "Watchman: Context cancelled, shutting down watcher for %d roots.", len(shutdownRoots))
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				runtime.LogInfo(w.app.ctx, "Watchman: fsnotify events channel closed.")
				return
			}
			runtime.LogDebugf(w.app.ctx, "Watchman: fsnotify event: %s", event)

			w.mu.Lock()
			currentRootDirs = append([]string{}, w.rootDirs...)
			projIgns := w.currentProjectGitignores
			custIgn := w.currentCustomPatterns
			w.mu.Unlock()

			if len(currentRootDirs) == 0 { // Watcher might have been stopped
				continue
			}

			rootDir, relEventPath := findRootForPath(event.Name, currentRootDirs)
			if rootDir == "" {
				continue
			}

			var projIgn *gitignore.GitIgnore
			if projIgns != nil {
				projIgn = projIgns[rootDir]
			}

			// Check if the event path is ignored
			isIgnoredByGit := projIgn != nil && projIgn.MatchesPath(relEventPath)
			isIgnoredByCustom := custIgn != nil && custIgn.MatchesPath(relEventPath)

			if isIgnoredByGit || isIgnoredByCustom {
				runtime.LogDebugf(w.app.ctx, "Watchman: Ignoring event for %s as it's an ignored path.", event.Name)
				continue
			}

			// Handle relevant events (excluding Chmod)
			if event.Op&fsnotify.Chmod == 0 {
				runtime.LogInfof(w.app.ctx, "Watchman: Relevant change detected for %s in %s", event.Name, rootDir)
				w.app.notifyFileChange(rootDir)
			}

			// Dynamic directory watching
			if event.Op&fsnotify.Create != 0 {
				info, statErr := os.Stat(event.Name)
				if statErr == nil && info.IsDir() {
					// Check if this new directory itself is ignored before adding
					isNewDirIgnoredByGit := projIgn != nil && projIgn.MatchesPath(relEventPath)
					isNewDirIgnoredByCustom := custIgn != nil && custIgn.MatchesPath(relEventPath)
					if !isNewDirIgnoredByGit && !isNewDirIgnoredByCustom {
						runtime.LogDebugf(w.app.ctx, "Watchman: New directory created %s, adding to watcher.", event.Name)
						w.addPathsToWatcherRecursive(rootDir, event.Name) // This will add event.Name and its children
					} else {
						runtime.LogDebugf(w.app.ctx, "Watchman: New directory %s is ignored, not adding to watcher.", event.Name)
					}
				}
			}

			if event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0 {
				w.mu.Lock()
				if w.watchedDirs[event.Name] {
					runtime.LogDebugf(w.app.ctx, "Watchman: Watched directory %s removed/renamed, removing from watcher.", event.Name)
					// fsnotify might remove it automatically, but explicit removal is safer for our tracking
					if w.fsWatcher != nil { // Check fsWatcher as it might be closed by Stop()
						err := w.fsWatcher.Remove(event.Name)
						if err != nil {
							runtime.LogWarningf(w.app.ctx, "Watchman: Error removing path %s from fsnotify: %v", event.Name, err)
						}
					}
					delete(w.watchedDirs, event.Name)
				}
				w.mu.Unlock()
			}

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				runtime.LogInfo(w.app.ctx, "Watchman: fsnotify errors channel closed.")
				return
			}
			runtime.LogErrorf(w.app.ctx, "Watchman: fsnotify error: %v", err)
		}
	}
}

func (w *Watchman) addPathsToWatcherRecursive(rootDir, baseDirToAdd string) {
	w.mu.Lock() // Lock to access watcher and ignore patterns
	fsW := w.fsWatcher
	projIgns := w.currentProjectGitignores
	custIgn := w.currentCustomPatterns
	w.mu.Unlock()

	if fsW == nil || rootDir == "" {
		runtime.LogWarningf(w.app.ctx, "Watchman.addPathsToWatcherRecursive: fsWatcher is nil or rootDir is empty. Skipping add for %s.", baseDirToAdd)
		return
	}

	var projIgn *gitignore.GitIgnore
	if projIgns != nil {
		projIgn = projIgns[rootDir]
	}

	filepath.WalkDir(baseDirToAdd, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			runtime.LogWarningf(w.app.ctx, "Watchman scan error accessing %s: %v", path, walkErr)
			if d != nil && d.IsDir() && path != rootDir { // Changed scanRootDir to rootDir for clarity
				return filepath.SkipDir
			}
			return nil // Try to continue
		}

		if !d.IsDir() {
			return nil
		}

		relPath, errRel := filepath.Rel(rootDir, path)
		if errRel != nil {
			runtime.LogWarningf(w.app.ctx, "Watchman.addPathsToWatcherRecursive: Could not get relative path for %s (root: %s): %v", path, rootDir, errRel)
			return nil // Continue with other paths
		}

		// Skip .git directory at the top level of rootDir
		if d.IsDir() && d.Name() == ".git" {
			parentDir := filepath.Dir(path)
			if parentDir == rootDir {
				runtime.LogDebugf(w.app.ctx, "Watchman.addPathsToWatcherRecursive: Skipping .git directory: %s", path)
				return filepath.SkipDir
			}
		}

		isIgnoredByGit := projIgn != nil && projIgn.MatchesPath(relPath)
		isIgnoredByCustom := custIgn != nil && custIgn.MatchesPath(relPath)

		if isIgnoredByGit || isIgnoredByCustom {
			runtime.LogDebugf(w.app.ctx, "Watchman.addPathsToWatcherRecursive: Skipping ignored directory: %s", path)
			return filepath.SkipDir
		}

		errAdd := fsW.Add(path)
		if errAdd != nil {
			runtime.LogWarningf(w.app.ctx, "Watchman.addPathsToWatcherRecursive: Error adding path %s to fsnotify: %v", path, errAdd)
		} else {
			runtime.LogDebugf(w.app.ctx, "Watchman.addPathsToWatcherRecursive: Added to watcher: %s", path)
			w.mu.Lock()
			w.watchedDirs[path] = true
			w.mu.Unlock()
		}
		return nil
	})
}

// notifyFileChange is an internal method for the App to emit a Wails event.
func (a *App) notifyFileChange(rootDir string) {
	runtime.EventsEmit(a.ctx, "projectFilesChanged", rootDir)
}

// RefreshIgnoresAndRescan is called when ignore settings change in the App.
func (w *Watchman) RefreshIgnoresAndRescan() error {
	w.mu.Lock()
	if len(w.rootDirs) == 0 {
		w.mu.Unlock()
		runtime.LogInfo(w.app.ctx, "Watchman.RefreshIgnoresAndRescan: No rootDirs, skipping.")
		return nil
	}
	runtime.LogInfo(w.app.ctx, "Watchman.RefreshIgnoresAndRescan: Refreshing ignore patterns and re-scanning.")

	// Update patterns based on App's current state
	if w.app.useGitignore {
		w.currentProjectGitignores = copyGitignoreMap(w.app.projectGitignores)
	} else {
		w.currentProjectGitignores = nil
	}
	if w.app.useCustomIgnore {
		w.currentCustomPatterns = w.app.currentCustomIgnorePatterns
	} else {
		w.currentCustomPatterns = nil
	}
	currentRoots := append([]string{}, w.rootDirs...)
	defer w.mu.Unlock()

	// Stop existing watcher (closes, clears watchedDirs)
	if w.cancelFunc != nil {
		w.cancelFunc()
	}
	if w.fsWatcher != nil {
		w.fsWatcher.Close()
	}
	w.watchedDirs = make(map[string]bool)

	// Create new watcher
	var err error
	w.fsWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		runtime.LogErrorf(w.app.ctx, "Watchman.RefreshIgnoresAndRescan: Error creating new fsnotify watcher: %v", err)
		return fmt.Errorf("failed to create new fsnotify watcher: %w", err)
	}

	for _, rootDir := range currentRoots {
		w.addPathsToWatcherRecursive(rootDir, rootDir) // Add paths with new rules
		w.app.notifyFileChange(rootDir)                // Notify frontend to refresh its view
	}

	return nil
}

// --- Configuration Management ---

func (a *App) compileCustomIgnorePatterns() error {
	if strings.TrimSpace(a.settings.CustomIgnoreRules) == "" {
		a.currentCustomIgnorePatterns = nil
		runtime.LogDebug(a.ctx, "Custom ignore rules are empty, no patterns compiled.")
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(a.settings.CustomIgnoreRules, "\r\n", "\n"), "\n")
	var validLines []string
	for _, line := range lines {
		// CompileIgnoreLines should handle empty/comment lines appropriately based on .gitignore syntax
		validLines = append(validLines, line)
	}

	ign := gitignore.CompileIgnoreLines(validLines...)
	// Поскольку CompileIgnoreLines в этой версии не возвращает ошибку,
	// проверка на err удалена.
	// Если ign будет nil (например, если все строки были пустыми или комментариями,
	// и библиотека так обрабатывает), то это будет корректно обработано ниже.
	a.currentCustomIgnorePatterns = ign
	runtime.LogInfo(a.ctx, "Successfully compiled custom ignore patterns.")
	return nil
}

func (a *App) loadSettings() {
	// Default to embedded rules
	a.settings.CustomIgnoreRules = defaultCustomIgnoreRulesContent
	shouldSaveSettings := false

	if a.configPath == "" {
		runtime.LogWarningf(a.ctx, "Config path is empty, using default custom ignore rules (embedded).")
		if err := a.compileCustomIgnorePatterns(); err != nil {
			// Error already logged in compileCustomIgnorePatterns
		}
		return
	}

	data, err := os.ReadFile(a.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			runtime.LogInfo(a.ctx, "Settings file not found. Using default custom ignore rules (embedded).")
			shouldSaveSettings = true
		} else {
			runtime.LogErrorf(a.ctx, "Error reading settings file %s: %v. Using default custom ignore rules (embedded).", a.configPath, err)
		}
	} else {
		err = json.Unmarshal(data, &a.settings)
		if err != nil {
			runtime.LogErrorf(a.ctx, "Error unmarshalling settings from %s: %v. Using default custom ignore rules (embedded).", a.configPath, err)
			a.settings.CustomIgnoreRules = defaultCustomIgnoreRulesContent // Reset to default on unmarshal error
		} else {
			runtime.LogInfo(a.ctx, "Successfully loaded custom ignore rules from config.")
			// If loaded rules are empty but default embedded rules are not, use default.
			if strings.TrimSpace(a.settings.CustomIgnoreRules) == "" && strings.TrimSpace(defaultCustomIgnoreRulesContent) != "" {
				runtime.LogInfo(a.ctx, "Loaded custom ignore rules are empty, falling back to default embedded rules.")
				a.settings.CustomIgnoreRules = defaultCustomIgnoreRulesContent
			}
			// Handle CustomPromptRules similarly
			if strings.TrimSpace(a.settings.CustomPromptRules) == "" {
				runtime.LogInfo(a.ctx, "Custom prompt rules are empty or missing, using default.")
				a.settings.CustomPromptRules = defaultCustomPromptRulesContent
				shouldSaveSettings = true
			} else if strings.TrimSpace(a.settings.CustomPromptRules) == legacyDefaultCustomPromptRulesContent {
				runtime.LogInfo(a.ctx, "Custom prompt rules match legacy default, updating to new default.")
				a.settings.CustomPromptRules = defaultCustomPromptRulesContent
				shouldSaveSettings = true
			}
		}
	}
	if a.settings.MaxContextBytes <= 0 {
		runtime.LogWarningf(a.ctx, "MaxContextBytes is missing or invalid, defaulting to %d.", defaultMaxOutputSizeBytes)
		a.settings.MaxContextBytes = defaultMaxOutputSizeBytes
		shouldSaveSettings = true
	}

	a.ensureLLMSettingsDefaults()

	if errCompile := a.compileCustomIgnorePatterns(); errCompile != nil {
		// Error already logged in compileCustomIgnorePatterns
	}

	if shouldSaveSettings {
		if err := a.saveSettings(); err != nil {
			runtime.LogErrorf(a.ctx, "Failed to save settings: %v", err)
		}
	}
}

func (a *App) saveSettings() error {
	if a.configPath == "" {
		err := errors.New("config path is not set, cannot save settings")
		runtime.LogError(a.ctx, err.Error())
		return err
	}

	data, err := json.MarshalIndent(a.settings, "", "  ")
	if err != nil {
		runtime.LogErrorf(a.ctx, "Error marshalling settings: %v", err)
		return err
	}

	configDir := filepath.Dir(a.configPath)
	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		runtime.LogErrorf(a.ctx, "Error creating config directory %s: %v", configDir, err)
		return err
	}

	err = os.WriteFile(a.configPath, data, 0644)
	if err != nil {
		runtime.LogErrorf(a.ctx, "Error writing settings to %s: %v", a.configPath, err)
		return err
	}
	runtime.LogInfo(a.ctx, "Settings saved successfully.")
	return nil
}

// GetCustomIgnoreRules returns the current custom ignore rules as a string.
func (a *App) GetCustomIgnoreRules() string {
	// Ensure settings are loaded if they haven't been (e.g. if called before startup completes, though unlikely)
	// However, loadSettings is called in startup, so this should generally be populated.
	return a.settings.CustomIgnoreRules
}

// SetCustomIgnoreRules updates the custom ignore rules, saves them, and recompiles.
func (a *App) SetCustomIgnoreRules(rules string) error {
	a.settings.CustomIgnoreRules = rules
	// Attempt to compile first. If compilation fails, we might not want to save invalid rules,
	// or save them and let the user know they are not effective.
	// For now, compile then save. If compile fails, the old patterns (or nil) remain active.
	compileErr := a.compileCustomIgnorePatterns()

	saveErr := a.saveSettings()
	if saveErr != nil {
		return fmt.Errorf("failed to save settings: %w (compile error: %v)", saveErr, compileErr)
	}
	if compileErr != nil {
		return fmt.Errorf("rules saved, but failed to compile custom ignore patterns: %w", compileErr)
	}

	if a.fileWatcher != nil {
		return a.fileWatcher.RefreshIgnoresAndRescan()
	}
	return nil
}

// GetCustomPromptRules returns the current custom prompt rules as a string.
func (a *App) GetCustomPromptRules() string {
	if strings.TrimSpace(a.settings.CustomPromptRules) == "" {
		return defaultCustomPromptRulesContent
	}
	return a.settings.CustomPromptRules
}

// SetCustomPromptRules updates the custom prompt rules and saves them.
func (a *App) SetCustomPromptRules(rules string) error {
	a.settings.CustomPromptRules = rules
	err := a.saveSettings()
	if err != nil {
		return fmt.Errorf("failed to save custom prompt rules: %w", err)
	}
	runtime.LogInfo(a.ctx, "Custom prompt rules saved successfully.")
	return nil
}

// SetUseGitignore updates the app's setting for using .gitignore and informs the watcher.
func (a *App) SetUseGitignore(enabled bool) error {
	a.useGitignore = enabled
	runtime.LogInfof(a.ctx, "App setting useGitignore changed to: %v", enabled)
	if a.fileWatcher != nil {
		return a.fileWatcher.RefreshIgnoresAndRescan()
	}
	return nil
}

// SetUseCustomIgnore updates the app's setting for using custom ignore rules and informs the watcher.
func (a *App) SetUseCustomIgnore(enabled bool) error {
	a.useCustomIgnore = enabled
	runtime.LogInfof(a.ctx, "App setting useCustomIgnore changed to: %v", enabled)
	if a.fileWatcher != nil {
		return a.fileWatcher.RefreshIgnoresAndRescan()
	}
	return nil
}

func (a *App) emitAutoContextError(message string) {
	runtime.LogError(a.ctx, message)
	runtime.EventsEmit(a.ctx, "autoContextError", message)
}

// SaveRepoScan saves the repo scan content to shotgun_reposcan.md in the root directory
func (a *App) SaveRepoScan(rootDir string, content string) error {
	if rootDir == "" {
		return errors.New("root directory is empty")
	}
	filePath := filepath.Join(rootDir, "shotgun_reposcan.md")
	return os.WriteFile(filePath, []byte(content), 0644)
}

// LoadRepoScan loads the repo scan content from shotgun_reposcan.md in the root directory
func (a *App) LoadRepoScan(rootDir string) (string, error) {
	if rootDir == "" {
		return "", errors.New("root directory is empty")
	}
	filePath := filepath.Join(rootDir, "shotgun_reposcan.md")
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Return empty string if file doesn't exist
		}
		return "", err
	}
	return string(content), nil
}
