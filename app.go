package main

import (
	"context"
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
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxOutputSizeBytes = 10_000_000 // 10MB
var ErrContextTooLong = errors.New("context is too long")

//go:embed ignore.glob
var defaultCustomIgnoreRulesContent string

const defaultCustomPromptRulesContent = "no additional rules"

type AppSettings struct {
	CustomIgnoreRules string `json:"customIgnoreRules"`
	CustomPromptRules string `json:"customPromptRules"`
}

type Project struct {
	ID            string               `json:"id"`            // Unique identifier for the project
	Name          string               `json:"name"`          // Display name (usually the folder name)
	RootPath      string               `json:"rootPath"`      // Full path to the project directory
	Gitignore     *gitignore.GitIgnore `json:"-"`             // Compiled .gitignore for this project (not serialized)
	FileTree      []*FileNode          `json:"fileTree"`      // Cached file tree for this project
	ExcludedPaths map[string]bool      `json:"excludedPaths"` // User-excluded paths for this project
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
	projects                    map[string]*Project  // Map of project ID to Project
	projectOrder                []string             // Ordered list of project IDs for consistent display
	projectGitignore            *gitignore.GitIgnore // Deprecated: kept for compatibility, will be removed
}

func NewApp() *App {
	return &App{
		projects:     make(map[string]*Project),
		projectOrder: make([]string, 0),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.contextGenerator = NewContextGenerator(a)
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
	// Ensure CustomPromptRules has a default if it's empty after loading
	if strings.TrimSpace(a.settings.CustomPromptRules) == "" {
		a.settings.CustomPromptRules = defaultCustomPromptRulesContent
	}
}

type FileNode struct {
	Name            string      `json:"name"`
	Path            string      `json:"path"`    // Full path
	RelPath         string      `json:"relPath"` // Path relative to selected root
	IsDir           bool        `json:"isDir"`
	Children        []*FileNode `json:"children,omitempty"`
	IsGitignored    bool        `json:"isGitignored"`    // True if path matches a .gitignore rule
	IsCustomIgnored bool        `json:"isCustomIgnored"` // True if path matches a ignore.glob rule
	ProjectID       string      `json:"projectId"`       // ID of the project this node belongs to
}

// SelectDirectory opens a dialog to select a directory and returns the chosen path
func (a *App) SelectDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{})
}

// ListFiles lists files and folders in a directory, parsing .gitignore if present
func (a *App) ListFiles(dirPath string) ([]*FileNode, error) {
	runtime.LogDebugf(a.ctx, "ListFiles called for directory: %s", dirPath)

	a.projectGitignore = nil        // Reset for the new directory
	var gitIgn *gitignore.GitIgnore // For .gitignore in the project directory
	gitignorePath := filepath.Join(dirPath, ".gitignore")
	runtime.LogDebugf(a.ctx, "Attempting to find .gitignore at: %s", gitignorePath)
	if _, err := os.Stat(gitignorePath); err == nil {
		runtime.LogDebugf(a.ctx, ".gitignore found at: %s", gitignorePath)
		gitIgn, err = gitignore.CompileIgnoreFile(gitignorePath)
		if err != nil {
			runtime.LogWarningf(a.ctx, "Error compiling .gitignore file at %s: %v", gitignorePath, err)
			gitIgn = nil
		} else {
			a.projectGitignore = gitIgn // Store the compiled project-specific gitignore
			runtime.LogDebug(a.ctx, ".gitignore compiled successfully.")
		}
	} else {
		runtime.LogDebugf(a.ctx, ".gitignore not found at %s (os.Stat error: %v)", gitignorePath, err)
		gitIgn = nil
	}

	// App-level custom ignore patterns are in a.currentCustomIgnorePatterns

	rootNode := &FileNode{
		Name:         filepath.Base(dirPath),
		Path:         dirPath,
		RelPath:      ".",
		IsDir:        true,
		IsGitignored: false, // Root itself is not gitignored by default
		// IsCustomIgnored for root is also false by default, specific patterns would be needed
		IsCustomIgnored: a.currentCustomIgnorePatterns != nil && a.currentCustomIgnorePatterns.MatchesPath("."),
	}

	children, err := buildTreeRecursive(context.TODO(), dirPath, dirPath, gitIgn, a.currentCustomIgnorePatterns, 0)
	if err != nil {
		return []*FileNode{rootNode}, fmt.Errorf("error building children tree for %s: %w", dirPath, err)
	}
	rootNode.Children = children

	return []*FileNode{rootNode}, nil
}

