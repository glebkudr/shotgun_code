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

type App struct {
	ctx                         context.Context
	contextGenerator            *ContextGenerator
	fileWatcher                 *Watchman
	settings                    AppSettings
	currentCustomIgnorePatterns *gitignore.GitIgnore
	configPath                  string
	useGitignore                bool
	useCustomIgnore             bool
	projectGitignore            *gitignore.GitIgnore // Compiled .gitignore for the current project
}

func NewApp() *App {
	return &App{}
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
func (a *App) RequestShotgunContextGeneration(rootDir string, excludedPaths []string) {
	if a.contextGenerator == nil {
		// This should not happen if startup initializes it correctly
		runtime.LogError(a.ctx, "ContextGenerator not initialized")
		runtime.EventsEmit(a.ctx, "shotgunContextError", "Internal error: ContextGenerator not initialized")
		return
	}
	a.contextGenerator.requestShotgunContextGenerationInternal(rootDir, excludedPaths)
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
