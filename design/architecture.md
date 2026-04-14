# Shotgun Application Architecture

## 1. Overview

Shotgun App is a desktop application built with Wails (v2) and Vue.js (v3).
Wails makes it possible to build cross‑platform desktop applications with a Go backend and a web frontend.

-   **Backend:** Go. Handles filesystem operations, logic for determining files/folders to exclude, and generation of the textual "Shotgun" output.
-   **Frontend:** Vue.js (with Vite and Tailwind CSS). Provides the user interface for selecting a folder, displaying the file/folder tree, marking items to exclude, and showing the generated output.
-   **Integration:** Wails provides the bridge between Go functions and JavaScript calls from the frontend.

## 2. Backend (Go)

The Go backend is structured mainly inside the `main` package.

### Key components (`app.go`):

-   **`App` struct**: Stores the application state and its dependencies.
    -   `ctx context.Context`: Wails context.
    -   `contextGenerator *ContextGenerator`: Instance of the context generator.
    -   `fileWatcher *Watchman`: Instance of the filesystem watcher.
    -   `settings AppSettings`: Current application settings (ignore rules, prompt rules).
    -   `currentCustomIgnorePatterns *gitignore.GitIgnore`: Compiled custom ignore rules.
    -   `configPath string`: Path to the `settings.json` configuration file.
    -   `useGitignore bool`: Flag for using `.gitignore` rules.
    -   `useCustomIgnore bool`: Flag for using custom ignore rules.
    -   `projectGitignore *gitignore.GitIgnore`: Compiled rules from the project's `.gitignore`.
-   **`startup(ctx context.Context)`**: Wails lifecycle hook. Initializes `ctx`, `contextGenerator`, `fileWatcher`, and loads settings.
-   **`FileNode` struct**: Represents a file or folder in the tree. Includes `Name`, `Path`, `RelPath`, `IsDir`, `Children`, `IsGitignored`, `IsCustomIgnored`.
-   **`SelectDirectory() (string, error)`**: Opens the system dialog to select a directory.
    -   **`ListFiles(dirPath string) ([]*FileNode, error)`**:
        -   Accepts a directory path.
        -   Loads and compiles `.gitignore` from the given directory (if it exists) and stores it in `projectGitignore`.
        -   Creates the root `FileNode` representing `dirPath`.
        -   Recursively scans `dirPath` using `buildTreeRecursive` to build the tree of child `FileNode`s, taking into account rules from `.gitignore` (if `useGitignore` is enabled) and custom rules (if `useCustomIgnore` is enabled).
        -   Returns a slice containing only the root `FileNode`.

-   **`ContextGenerator` struct**: Manages asynchronous context generation.
    -   `requestShotgunContextGenerationInternal(rootDir string, excludedPaths []string)`: Internal method for starting/restarting generation in a separate goroutine. Handles cancellation of previous jobs.
-   **`RequestShotgunContextGeneration(rootDir string, excludedPaths []string) error`**: Method exposed to the frontend via Wails. Delegates the call to `contextGenerator`.
-   **`countProcessableItems(jobCtx context.Context, rootDir string, excludedMap map[string]bool) (int, error)`**: Recursively counts the number of items (directories, files to be listed, files whose contents will be read) to estimate total progress for context generation.
-   **`generateShotgunOutputWithProgress(jobCtx context.Context, rootDir string, excludedPaths []string) (string, error)`**:
    -   Main function for generating the textual project context.
    -   Accepts the job context `jobCtx` for cancellation, `rootDir`, and the list of `excludedPaths`.
    -   Builds a textual representation of the file tree and aggregates the contents of non‑excluded files in an XML‑like format (`<file path="...">...</file>`).
    -   Periodically calls `emitProgress` to send the `shotgunContextGenerationProgress` event to the frontend.
    -   No hard output size limit: the full selection is always emitted. The frontend surfaces a per-directory token estimate in the tree and a total token counter so users can see when they approach an LLM's context window.
    -   On completion (or on error/cancellation) sends either `shotgunContextGenerated` or `shotgunContextError` to the frontend.