func buildTreeRecursive(ctx context.Context, currentPath, rootPath string, gitIgn *gitignore.GitIgnore, customIgn *gitignore.GitIgnore, depth int) ([]*FileNode, error) {
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
		pathToMatch := relPath
		if entry.IsDir() {
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
			fmt.Printf("Checking path: '%s' (original relPath: '%s'), IsDir: %v, Gitignored: %v, CustomIgnored: %v\n", pathToMatch, relPath, entry.IsDir(), isGitignored, isCustomIgnored)
		}

		node := &FileNode{
			Name:            entry.Name(),
			Path:            nodePath,
			RelPath:         relPath,
			IsDir:           entry.IsDir(),
			IsGitignored:    isGitignored,
			IsCustomIgnored: isCustomIgnored,
		}

		if entry.IsDir() {
			// If it's a directory, recursively call buildTree
			// Only recurse if not ignored
			if !isGitignored && !isCustomIgnored {
				children, err := buildTreeRecursive(ctx, nodePath, rootPath, gitIgn, customIgn, depth+1)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return nil, err // Propagate cancellation
					}
					// runtime.LogWarnf(ctx, "Error building subtree for %s: %v", nodePath, err) // Use ctx if available
					runtime.LogWarningf(context.Background(), "Error building subtree for %s: %v", nodePath, err) // Fallback for now
					// Decide: skip this dir or return error up. For now, skip with log.
				} else {
					node.Children = children
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
func (cg *ContextGenerator) requestShotgunContextGenerationInternal(rootDir string, excludedPaths []string) {
	cg.mu.Lock()
	if cg.currentCancelFunc != nil {
		runtime.LogDebug(cg.app.ctx, "Cancelling previous context generation job.")
		cg.currentCancelFunc()
	}

	genCtx, cancel := context.WithCancel(cg.app.ctx)
	myToken := new(struct{}) // Create a unique token for this generation job
	cg.currentCancelFunc = cancel
	cg.currentCancelToken = myToken
	runtime.LogInfof(cg.app.ctx, "Starting new shotgun context generation for: %s. Max size: %d bytes.", rootDir, maxOutputSizeBytes)
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
			runtime.LogInfo(cg.app.ctx, fmt.Sprintf("Context generation for %s cancelled before starting: %v", rootDir, genCtx.Err()))
			return
		}

		output, err := cg.app.generateShotgunOutputWithProgress(genCtx, rootDir, excludedPaths)

		select {
		case <-genCtx.Done():
			errMsg := fmt.Sprintf("Shotgun context generation cancelled for %s: %v", rootDir, genCtx.Err())
			runtime.LogInfo(cg.app.ctx, errMsg) // Changed from LogWarn
			runtime.EventsEmit(cg.app.ctx, "shotgunContextError", errMsg)
		default:
			if err != nil {
				errMsg := fmt.Sprintf("Error generating shotgun output for %s: %v", rootDir, err)
				runtime.LogError(cg.app.ctx, errMsg)
				runtime.EventsEmit(cg.app.ctx, "shotgunContextError", errMsg)
			} else {
				finalSize := len(output)
				successMsg := fmt.Sprintf("Shotgun context generated successfully for %s. Size: %d bytes.", rootDir, finalSize)
				if finalSize > maxOutputSizeBytes { // Should have been caught by ErrContextTooLong, but as a safeguard
					runtime.LogWarningf(cg.app.ctx, "Warning: Generated context size %d exceeds max %d, but was not caught by ErrContextTooLong.", finalSize, maxOutputSizeBytes)
				}
				runtime.LogInfo(cg.app.ctx, successMsg)
				runtime.EventsEmit(cg.app.ctx, "shotgunContextGenerated", output)
			}
		}
	}(myToken) // Pass the token to the goroutine
}

// RequestShotgunContextGeneration is the method bound to Wails.
// Updated to support multiple project paths
func (a *App) RequestShotgunContextGeneration(projectPaths []string, excludedPaths []string) {
	if a.contextGenerator == nil {
		// This should not happen if startup initializes it correctly
		runtime.LogError(a.ctx, "ContextGenerator not initialized")
		runtime.EventsEmit(a.ctx, "shotgunContextError", "Internal error: ContextGenerator not initialized")
		return
	}

	if len(projectPaths) == 0 {
		runtime.LogError(a.ctx, "No project paths provided")
		runtime.EventsEmit(a.ctx, "shotgunContextError", "No project paths provided")
		return
	}

	// If only one project, use the existing single-project logic
	if len(projectPaths) == 1 {
		a.contextGenerator.requestShotgunContextGenerationInternal(projectPaths[0], excludedPaths)
		return
	}

	// Multiple projects - use new multi-project logic
	a.contextGenerator.requestShotgunContextGenerationForMultiplePaths(projectPaths, excludedPaths)
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
func (a *App) generateShotgunOutputWithProgress(jobCtx context.Context, rootDir string, excludedPaths []string) (string, error) {
	if err := jobCtx.Err(); err != nil { // Check for cancellation at the beginning
		return "", err
	}

	excludedMap := make(map[string]bool)
	for _, p := range excludedPaths {
		excludedMap[p] = true
	}
	if excludedMap["."] {
		runtime.LogDebugf(a.ctx, "Project root '%s' (relPath '.') is excluded by frontend. Returning minimal context.", rootDir)
		var sb strings.Builder
		sb.WriteString(filepath.Base(rootDir) + string(os.PathSeparator) + "\n")
		// sb.WriteString("  └── (all content excluded)\n") // Optional marker
		return sb.String(), nil // Return minimal string and no error
	}

	totalItems, err := a.countProcessableItems(jobCtx, rootDir, excludedMap)
	if err != nil {
		return "", fmt.Errorf("failed to count processable items: %w", err)
	}
	progressState := &generationProgressState{processedItems: 0, totalItems: totalItems}
	a.emitProgress(progressState) // Initial progress (0 / total)

	var output strings.Builder
	var fileContents strings.Builder

	// Root directory line
	output.WriteString(filepath.Base(rootDir) + string(os.PathSeparator) + "\n")
	progressState.processedItems++
	a.emitProgress(progressState)
	if output.Len() > maxOutputSizeBytes {
		return "", fmt.Errorf("%w: content limit of %d bytes exceeded after root dir line (size: %d bytes)", ErrContextTooLong, maxOutputSizeBytes, output.Len())
	}

	// buildShotgunTreeRecursive is a recursive helper for generating the tree string and file contents
	var buildShotgunTreeRecursive func(pCtx context.Context, currentPath, prefix string) error
	buildShotgunTreeRecursive = func(pCtx context.Context, currentPath, prefix string) error {
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

			if output.Len()+fileContents.Len() > maxOutputSizeBytes {
				return fmt.Errorf("%w: content limit of %d bytes exceeded during tree generation (size: %d bytes)", ErrContextTooLong, maxOutputSizeBytes, output.Len()+fileContents.Len())
			}

			if entry.IsDir() {
				err := buildShotgunTreeRecursive(pCtx, path, nextPrefix)
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

				// Ensure forward slashes for the name attribute, consistent with documentation.
				relPathForwardSlash := filepath.ToSlash(relPath)

				fileContents.WriteString(fmt.Sprintf("<file path=\"%s\">\n", relPathForwardSlash))
				fileContents.WriteString(string(content))
				fileContents.WriteString("\n</file>\n") // Each file block ends with a newline

				progressState.processedItems++ // For file content
				a.emitProgress(progressState)

				if output.Len()+fileContents.Len() > maxOutputSizeBytes { // Final check after append
					return fmt.Errorf("%w: content limit of %d bytes exceeded after appending file %s (total size: %d bytes)", ErrContextTooLong, maxOutputSizeBytes, relPath, output.Len()+fileContents.Len())
				}
			}
		}
		return nil
	}

	err = buildShotgunTreeRecursive(jobCtx, rootDir, "")
	if err != nil {
		return "", fmt.Errorf("failed to build tree for shotgun: %w", err)
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

type Watchman struct {
	app         *App
	rootDir     string
	fsWatcher   *fsnotify.Watcher
	watchedDirs map[string]bool // Tracks directories explicitly added to fsnotify

	// lastKnownState map[string]fileMeta // Removed, fsnotify handles state
	mu         sync.Mutex // Changed to Mutex for simplicity with Start/Stop/Refresh
	cancelFunc context.CancelFunc

	// Store current patterns to be used by scanDirectoryStateInternal
	currentProjectGitignore *gitignore.GitIgnore
	currentCustomPatterns   *gitignore.GitIgnore
}

func NewWatchman(app *App) *Watchman {
	return &Watchman{
		app:         app,
		watchedDirs: make(map[string]bool),
	}
}

// StartFileWatcher is called by JavaScript to start watching a directory.
func (a *App) StartFileWatcher(rootDirPath string) error {
	runtime.LogInfof(a.ctx, "StartFileWatcher called for: %s", rootDirPath)
	if a.fileWatcher == nil {
		return fmt.Errorf("file watcher not initialized")
	}
	return a.fileWatcher.Start(rootDirPath)
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

func (w *Watchman) Start(newRootDir string) error {
	w.Stop() // Stop any existing watcher

	w.mu.Lock()
	w.rootDir = newRootDir
	if w.rootDir == "" {
		w.mu.Unlock()
		runtime.LogInfo(w.app.ctx, "Watchman: Root directory is empty, not starting.")
		return nil
	}
	w.mu.Unlock()

	// Initialize patterns based on App's current state
	if w.app.useGitignore {
		w.currentProjectGitignore = w.app.projectGitignore
	} else {
		w.currentProjectGitignore = nil
	}
	if w.app.useCustomIgnore {
		w.currentCustomPatterns = w.app.currentCustomIgnorePatterns
	} else {
		w.currentCustomPatterns = nil
	}

	w.mu.Lock()
	// Ensure settings are loaded if they haven't been (e.g. if called before startup completes, though unlikely)
	// However, loadSettings is called in startup, so this should generally be populated.
	ctx, cancel := context.WithCancel(w.app.ctx) // Use app's context as parent
	w.cancelFunc = cancel
	w.mu.Unlock()

	var err error
	w.fsWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		runtime.LogErrorf(w.app.ctx, "Watchman: Error creating fsnotify watcher: %v", err)
		return fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}
	w.watchedDirs = make(map[string]bool) // Initialize/clear

	runtime.LogInfof(w.app.ctx, "Watchman: Starting for directory %s", newRootDir)
	w.addPathsToWatcherRecursive(newRootDir) // Add initial paths

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
	w.rootDir = ""
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
	currentRootDir := w.rootDir
	w.mu.Unlock()
	runtime.LogInfof(w.app.ctx, "Watchman: Monitoring goroutine started for %s", currentRootDir)

	for {
		select {
		case <-ctx.Done():
			w.mu.Lock()
			shutdownRootDir := w.rootDir // Re-fetch rootDir under lock as it might have changed
			w.mu.Unlock()
			runtime.LogInfof(w.app.ctx, "Watchman: Context cancelled, shutting down watcher for %s.", shutdownRootDir)
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				runtime.LogInfo(w.app.ctx, "Watchman: fsnotify events channel closed.")
				return
			}
			runtime.LogDebugf(w.app.ctx, "Watchman: fsnotify event: %s", event)

			w.mu.Lock()
			currentRootDir = w.rootDir // Update currentRootDir under lock
			// Safely copy ignore patterns
			projIgn := w.currentProjectGitignore
			custIgn := w.currentCustomPatterns
			w.mu.Unlock()

			if currentRootDir == "" { // Watcher might have been stopped
				continue
			}

			relEventPath, err := filepath.Rel(currentRootDir, event.Name)
			if err != nil {
				runtime.LogWarningf(w.app.ctx, "Watchman: Could not get relative path for event %s (root: %s): %v", event.Name, currentRootDir, err)
				continue
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
				runtime.LogInfof(w.app.ctx, "Watchman: Relevant change detected for %s in %s", event.Name, currentRootDir)
				w.app.notifyFileChange(currentRootDir)
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
						w.addPathsToWatcherRecursive(event.Name) // This will add event.Name and its children
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

func (w *Watchman) addPathsToWatcherRecursive(baseDirToAdd string) {
	w.mu.Lock() // Lock to access watcher and ignore patterns
	fsW := w.fsWatcher
	projIgn := w.currentProjectGitignore
	custIgn := w.currentCustomPatterns
	overallRoot := w.rootDir
	w.mu.Unlock()

	if fsW == nil || overallRoot == "" {
		runtime.LogWarningf(w.app.ctx, "Watchman.addPathsToWatcherRecursive: fsWatcher is nil or rootDir is empty. Skipping add for %s.", baseDirToAdd)
		return
	}

	filepath.WalkDir(baseDirToAdd, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			runtime.LogWarningf(w.app.ctx, "Watchman scan error accessing %s: %v", path, walkErr)
			if d != nil && d.IsDir() && path != overallRoot { // Changed scanRootDir to overallRoot for clarity
				return filepath.SkipDir
			}
			return nil // Try to continue
		}

		if !d.IsDir() {
			return nil
		}

		relPath, errRel := filepath.Rel(overallRoot, path)
		if errRel != nil {
			runtime.LogWarningf(w.app.ctx, "Watchman.addPathsToWatcherRecursive: Could not get relative path for %s (root: %s): %v", path, overallRoot, errRel)
			return nil // Continue with other paths
		}

		// Skip .git directory at the top level of overallRoot
		if d.IsDir() && d.Name() == ".git" {
			parentDir := filepath.Dir(path)
			if parentDir == overallRoot {
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
	if w.rootDir == "" {
		w.mu.Unlock()
		runtime.LogInfo(w.app.ctx, "Watchman.RefreshIgnoresAndRescan: No rootDir, skipping.")
		return nil
	}
	runtime.LogInfo(w.app.ctx, "Watchman.RefreshIgnoresAndRescan: Refreshing ignore patterns and re-scanning.")

	// Update patterns based on App's current state
	if w.app.useGitignore {
		w.currentProjectGitignore = w.app.projectGitignore
	} else {
		w.currentProjectGitignore = nil
	}
	if w.app.useCustomIgnore {
		w.currentCustomPatterns = w.app.currentCustomIgnorePatterns
	} else {
		w.currentCustomPatterns = nil
	}
	currentRootDir := w.rootDir
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

	w.addPathsToWatcherRecursive(currentRootDir) // Add paths with new rules
	w.app.notifyFileChange(currentRootDir)       // Notify frontend to refresh its view

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
	// CompileIgnoreLines should handle empty/comment lines appropriately based on .gitignore syntax
	validLines := append([]string{}, lines...)

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
			runtime.LogInfo(a.ctx, "Settings file not found. Using default custom ignore rules (embedded) and attempting to save them.")
			// Save default settings to create the file. compileCustomIgnorePatterns will be called after this.
			if errSave := a.saveSettings(); errSave != nil { // saveSettings will use a.settings.CustomIgnoreRules which is currently default
				runtime.LogErrorf(a.ctx, "Failed to save default settings: %v", errSave)
			}
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
			}
		}
	}

	if errCompile := a.compileCustomIgnorePatterns(); errCompile != nil {
		// Error already logged in compileCustomIgnorePatterns
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
	// Однако, loadSettings вызывается в старте, так что это должно быть населено обычно.
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

	if a.fileWatcher != nil && a.fileWatcher.rootDir != "" {
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
	if a.fileWatcher != nil && a.fileWatcher.rootDir != "" {
		// Assuming watcher is for the current project if active.
		return a.fileWatcher.RefreshIgnoresAndRescan()
	}
	return nil
}

// SetUseCustomIgnore updates the app's setting for using custom ignore rules and informs the watcher.
func (a *App) SetUseCustomIgnore(enabled bool) error {
	a.useCustomIgnore = enabled
	runtime.LogInfof(a.ctx, "App setting useCustomIgnore changed to: %v", enabled)
	if a.fileWatcher != nil && a.fileWatcher.rootDir != "" {
		// Assuming watcher is for the current project if active.
		return a.fileWatcher.RefreshIgnoresAndRescan()
	}
	return nil
}

// --- Multiple Projects Management ---

// AddProject adds a new project to the app
func (a *App) AddProject(dirPath string) (*Project, error) {
	// Generate a unique ID for the project
	projectID := fmt.Sprintf("project_%d", time.Now().UnixNano())
	projectName := filepath.Base(dirPath)

	project := &Project{
		ID:            projectID,
		Name:          projectName,
		RootPath:      dirPath,
		ExcludedPaths: make(map[string]bool),
	}

	// Load .gitignore for this project
	gitignorePath := filepath.Join(dirPath, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		gitIgn, err := gitignore.CompileIgnoreFile(gitignorePath)
		if err != nil {
			runtime.LogWarningf(a.ctx, "Error compiling .gitignore file at %s: %v", gitignorePath, err)
		} else {
			project.Gitignore = gitIgn
			runtime.LogDebugf(a.ctx, "Compiled .gitignore for project %s", projectName)
		}
	}

	// Build initial file tree
	fileTree, err := a.buildProjectFileTree(project)
	if err != nil {
		return nil, fmt.Errorf("failed to build file tree for project %s: %w", projectName, err)
	}
	project.FileTree = fileTree

	// Add to projects map and order
	a.projects[projectID] = project
	a.projectOrder = append(a.projectOrder, projectID)

	runtime.LogInfof(a.ctx, "Added project: %s (ID: %s)", projectName, projectID)
	return project, nil
}

// RemoveProject removes a project from the app
func (a *App) RemoveProject(projectID string) error {
	if _, exists := a.projects[projectID]; !exists {
		return fmt.Errorf("project with ID %s not found", projectID)
	}

	// Remove from projects map
	delete(a.projects, projectID)

	// Remove from project order
	for i, id := range a.projectOrder {
		if id == projectID {
			a.projectOrder = append(a.projectOrder[:i], a.projectOrder[i+1:]...)
			break
		}
	}

	runtime.LogInfof(a.ctx, "Removed project with ID: %s", projectID)
	return nil
}

// GetProjects returns all projects in display order
func (a *App) GetProjects() []*Project {
	projects := make([]*Project, 0, len(a.projectOrder))
	for _, projectID := range a.projectOrder {
		if project, exists := a.projects[projectID]; exists {
			projects = append(projects, project)
		}
	}
	return projects
}

// GetProject returns a specific project by ID
func (a *App) GetProject(projectID string) (*Project, bool) {
	project, exists := a.projects[projectID]
	return project, exists
}

// buildProjectFileTree builds the file tree for a specific project
func (a *App) buildProjectFileTree(project *Project) ([]*FileNode, error) {
	rootNode := &FileNode{
		Name:            project.Name,
		Path:            project.RootPath,
		RelPath:         ".",
		IsDir:           true,
		ProjectID:       project.ID,
		IsGitignored:    false,
		IsCustomIgnored: a.currentCustomIgnorePatterns != nil && a.currentCustomIgnorePatterns.MatchesPath("."),
	}

	children, err := a.buildTreeRecursiveForProject(context.TODO(), project.RootPath, project.RootPath, project.Gitignore, a.currentCustomIgnorePatterns, 0, project.ID)
	if err != nil {
		return []*FileNode{rootNode}, fmt.Errorf("error building children tree for %s: %w", project.RootPath, err)
	}
	rootNode.Children = children

	return []*FileNode{rootNode}, nil
}

// buildTreeRecursiveForProject is similar to buildTreeRecursive but includes project ID
func (a *App) buildTreeRecursiveForProject(ctx context.Context, currentPath, rootPath string, gitIgn *gitignore.GitIgnore, customIgn *gitignore.GitIgnore, depth int, projectID string) ([]*FileNode, error) {
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

		isGitignored := false
		isCustomIgnored := false
		pathToMatch := relPath
		if entry.IsDir() {
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

		node := &FileNode{
			Name:            entry.Name(),
			Path:            nodePath,
			RelPath:         relPath,
			IsDir:           entry.IsDir(),
			ProjectID:       projectID,
			IsGitignored:    isGitignored,
			IsCustomIgnored: isCustomIgnored,
		}

		if entry.IsDir() {
			if !isGitignored && !isCustomIgnored {
				children, err := a.buildTreeRecursiveForProject(ctx, nodePath, rootPath, gitIgn, customIgn, depth+1, projectID)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return nil, err
					}
					runtime.LogWarningf(context.Background(), "Error building subtree for %s: %v", nodePath, err)
				} else {
					node.Children = children
				}
			}
		}
		nodes = append(nodes, node)
	}

	// Sort nodes: directories first, then files, then alphabetically
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir // directories first
		}
		return nodes[i].Name < nodes[j].Name
	})

	return nodes, nil
}

// RefreshProject rebuilds the file tree for a specific project
func (a *App) RefreshProject(projectID string) error {
	project, exists := a.projects[projectID]
	if !exists {
		return fmt.Errorf("project with ID %s not found", projectID)
	}

	fileTree, err := a.buildProjectFileTree(project)
	if err != nil {
		return fmt.Errorf("failed to refresh file tree for project %s: %w", project.Name, err)
	}

	project.FileTree = fileTree
	runtime.LogInfof(a.ctx, "Refreshed project: %s", project.Name)
	return nil
}

// SelectDirectoryAndAddProject opens a dialog to select a directory and adds it as a project
func (a *App) SelectDirectoryAndAddProject() (*Project, error) {
	dirPath, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{})
	if err != nil {
		return nil, err
	}
	if dirPath == "" {
		return nil, nil // User cancelled
	}

	return a.AddProject(dirPath)
}