-   **`Watchman` struct**: Component for monitoring filesystem changes.
    -   `StartFileWatcher(rootDirPath string) error` / `StopFileWatcher() error`: Methods exposed to the frontend.
    -   `Start(newRootDir string)`: Initializes and starts watching using `fsnotify`.
    -   `Stop()`: Stops the current watcher.
    -   `run(ctx context.Context)`: Main monitoring loop running in a goroutine. Reacts to `fsnotify` events, filters them using ignore rules, manages adding/removing directories from watching, and notifies the frontend via `App.notifyFileChange`.
    -   `addPathsToWatcherRecursive(baseDirToAdd string)`: Recursively adds directories to `fsnotify.Watcher`, skipping ignored paths.
    -   `RefreshIgnoresAndRescan()`: Reloads ignore rules and restarts `fsnotify.Watcher` with updated paths.
-   **`AppSettings` struct**: Structure for storing settings (`CustomIgnoreRules`, `CustomPromptRules`).

-   **Configuration management**:
    -   `compileCustomIgnorePatterns()`: Compiles textual ignore rules into a `gitignore.GitIgnore` object.
    -   `loadSettings()`: Loads settings from `settings.json` (using `xdg.ConfigFile`). If the file is missing or invalid, uses built‑in defaults (`defaultCustomIgnoreRulesContent` and `defaultCustomPromptRulesContent`).
    -   `saveSettings()`: Persists current settings to `settings.json`.

-   **Methods for managing rules and flags (exposed to the frontend)**:
    -   `GetCustomIgnoreRules() string`
    -   `SetCustomIgnoreRules(rules string) error`
    -   `GetCustomPromptRules() string`
    -   `SetCustomPromptRules(rules string) error`
    -   `SetUseGitignore(enabled bool) error`
    -   `SetUseCustomIgnore(enabled bool) error`

-   **`notifyFileChange(rootDir string)`**: Sends the `projectFilesChanged` event to the frontend.

### `main.go`:

-   Initializes Wails.
-   Configures the application window and its options (title, dimensions, background color).
-   Embeds frontend assets (`embed`).
-   Binds an instance of `App` so its public methods can be called from the frontend.
-   Configures the system menu (for example, the standard macOS menu).

## 3. Frontend (Vue.js)

The frontend is a single‑page application (SPA) built with Vue 3 (Composition API), Vite, and Tailwind CSS.
It implements a multi‑step user interface for preparing project context, composing prompts, "executing" them, and "applying" patches.

### Key components:

-   **`main.js`**: Entry point of the Vue application.
-   **`App.vue`**: Root component that mounts `MainLayout.vue`.
-   **`components/MainLayout.vue`**:
    -   **Structure**: Manages the main layout: horizontal stepper (`HorizontalStepper`), left sidebar (`LeftSidebar`), central content panel (`CentralPanel`), and bottom console (`BottomConsole`).
    -   **State management** (using `ref` and `reactive`):
        -   `currentStep`: Current active step (1–4).
        -   `steps`: Array of step objects with `id`, `title`, `completed`, `description`.
        -   `logMessages`: Array of log messages for `BottomConsole`.
        -   `projectRoot`, `fileTree`, `shotgunPromptContext`, `useGitignore`, `useCustomIgnore`, `loadingError`, `isGeneratingContext`, `generationProgressData`, `userTask`, `rulesContent`, `finalPrompt`: State related to the project, context, and user input.
    -   **Logic**:
        -   Navigation between steps (`navigateToStep`).
        -   Handling actions from step components (`handleStepAction`).
        -   Interaction with the Go backend (calling `SelectDirectoryGo`, `ListFiles`, `RequestShotgunContextGeneration`, settings methods).
        -   Subscribing to Wails events (`shotgunContextGenerated`, `shotgunContextError`, `shotgunContextGenerationProgress`, `projectFilesChanged`).
        -   Managing `Watchman` (start/stop).
        -   Debouncing context generation (`debouncedTriggerShotgunContextGeneration`).
        -   Updating excluded state for nodes in the tree (`updateAllNodesExcludedState`, `toggleExcludeNode`).
-   **`components/HorizontalStepper.vue`**: Displays steps (1–4) at the top and enables navigation.
-   **`components/LeftSidebar.vue`**:
    -   Displays the "Select Project Folder" button and the project path.
    -   Contains the "Use .gitignore rules" and "Use custom rules" checkboxes.
    -   Provides a button (⚙️) to open the modal for editing custom ignore rules (`CustomRulesModal.vue`).
    -   Displays the project file tree using `FileTree.vue`.
    -   Shows the list of steps for navigation.
-   **`components/CentralPanel.vue`**: Dynamically renders the component for the current step.
-   **`components/steps/Step1PrepareContext.vue`**:
    -   UI for the first step. Displays a progress bar while context is being generated.
    -   Shows the generated `generatedContext` in a read‑only `<textarea>` or an error message.
    -   Provides a button to copy the context to the clipboard.