// ListAllProjects returns all projects with their file trees
func (a *App) ListAllProjects() ([]*Project, error) {
	return a.GetProjects(), nil
}

// ToggleExcludeNodeInProject toggles the exclusion state of a node in a specific project
func (a *App) ToggleExcludeNodeInProject(projectID, nodePath string, excluded bool) error {
	project, exists := a.projects[projectID]
	if !exists {
		return fmt.Errorf("project with ID %s not found", projectID)
	}

	// Convert absolute path to relative path
	relPath, err := filepath.Rel(project.RootPath, nodePath)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %w", err)
	}

	if excluded {
		project.ExcludedPaths[relPath] = true
	} else {
		delete(project.ExcludedPaths, relPath)
	}

	runtime.LogInfof(a.ctx, "Toggled exclusion for %s in project %s: %v", relPath, project.Name, excluded)
	return nil
}

// GetExcludedPathsForProject returns the excluded paths for a specific project
func (a *App) GetExcludedPathsForProject(projectID string) ([]string, error) {
	project, exists := a.projects[projectID]
	if !exists {
		return nil, fmt.Errorf("project with ID %s not found", projectID)
	}

	excludedPaths := make([]string, 0, len(project.ExcludedPaths))
	for path := range project.ExcludedPaths {
		excludedPaths = append(excludedPaths, path)
	}

	return excludedPaths, nil
}

// RequestShotgunContextGenerationForAllProjects generates context for all projects
func (a *App) RequestShotgunContextGenerationForAllProjects() {
	if a.contextGenerator == nil {
		runtime.LogError(a.ctx, "ContextGenerator not initialized")
		runtime.EventsEmit(a.ctx, "shotgunContextError", "Internal error: ContextGenerator not initialized")
		return
	}

	projects := a.GetProjects()
	if len(projects) == 0 {
		runtime.EventsEmit(a.ctx, "shotgunContextError", "No projects added")
		return
	}

	a.contextGenerator.requestShotgunContextGenerationForMultipleProjects(projects)
}

// requestShotgunContextGenerationForMultipleProjects generates context for multiple projects
func (cg *ContextGenerator) requestShotgunContextGenerationForMultipleProjects(projects []*Project) {
	cg.mu.Lock()
	if cg.currentCancelFunc != nil {
		runtime.LogDebug(cg.app.ctx, "Cancelling previous context generation job.")
		cg.currentCancelFunc()
	}

	genCtx, cancel := context.WithCancel(cg.app.ctx)
	myToken := new(struct{}) // Create a unique token for this generation job
	cg.currentCancelFunc = cancel
	cg.currentCancelToken = myToken
	runtime.LogInfof(cg.app.ctx, "Starting new shotgun context generation for %d projects. Max size: %d bytes.", len(projects), maxOutputSizeBytes)
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
			runtime.LogInfo(cg.app.ctx, fmt.Sprintf("Context generation for multiple projects cancelled before starting: %v", genCtx.Err()))
			return
		}

		output, err := cg.app.generateShotgunOutputForMultipleProjects(genCtx, projects)

		select {
		case <-genCtx.Done():
			errMsg := fmt.Sprintf("Shotgun context generation cancelled for multiple projects: %v", genCtx.Err())
			runtime.LogInfo(cg.app.ctx, errMsg)
			runtime.EventsEmit(cg.app.ctx, "shotgunContextError", errMsg)
		default:
			if err != nil {
				errMsg := fmt.Sprintf("Error generating shotgun output for multiple projects: %v", err)
				runtime.LogError(cg.app.ctx, errMsg)
				runtime.EventsEmit(cg.app.ctx, "shotgunContextError", errMsg)
			} else {
				finalSize := len(output)
				successMsg := fmt.Sprintf("Shotgun context generated successfully for %d projects. Size: %d bytes.", len(projects), finalSize)
				if finalSize > maxOutputSizeBytes {
					runtime.LogWarningf(cg.app.ctx, "Warning: Generated context size %d exceeds max %d, but was not caught by ErrContextTooLong.", finalSize, maxOutputSizeBytes)
				}
				runtime.LogInfo(cg.app.ctx, successMsg)
				runtime.EventsEmit(cg.app.ctx, "shotgunContextGenerated", output)
			}
		}
	}(myToken)
}