-   **`components/steps/Step2ComposePrompt.vue`**:
    -   UI for the second step.
    -   `<textarea>` for entering the user's task (`userTask`).
    -   `<textarea>` for displaying/editing custom prompt rules (`rulesContent`), with a button (⚙️) to edit them through `CustomRulesModal.vue`.
    -   Read‑only `<textarea>` for displaying file context (`fileListContext` from `shotgunPromptContext`).
    -   Dropdown for selecting a prompt template (`promptTemplates`).
    -   `<textarea>` for displaying the final composed prompt (`finalPrompt`).
    -   Approximate token count indicator and a button to copy the final prompt.
    -   Automatic recomputation of the final prompt whenever input data changes.
-   **`components/steps/Step3ExecutePrompt.vue`**: Placeholder UI for the third step.
-   **`components/steps/Step4ApplyPatch.vue`**: Placeholder UI for the fourth step that imitates a patch editor.
-   **`components/BottomConsole.vue`**: Displays global execution logs passed from `MainLayout.vue`. Console height is resizable by the user.
-   **`components/FileTree.vue`**: Recursive component for rendering the file/folder tree. Allows marking items for exclusion using checkboxes.
-   **`components/CustomRulesModal.vue`**: Modal dialog for editing custom rules (ignore or prompt).
-   **Integration with Wails**:
    -   Calling Go methods from `frontend/wailsjs/go/main/App.js`.
    -   Subscribing to events from Go using `EventsOn` from `frontend/wailsjs/runtime/runtime.js`.

## 4. Data Flow and Application Logic (Multi‑step UI)

The application operates as a sequence of steps coordinated by `MainLayout.vue`:

1.  **Initialization and Step 1 (Prepare Context)**:
    -   The application starts on Step 1.
    -   The user clicks "Select Project Folder". `MainLayout.vue` calls `SelectDirectoryGo`.
    -   On successful directory selection (`projectRoot` is updated):
        -   `MainLayout.vue` calls `ListFiles(projectRoot)` to load the file structure.
        -   The received data is transformed into `fileTree`.
        -   `Watchman` is started for `projectRoot` (`StartFileWatcher`).
        -   `debouncedTriggerShotgunContextGeneration` is automatically invoked to generate `shotgunPromptContext`.
    -   `CentralPanel.vue` renders `Step1PrepareContext.vue`, which shows progress and then the result (`shotgunPromptContext`) or an error.
    -   The user can change `useGitignore`, `useCustomIgnore`, or exclude files in `FileTree.vue`. These actions trigger `debouncedTriggerShotgunContextGeneration`.
    -   Step 1 is considered completed (`completed: true`) when `shotgunPromptContext` is generated successfully.

2.  **Step 2 (Compose Prompt)**:
    -   `CentralPanel.vue` renders `Step2ComposePrompt.vue`.
    -   `shotgunPromptContext` (as `fileListContext`), `userTask`, and `rulesContent` are used to build `finalPrompt` based on the selected template.
    -   The user enters `userTask`, can edit `rulesContent`, and executes the prompt via the LLM integration.
    -   `finalPrompt` is updated automatically.
    -   Step 2 is considered completed when `finalPrompt` is non‑empty and Step 1 is completed.

3.  **Step 3 (Prompt History)**:
    -   `CentralPanel.vue` renders `Step3ExecutePrompt.vue`, which displays previously executed prompts, responses, and API call payloads.
    -   This step is accessible at any time (its navigation button is always enabled) so that users can copy or audit past executions without completing earlier stages first.

**Navigation and state:**

-   `HorizontalStepper.vue` and `LeftSidebar.vue` (step list) provide navigation. The user can go back to completed steps or move to the next incomplete one, and the Prompt History step is always available.
-   The `completed` status of each step in `MainLayout.vue` controls navigation and visual state.
-   The `projectFilesChanged` event from `Watchman` triggers reloading of `fileTree` and, consequently, regeneration of `shotgunPromptContext`, provided the system is not busy with other operations.

## 5. Asynchronous Project Context Generation

The project context (`shotgunPromptContext`) is generated asynchronously to improve UX and UI responsiveness:

-   **Automatic regeneration**: When changes occur in the file tree (project selection, ignore rules update, filesystem changes detected by `Watchman`), the context is automatically regenerated in a background goroutine on the Go side.
-   **Loading indication**: While generation is in progress, the frontend displays a progress bar (`Step1PrepareContext.vue`).
-   **Task cancellation and debouncing**: On rapid consecutive changes, debouncing is used (a delay before starting generation). If a new generation request arrives while the previous one is running, the previous job is cancelled.
-   **No manual trigger**: The "Prepare Project Context & Proceed" button was removed from Step 1, because the context is always kept up to date automatically.

## 6. Improved Context Generation and UI Feedback

### 6.1. Progress reporting

-   **Backend (Go – `app.go`)**:
    -   `countProcessableItems` estimates the total number of operations.
    -   `generateShotgunOutputWithProgress` tracks `processedItems` and `totalItems`.
    -   `emitProgress` periodically sends the `shotgunContextGenerationProgress` event with `{ "current": X, "total": Y }`.
-   **Frontend (Vue.js)**:
    -   `MainLayout.vue` listens for the event and updates `generationProgressData`.
    -   `Step1PrepareContext.vue` shows a progress bar and textual progress information.

### 6.2. Output size guidance (non-blocking)

-   **Backend (Go – `app.go`)**: no hard size cap. Generation always emits the full selection. `FileNode` carries `size` (bytes) and `tokenEstimate` (bytes / 4) aggregated for directories so the frontend can show per-node token counts in the tree.
-   **Frontend (Vue.js)**: `FileTree.vue` shows a `~Xk` token badge next to each node. `Step1PrepareContext.vue` and `LargeTextViewer.vue` render a color-coded total-token counter (green < 500k, yellow < 1.5M, red ≥ 1.5M). Users decide when a selection is too large for the target LLM's context window.

## 7. Cross‑platform Support

-   **Wails**: Natively supports building for Windows, macOS, and Linux from a single codebase.
-   **Go**: Standard library packages (`os`, `path/filepath`) are cross‑platform.
-   **Frontend**: Web technologies (HTML, CSS, JS) are inherently cross‑platform.

## 8. Simplicity and Minimal Dependencies

-   **Go backend**: Primarily uses the Go standard library. Wails is the main external dependency. Additional ones: `github.com/adrg/xdg` (for configuration paths), `github.com/fsnotify/fsnotify` (for filesystem watching), `github.com/sabhiram/go-gitignore` (for parsing `.gitignore`).
-   **Vue frontend**: Vue 3, Vite, Tailwind CSS. UI components (stepper, panels, steps) are custom.

## 9. Configuration Management and Custom Ignore/Prompt Rules

-   **Configuration storage**: Application settings, including custom ignore rules (`CustomIgnoreRules`) and prompt rules (`CustomPromptRules`), are stored in a JSON file (`settings.json`) in the user's standard configuration directory (for example, `~/.config/shotgun-code/settings.json` on Linux). Managed via `github.com/adrg/xdg`.
-   **Default custom ignore rules**: The `ignore.glob` file in the application repository root serves as the source of default rules. They are embedded into the binary at build time (`embed`).
-   **Default prompt rules**: The `defaultCustomPromptRulesContent` string in `app.go`.
-   **Loading rules**: On startup the application attempts to load `settings.json`.
    -   If the file exists and contains valid rules, they are used.
    -   If the file does not exist or rules are missing/invalid, built‑in default values are used. The application also attempts to create/update `settings.json` with these values.
-   **Editing custom rules**:
    -   The "gear" icon (⚙️) next to the "Use custom rules" checkbox in `LeftSidebar` opens the modal (`CustomRulesModal.vue`) for ignore rules.
    -   A similar icon in `Step2ComposePrompt.vue` is used for prompt rules.
    -   The modal allows viewing and editing rules. Ignore rules use `.gitignore` syntax.
    -   Saving rules updates `settings.json`. For ignore rules, this also recompiles them into `currentCustomIgnorePatterns`.
-   **Impact of rule changes**:
    -   Any change to custom ignore rules (via the modal and saving) triggers reloading of the project file tree in `LeftSidebar` (`fileWatcher.RefreshIgnoresAndRescan`).
    -   This ensures that the `IsCustomIgnored` state of files and folders is updated.
    -   Subsequently, if "Use custom rules" is active, the project context (output of Step 1) is also regenerated.
    -   Changes to prompt rules affect `finalPrompt` in Step 2.
-   **`ignore.glob` in the project directory**: The `ignore.glob` file that may exist in the user‑selected project directory is **no longer used** for the "Use custom rules" feature. This feature now relies solely on application‑level configuration. The project's `.gitignore` file continues to be used for the "Use .gitignore rules" feature.