// generateShotgunOutputForMultipleProjects generates combined output for multiple projects
func (a *App) generateShotgunOutputForMultipleProjects(jobCtx context.Context, projects []*Project) (string, error) {
	if err := jobCtx.Err(); err != nil {
		return "", err
	}

	var output strings.Builder
	var fileContents strings.Builder
	// Filter projects to only include those with content
	var projectsWithContent []*Project
	for _, project := range projects {
		hasContent, err := a.hasIncludableContent(jobCtx, project)
		if err != nil {
			return "", fmt.Errorf("failed to check content for project %s: %w", project.Name, err)
		}
		if hasContent {
			projectsWithContent = append(projectsWithContent, project)
		}
	}

	// If no projects have content, return empty result
	if len(projectsWithContent) == 0 {
		return "", nil
	}

	// Calculate total items across projects with content for progress tracking
	totalItems := 0
	for _, project := range projectsWithContent {
		excludedMap := project.ExcludedPaths
		count, err := a.countProcessableItemsForProject(jobCtx, project, excludedMap)
		if err != nil {
			return "", fmt.Errorf("failed to count processable items for project %s: %w", project.Name, err)
		}
		totalItems += count
	}

	progressState := &generationProgressState{processedItems: 0, totalItems: totalItems}
	a.emitProgress(progressState)
	// Process each project with content
	for i, project := range projectsWithContent {
		if err := jobCtx.Err(); err != nil {
			return "", err
		}

		// Add project header
		projectHeader := fmt.Sprintf("\n=== PROJECT %d: %s ===\n", i+1, project.Name)
		output.WriteString(projectHeader)
		progressState.processedItems++
		a.emitProgress(progressState)

		if output.Len() > maxOutputSizeBytes {
			return "", fmt.Errorf("%w: content limit of %d bytes exceeded after project header (size: %d bytes)", ErrContextTooLong, maxOutputSizeBytes, output.Len())
		}

		// Add project root directory line
		output.WriteString(project.Name + string(os.PathSeparator) + "\n")
		progressState.processedItems++
		a.emitProgress(progressState)

		if output.Len() > maxOutputSizeBytes {
			return "", fmt.Errorf("%w: content limit of %d bytes exceeded after project root dir line (size: %d bytes)", ErrContextTooLong, maxOutputSizeBytes, output.Len())
		}

		// Build tree and file contents for this project
		err := a.buildShotgunTreeRecursiveForProject(jobCtx, project, "", &output, &fileContents, progressState)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return "", err
			}
			return "", fmt.Errorf("failed to build content for project %s: %w", project.Name, err)
		}

		// Check size limit after each project
		if output.Len()+fileContents.Len() > maxOutputSizeBytes {
			return "", fmt.Errorf("%w: content limit of %d bytes exceeded after project %s (size: %d bytes)", ErrContextTooLong, maxOutputSizeBytes, project.Name, output.Len()+fileContents.Len())
		}
	}

	return output.String() + "\n" + strings.TrimRight(fileContents.String(), "\n"), nil
}

// countProcessableItemsForProject counts items for a specific project
func (a *App) countProcessableItemsForProject(jobCtx context.Context, project *Project, excludedMap map[string]bool) (int, error) {
	count := 2 // For project header and root directory line

	var counterHelper func(currentPath string) error
	counterHelper = func(currentPath string) error {
		select {
		case <-jobCtx.Done():
			return jobCtx.Err()
		default:
		}

		entries, err := os.ReadDir(currentPath)
		if err != nil {
			runtime.LogWarningf(a.ctx, "countProcessableItemsForProject: error reading dir %s: %v", currentPath, err)
			return nil
		}

		for _, entry := range entries {
			path := filepath.Join(currentPath, entry.Name())
			relPath, _ := filepath.Rel(project.RootPath, path)

			if excludedMap[relPath] {
				continue
			}

			count++ // For the tree entry

			if entry.IsDir() {
				err := counterHelper(path)
				if err != nil {
					return err
				}
			} else {
				count++ // For reading the file content
			}
		}
		return nil
	}

	err := counterHelper(project.RootPath)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// buildShotgunTreeRecursiveForProject builds tree content for a specific project
func (a *App) buildShotgunTreeRecursiveForProject(pCtx context.Context, project *Project, prefix string, output, fileContents *strings.Builder, progressState *generationProgressState) error {
	select {
	case <-pCtx.Done():
		return pCtx.Err()
	default:
	}

	entries, err := os.ReadDir(project.RootPath)
	if err != nil {
		runtime.LogWarningf(a.ctx, "buildShotgunTreeRecursiveForProject: error reading dir %s: %v", project.RootPath, err)
		return nil
	}

	// Sort entries
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		select {
		case <-pCtx.Done():
			return pCtx.Err()
		default:
		}

		path := filepath.Join(project.RootPath, entry.Name())
		relPath, _ := filepath.Rel(project.RootPath, path)

		// Check if excluded
		if project.ExcludedPaths[relPath] {
			continue
		}

		// Check ignore rules
		isGitignored := false
		isCustomIgnored := false
		pathToMatch := relPath
		if entry.IsDir() {
			if !strings.HasSuffix(pathToMatch, string(os.PathSeparator)) {
				pathToMatch += string(os.PathSeparator)
			}
		}

		if project.Gitignore != nil && a.useGitignore {
			isGitignored = project.Gitignore.MatchesPath(pathToMatch)
		}
		if a.currentCustomIgnorePatterns != nil && a.useCustomIgnore {
			isCustomIgnored = a.currentCustomIgnorePatterns.MatchesPath(pathToMatch)
		}

		if isGitignored || isCustomIgnored {
			continue
		}

		// Add to tree
		if entry.IsDir() {
			output.WriteString(prefix + entry.Name() + string(os.PathSeparator) + "\n")
		} else {
			output.WriteString(prefix + entry.Name() + "\n")
		}

		progressState.processedItems++
		a.emitProgress(progressState)

		if output.Len() > maxOutputSizeBytes {
			return fmt.Errorf("%w: tree content limit exceeded (size: %d bytes)", ErrContextTooLong, output.Len())
		}

		// Process files and directories
		if entry.IsDir() {
			err := a.buildShotgunTreeRecursiveForProjectDir(pCtx, path, project.RootPath, project, prefix+"  ", output, fileContents, progressState)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				runtime.LogWarningf(a.ctx, "Error processing directory %s: %v", path, err)
			}
		} else {
			err := a.addFileContentToOutput(pCtx, path, relPath, fileContents, progressState)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				runtime.LogWarningf(a.ctx, "Error reading file %s: %v", path, err)
			}
		}
	}

	return nil
}

// buildShotgunTreeRecursiveForProjectDir recursively processes directories within a project
func (a *App) buildShotgunTreeRecursiveForProjectDir(pCtx context.Context, currentPath, rootPath string, project *Project, prefix string, output, fileContents *strings.Builder, progressState *generationProgressState) error {
	select {
	case <-pCtx.Done():
		return pCtx.Err()
	default:
	}

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return err
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		select {
		case <-pCtx.Done():
			return pCtx.Err()
		default:
		}

		path := filepath.Join(currentPath, entry.Name())
		relPath, _ := filepath.Rel(rootPath, path)

		if project.ExcludedPaths[relPath] {
			continue
		}

		// Check ignore rules
		isGitignored := false
		isCustomIgnored := false
		pathToMatch := relPath
		if entry.IsDir() {
			if !strings.HasSuffix(pathToMatch, string(os.PathSeparator)) {
				pathToMatch += string(os.PathSeparator)
			}
		}

		if project.Gitignore != nil && a.useGitignore {
			isGitignored = project.Gitignore.MatchesPath(pathToMatch)
		}
		if a.currentCustomIgnorePatterns != nil && a.useCustomIgnore {
			isCustomIgnored = a.currentCustomIgnorePatterns.MatchesPath(pathToMatch)
		}

		if isGitignored || isCustomIgnored {
			continue
		}

		// Add to tree
		if entry.IsDir() {
			output.WriteString(prefix + entry.Name() + string(os.PathSeparator) + "\n")
		} else {
			output.WriteString(prefix + entry.Name() + "\n")
		}

		progressState.processedItems++
		a.emitProgress(progressState)

		if output.Len() > maxOutputSizeBytes {
			return fmt.Errorf("%w: tree content limit exceeded (size: %d bytes)", ErrContextTooLong, output.Len())
		}

		if entry.IsDir() {
			err := a.buildShotgunTreeRecursiveForProjectDir(pCtx, path, rootPath, project, prefix+"  ", output, fileContents, progressState)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				runtime.LogWarningf(a.ctx, "Error processing directory %s: %v", path, err)
			}
		} else {
			err := a.addFileContentToOutput(pCtx, path, relPath, fileContents, progressState)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				runtime.LogWarningf(a.ctx, "Error reading file %s: %v", path, err)
			}
		}
	}

	return nil
}

// addFileContentToOutput adds file content to the output
func (a *App) addFileContentToOutput(pCtx context.Context, filePath, relPath string, fileContents *strings.Builder, progressState *generationProgressState) error {
	select {
	case <-pCtx.Done():
		return pCtx.Err()
	default:
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	fileContents.WriteString(fmt.Sprintf("<file path=\"%s\">\n", relPath))
	fileContents.Write(content)
	if !strings.HasSuffix(string(content), "\n") {
		fileContents.WriteString("\n")
	}
	fileContents.WriteString("</file>\n")

	progressState.processedItems++
	a.emitProgress(progressState)

	if fileContents.Len() > maxOutputSizeBytes {
		return fmt.Errorf("%w: file content limit exceeded (size: %d bytes)", ErrContextTooLong, fileContents.Len())
	}

	return nil
}

// requestShotgunContextGenerationForMultiplePaths generates context for multiple project paths
func (cg *ContextGenerator) requestShotgunContextGenerationForMultiplePaths(projectPaths []string, excludedPaths []string) {
	cg.mu.Lock()
	if cg.currentCancelFunc != nil {
		cg.currentCancelFunc()
	}

	genCtx, cancel := context.WithCancel(cg.app.ctx)
	myToken := new(struct{})
	cg.currentCancelFunc = cancel
	cg.currentCancelToken = myToken
	cg.mu.Unlock()

	go func(tokenForThisJob interface{}) {
		defer func() {
			cg.mu.Lock()
			if cg.currentCancelToken == tokenForThisJob {
				cg.currentCancelFunc = nil
				cg.currentCancelToken = nil
			}
			cg.mu.Unlock()
		}()

		excludedMap := make(map[string]bool)
		for _, path := range excludedPaths {
			excludedMap[path] = true
		}

		var outputBuilder strings.Builder
		for i, projectPath := range projectPaths {
			select {
			case <-genCtx.Done():
				runtime.EventsEmit(cg.app.ctx, "shotgunContextCancelled", "Context generation was cancelled")
				return
			default:
			}

			if i > 0 {
				outputBuilder.WriteString("\n\n")
			}

			projectName := filepath.Base(projectPath)
			outputBuilder.WriteString(fmt.Sprintf("=== PROJECT: %s ===\n", projectName))
			outputBuilder.WriteString(fmt.Sprintf("Project Root: %s\n\n", projectPath))

			projectContent, _, _, err := cg.generateContextForSingleProject(genCtx, projectPath, excludedMap)
			if err != nil {
				outputBuilder.WriteString(fmt.Sprintf("ERROR: Failed to process project: %v\n", err))
				continue
			}

			if outputBuilder.Len()+len(projectContent) > maxOutputSizeBytes {
				outputBuilder.WriteString("*** TRUNCATED: Context size limit reached ***\n")
				break
			}

			outputBuilder.WriteString(projectContent)
			runtime.EventsEmit(cg.app.ctx, "shotgunContextProgress", map[string]interface{}{
				"current": i + 1,
				"total":   len(projectPaths),
				"message": fmt.Sprintf("Processed project: %s", projectName),
			})
		}

		runtime.EventsEmit(cg.app.ctx, "shotgunContextGenerated", outputBuilder.String())
	}(myToken)
}

// generateContextForSingleProject generates context for a single project path
func (cg *ContextGenerator) generateContextForSingleProject(ctx context.Context, projectPath string, excludedMap map[string]bool) (string, int, int, error) {
	if excludedMap["."] {
		runtime.LogDebugf(cg.app.ctx, "Project root '%s' (relPath '.') is explicitly excluded by frontend. Generating minimal output for this project.", projectPath)

		var minimalOutput strings.Builder
		// The shotgun format expects the project root name even if its contents are excluded.
		minimalOutput.WriteString(filepath.Base(projectPath) + string(os.PathSeparator) + "\n")
		// Optionally, add a marker to indicate to the user why it's empty:
		// minimalOutput.WriteString("  └── (all content excluded)\n")

		// Return this minimal tree string, 0 files, 0 size.
		return minimalOutput.String(), 0, 0, nil
	}
	if err := ctx.Err(); err != nil {
		return "", 0, 0, err
	}

	var output strings.Builder
	var fileContents strings.Builder

	// Root directory line
	output.WriteString(filepath.Base(projectPath) + string(os.PathSeparator) + "\n")
	if output.Len() > maxOutputSizeBytes {
		return "", 0, 0, fmt.Errorf("%w: content limit of %d bytes exceeded after root dir line (size: %d bytes)", ErrContextTooLong, maxOutputSizeBytes, output.Len())
	}
	var fileCount, totalSize int
	var buildShotgunTreeRecursive func(pCtx context.Context, currentPath, prefix string) error
	buildShotgunTreeRecursive = func(pCtx context.Context, currentPath, prefix string) error {
		select {
		case <-pCtx.Done():
			return pCtx.Err()
		default:
		}

		entries, err := os.ReadDir(currentPath)
		if err != nil {
			runtime.LogWarningf(cg.app.ctx, "buildShotgunTreeRecursive: error reading dir %s: %v", currentPath, err)
			return nil // Skip this directory
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

		for _, entry := range entries {
			select {
			case <-pCtx.Done():
				return pCtx.Err()
			default:
			}

			path := filepath.Join(currentPath, entry.Name())
			relPath, _ := filepath.Rel(projectPath, path)

			if excludedMap[relPath] {
				continue
			}

			// Add to tree
			if entry.IsDir() {
				output.WriteString(prefix + entry.Name() + string(os.PathSeparator) + "\n")
			} else {
				output.WriteString(prefix + entry.Name() + "\n")
			}

			if output.Len() > maxOutputSizeBytes {
				return fmt.Errorf("%w: content limit of %d bytes exceeded during tree generation (size: %d bytes)", ErrContextTooLong, maxOutputSizeBytes, output.Len())
			}

			// Process files and directories
			if entry.IsDir() {
				err := buildShotgunTreeRecursive(pCtx, path, prefix+"  ")
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return err
					}
					runtime.LogWarningf(cg.app.ctx, "Error processing directory %s: %v", path, err)
				}
			} else {
				select { // Check before heavy I/O
				case <-pCtx.Done():
					return pCtx.Err()
				default:
				}
				content, err := os.ReadFile(path)
				if err != nil {
					runtime.LogWarningf(cg.app.ctx, "Error reading file %s: %v", path, err)
					content = []byte(fmt.Sprintf("Error reading file: %v", err))
				}

				fileCount++
				totalSize += len(content)

				// Ensure forward slashes for the name attribute, consistent with documentation.
				relPathForwardSlash := filepath.ToSlash(relPath)

				fileContents.WriteString(fmt.Sprintf("<file path=\"%s\">\n", relPathForwardSlash))
				fileContents.WriteString(string(content))
				fileContents.WriteString("\n</file>\n") // Each file block ends with a newline

				if output.Len()+fileContents.Len() > maxOutputSizeBytes { // Final check after append
					return fmt.Errorf("%w: content limit of %d bytes exceeded after appending file %s (total size: %d bytes)", ErrContextTooLong, maxOutputSizeBytes, relPath, output.Len()+fileContents.Len())
				}
			}
		}
		return nil
	}

	err := buildShotgunTreeRecursive(ctx, projectPath, "")
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to build tree for shotgun: %w", err)
	}

	if err := ctx.Err(); err != nil { // Check for cancellation before final string operations
		return "", 0, 0, err
	}

	// The final output is the tree, a newline, then all concatenated file contents.
	return output.String() + "\n" + strings.TrimRight(fileContents.String(), "\n"), fileCount, totalSize, nil
}

// hasIncludableContent checks if a project has any files that would be included in the output
func (a *App) hasIncludableContent(jobCtx context.Context, project *Project) (bool, error) {
	var hasContent bool

	var checkHelper func(currentPath string) error
	checkHelper = func(currentPath string) error {
		if hasContent {
			return nil // Early exit if we already found content
		}

		select {
		case <-jobCtx.Done():
			return jobCtx.Err()
		default:
		}

		entries, err := os.ReadDir(currentPath)
		if err != nil {
			runtime.LogWarningf(a.ctx, "hasIncludableContent: error reading dir %s: %v", currentPath, err)
			return nil
		}

		for _, entry := range entries {
			if hasContent {
				return nil // Early exit if we already found content
			}

			path := filepath.Join(currentPath, entry.Name())
			relPath, _ := filepath.Rel(project.RootPath, path)

			// Check if excluded by user
			if project.ExcludedPaths[relPath] {
				continue
			}

			// Check ignore rules
			isGitignored := false
			isCustomIgnored := false
			pathToMatch := relPath
			if entry.IsDir() {
				if !strings.HasSuffix(pathToMatch, string(os.PathSeparator)) {
					pathToMatch += string(os.PathSeparator)
				}
			}

			if project.Gitignore != nil && a.useGitignore {
				isGitignored = project.Gitignore.MatchesPath(pathToMatch)
			}
			if a.currentCustomIgnorePatterns != nil && a.useCustomIgnore {
				isCustomIgnored = a.currentCustomIgnorePatterns.MatchesPath(pathToMatch)
			}

			if isGitignored || isCustomIgnored {
				continue
			}

			// If we reach here, we found includable content
			hasContent = true
			return nil
		}

		// Check subdirectories
		for _, entry := range entries {
			if hasContent {
				return nil // Early exit if we already found content
			}

			if entry.IsDir() {
				path := filepath.Join(currentPath, entry.Name())
				relPath, _ := filepath.Rel(project.RootPath, path)

				// Check if excluded by user
				if project.ExcludedPaths[relPath] {
					continue
				}

				// Check ignore rules
				pathToMatch := relPath
				if !strings.HasSuffix(pathToMatch, string(os.PathSeparator)) {
					pathToMatch += string(os.PathSeparator)
				}

				isGitignored := false
				isCustomIgnored := false
				if project.Gitignore != nil && a.useGitignore {
					isGitignored = project.Gitignore.MatchesPath(pathToMatch)
				}
				if a.currentCustomIgnorePatterns != nil && a.useCustomIgnore {
					isCustomIgnored = a.currentCustomIgnorePatterns.MatchesPath(pathToMatch)
				}

				if !isGitignored && !isCustomIgnored {
					err := checkHelper(path)
					if err != nil {
						return err
					}
				}
			}
		}

		return nil
	}

	err := checkHelper(project.RootPath)
	if err != nil {
		return false, err
	}

	return hasContent, nil
}
