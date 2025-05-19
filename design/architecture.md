# Shotgun App Architecture

## 1. Overview

The Shotgun App is a desktop application built using Wails (v2) and Vue.js (v3).
Wails allows for building cross-platform desktop applications with a Go backend and a web-based frontend.

-   **Backend:** Go. Handles file system operations, logic for identifying files/folders to exclude, and generation of the "Shotgun" text output.
-   **Frontend:** Vue.js (with Vite). Provides the user interface for folder selection, displaying the file/folder tree, marking items for exclusion, and showing the generated output.
-   **Communication:** Wails handles the binding between Go functions and JavaScript calls from the frontend.

## 2. Backend (Go)

The Go backend is structured into a `main` package.

### Key Components:

-   **`main.go`**:
    -   Initializes Wails.
    -   Sets up the application window and options.
    -   Binds the `App` struct methods to be callable from the frontend.
    -   Embeds the frontend assets.
-   **`app.go`**:
    -   **`App` struct**: Holds application state or context if needed (e.g., `context.Context`).
    -   **`startup(ctx context.Context)`**: Wails lifecycle hook, stores the context.
    -   **`FileNode` struct**: Represents a file or folder in the tree. Includes `Name`, `Path`, `RelPath`, `IsDir`, `Children`, and `Excluded` (for UI state).
    -   **`ListFiles(dirPath string) ([]*FileNode, error)`**:
        -   Takes a directory path as input.
        -   Creates a single root `FileNode` representing `dirPath` itself.
        -   The `Name` of this root node is the base name of `dirPath`.
        -   The `RelPath` of this root node is `"."`.
        -   `IsDir` for this root node is `true`.
        -   Its `Children` are populated by recursively scanning the `dirPath`.
        -   This recursive scan builds a tree structure of `FileNode` objects for the contents of `dirPath`.
        -   Uses `os.ReadDir` and `path/filepath` for file system interaction.
        -   Sorts entries (directories first, then by name).
        -   Returns a slice containing only the single root `FileNode` to the frontend.
    -   **`GenerateShotgunOutput(rootDir string, excludedPaths []string) (string, error)`**:
        -   Takes the root directory path and a list of relative paths to exclude.
        -   **Tree Generation**:
            -   Recursively traverses the `rootDir`.
            -   Skips any file or folder whose relative path is in `excludedPaths`. If a folder is excluded, its children are also skipped.
            -   Builds a textual tree representation (e.g., using `├──`, `└──`, `│`).
        -   **File Content Aggregation**:
            -   For each non-excluded file:
                -   Reads its content.
                -   Formats it using an XML-like structure: `<file path="path/to/file.ext">\n[file_content]\n</file>`.
                -   The `name` attribute contains the path relative to `rootDir`, using forward slashes.
        -   **Final Output**:
            -   Concatenates the tree structure, a newline, and all formatted file contents.
            -   Returns the complete string.

    -   **`Watchman` (File System Watcher)**:
        -   A component responsible for monitoring file system changes within the selected project directory.
        -   **`Watchman` struct**: Holds the application context, root directory being watched, a ticker for periodic checks, the last known state of files, a mutex for synchronization, and a cancel function for its goroutine.
        -   **`fileMeta` struct**: Stores metadata for each file/directory (ModTime, Size, IsDir) for comparison against the last known state.
        -   **`NewWatchman(app *App)`**: Constructor for the `Watchman`.
        -   **`App.StartFileWatcher(rootDirPath string) error` / `App.StopFileWatcher() error`**: Methods exposed to the frontend via Wails. These are called by the frontend to start and stop the file watcher for a given `rootDirPath`. They delegate to the `Watchman` instance.
        -   **`Watchman.Start(newRootDir string)`**: Initializes and starts the watching process. It stops any previous watcher, performs an initial scan of `newRootDir` using `scanDirectoryState` to establish `lastKnownState`, and then launches the `run` method in a new goroutine.
        -   **`Watchman.Stop()`**: Stops the current watcher by calling its `cancelFunc` (which cancels the context for the `run` goroutine), stops the ticker, and clears internal state like `rootDir` and `lastKnownState`.
        -   **`Watchman.run(ctx context.Context)`**: The main monitoring loop, running in its own goroutine. It uses a `time.Ticker` (e.g., every 200ms) to periodically:
            -   Check if the context has been cancelled (e.g., by `Stop()`).
            -   Call `scanDirectoryState` to get the current state of files and directories.
            -   Call `compareStates` to check for differences against `lastKnownState`.
            -   If changes are detected, it calls `App.notifyFileChange` and updates `lastKnownState`.
        -   **`Watchman.scanDirectoryState(scanRootDir string)`**: Recursively walks the `scanRootDir` (using `filepath.WalkDir`), collecting `fileMeta` for each file and directory. It calculates paths relative to `scanRootDir`. It includes logic to skip certain directories (e.g., `.git` at the top level of the scanned root) and handles errors during traversal by logging them and attempting to continue.
        -   **`Watchman.compareStates(oldState, newState map[string]fileMeta)`**: Compares the `newState` with the `oldState`. It checks for:
            -   Differences in the number of items (creations/deletions).
            -   Items present in `newState` but not `oldState` (creations).
            -   For items present in both, it checks for changes in `IsDir` status, `Size`, or `ModTime`.
            Returns `true` if any change is detected.
        -   **`App.notifyFileChange(rootDir string)`**: If `compareStates` detects any changes, this method is called on the `App` struct. It emits a `projectFilesChanged` Wails event to the frontend, passing the `rootDir` that experienced changes.
        -   The frontend listens to this `projectFilesChanged` event to trigger actions like queuing a reload of the file tree (`FileTree.vue`) and, subsequently, the project context if the system is not busy (e.g., not already loading the tree or generating context).

### Wails Integration:

-   The `App` struct and its public methods (`ListFiles`, `GenerateShotgunOutput`) are bound using `wails.Run(&options.App{Bind: ...})`.
-   Wails automatically generates JavaScript bindings in `frontend/wailsjs/go/main/App.js` for these methods, allowing the frontend to call them.
-   For folder selection, the Go backend can expose a method that uses `runtime.OpenDirectoryDialog` from the Wails runtime package. This method is then bound and called from the frontend.

## 3. Frontend (Vue.js)

The frontend is a Single Page Application (SPA) built with Vue 3 (Composition API), Vite, and Tailwind CSS.
It implements a multi-step user interface to guide the user through the process of preparing project context, composing prompts, executing them, and applying patches.

### Key Components:

-   **`main.js`**: Entry point for the Vue application. Initializes Vue and mounts the root `App.vue` component.
-   **`App.vue`**: Root component.
    -   Mounts the `MainLayout.vue` component, which orchestrates the overall UI.
-   **`components/MainLayout.vue`**:
    -   **Overall Structure**: Manages the main layout consisting of a horizontal stepper, a left sidebar, a central content panel, and a bottom console.
    -   **State Management (using `ref` and `reactive`)**:
        -   `currentStep`: Tracks the active step in the multi-step process (1-4).
        -   `steps`: An array of step objects, each with an `id`, `title`, and `completed` status.
        -   `logMessages`: Array for storing messages to be displayed in the `BottomConsole`.
    -   **Core Logic**:
        -   Handles navigation between steps via `navigateToStep`.
        -   Manages actions triggered from step components via `handleStepAction`, orchestrating the flow between steps and simulating backend interactions.
        -   Updates step completion status.
-   **`components/HorizontalStepper.vue`**:
    -   Displays the steps (1-4) horizontally at the top of the application.
    -   Highlights the current step and marks completed steps.
    -   Allows navigation to completed steps or the next uncompleted step.
-   **`components/LeftSidebar.vue`**:
    -   A persistent sidebar on the left.
    -   Contains a placeholder for a `FileTree.vue` component (for displaying project file structure).
    -   Displays a list of steps (e.g., "1. Prepare Context", "2. Compose Prompt"), allowing navigation similar to the `HorizontalStepper`.
-   **`components/CentralPanel.vue`**:
    -   The main content area that dynamically renders the component for the current active step.
    -   Uses `v-if` directives based on `currentStep` to show one of the following step-specific components:
        -   `components/steps/Step1CopyStructure.vue` (handles "Prepare Context")
        -   `components/steps/Step2GenerateDiff.vue` (handles "Compose Prompt")
        -   `components/steps/Step3ExecuteDiff.vue` (handles "Execute Prompt")
        -   `components/steps/Step4ApplyPatch.vue`
    -   Forwards actions from step components to `MainLayout.vue`.
    -   Exposes methods like `updateStep2DiffOutput` and `addLogToStep3Console` to allow `MainLayout.vue` to push data into specific step components (e.g., diff output into Step 2, logs into Step 3's console).
-   **`components/steps/Step1CopyStructure.vue`** (handles "Prepare Context"):
    -   UI for the first step.
    -   Contains a "Prepare Project Context & Proceed" button.
    -   Emits an action (e.g., `prepareContext`) to `MainLayout.vue` to signify completion and move to the next step.
-   **`components/steps/Step2GenerateDiff.vue`** (handles "Compose Prompt"):
    -   UI for the second step.
    -   Includes a `<textarea>` for the user to input their prompt for the LLM.
    -   A "Compose Prompt" button that triggers an action (e.g., `composePrompt`) with the prompt text.
    -   A `<pre>` block to display the diff output received from `MainLayout.vue`.
-   **`components/steps/Step3ExecuteDiff.vue`** (handles "Execute Prompt"):
    -   UI for the third step.
    -   A "Execute Prompt" button (e.g., `executePrompt`).
-   **`components/steps/Step4ApplyPatch.vue`**:
    -   UI for the fourth step.
    -   A placeholder for an interactive patch editor (shows stubbed hunks with checkboxes).
    -   "Apply Selected" and "Apply All & Finish" buttons.
-   **`components/BottomConsole.vue`**:
    -   A console area at the bottom of the application.
    -   Displays general execution status logs passed from `MainLayout.vue`.
    -   Typically visible from Step 3 onwards.
-   **`components/FileTree.vue`**: (Currently a stub in `LeftSidebar.vue`, based on previous architecture)
    -   Intended to be a recursive component to display the file/folder tree.
    -   Props: `nodes` (array of `FileNode`), `projectRoot`.
    -   Will allow users to view the project structure. (Interaction for exclusion marking was part of the previous design and may be re-integrated or adapted).
-   **`assets/main.css`**: Includes Tailwind CSS directives and minimal global styles.
-   **`tailwind.config.js` & `postcss.config.js`**: Configuration for Tailwind CSS.
-   **`index.html`**: Main HTML file.
-   **`vite.config.js`**: Vite build configuration.
-   **`package.json`**: Frontend project metadata and dependencies (Vue, Vite, Tailwind CSS).

### Wails Integration:

-   (As per previous architecture) Frontend calls Go methods via `import { MethodName } from '../../wailsjs/go/main/App';`.
-   The Go backend methods like `ListFiles` and `GenerateShotgunOutput` (and a potential `SelectDirectory` wrapper) are still relevant for providing data and performing core operations. The new UI will now trigger these at different stages of its multi-step process. For example, `ListFiles` might be called after an initial project selection step (implicitly part of or preceding Step 1), and `GenerateShotgunOutput` (or a similar method for LLM interaction) would be relevant around Step 2/3.

## 4. Data Flow & Application Logic (Multi-Step UI)

The application operates based on a sequence of steps, managed by `MainLayout.vue`:

1.  **Initialization (Step 1: Prepare Context)**:
    -   The application starts at Step 1.
    -   `CentralPanel.vue` displays `Step1CopyStructure.vue`.
    -   User Action: Clicks "Prepare Project Context & Proceed".
    -   `Step1CopyStructure.vue` emits an `action` (e.g., `prepareContext`) to `MainLayout.vue`.
    -   `MainLayout.vue` handles the action:
        -   Simulates a backend call (e.g., `GenerateShotgunOutput` to prepare the context).
        -   Logs the action to `BottomConsole.vue` (if visible) and potentially to a step-specific console.
        -   Marks Step 1 as completed.
        -   Advances `currentStep` to 2.

2.  **Step 2: Compose Prompt**:
    -   `CentralPanel.vue` now displays `Step2GenerateDiff.vue`.
    -   User Action: Enters a prompt in the textarea and clicks "Compose Prompt".
    -   `Step2GenerateDiff.vue` emits an `action` (e.g., `composePrompt`) with the prompt payload to `MainLayout.vue`.
    -   `MainLayout.vue` handles the action:
        -   Simulates a backend call to an LLM (this would involve sending the prompt and context to an LLM to get a diff).
        -   Receives a mock diff output.
        -   Calls `centralPanelRef.value.updateStep2DiffOutput(mockDiff)` to send the diff to `Step2GenerateDiff.vue` for display.
        -   Logs the action.
        -   Marks Step 2 as completed.
        -   Advances `currentStep` to 3.

3.  **Step 3: Execute Prompt**:
    -   `CentralPanel.vue` displays `Step3ExecuteDiff.vue`.
    -   `BottomConsole.vue` becomes visible (or more active).
    -   User Action: Clicks "Execute Prompt".
    -   `Step3ExecuteDiff.vue` emits an `action` (e.g., `executePrompt`) to `MainLayout.vue`.
    -   `MainLayout.vue` handles the action:
        -   Simulates the execution of the diff (e.g., applying changes in memory or preparing for a patch, this is conceptual for "executing the prompt's intent").
        -   Sends logs specifically to `Step3ExecuteDiff.vue`'s console via `centralPanelRef.value.addLogToStep3Console(message, type)`.
        -   Also sends general logs to `BottomConsole.vue`.
        -   Marks Step 3 as completed.
        -   Advances `currentStep` to 4.

4.  **Step 4: Apply Patch**:
    -   `CentralPanel.vue` displays `Step4ApplyPatch.vue`.
    -   User Action: Interacts with the (stubbed) patch editor (e.g., selecting hunks) and clicks "Apply Selected" or "Apply All & Finish".
    -   `Step4ApplyPatch.vue` emits an `action` (e.g., `applySelectedPatches` or `applyAllPatches`) to `MainLayout.vue`.
    -   `MainLayout.vue` handles the action:
        -   Simulates applying the patches to the file system.
        -   Logs the final actions.
        -   Marks Step 4 as completed.
        -   The process might conclude, or allow for further iterations/restarts.

**Navigation & State**:
-   `HorizontalStepper.vue` and `LeftSidebar.vue` allow navigation between steps. Users can typically go back to completed steps or forward to the next uncompleted step.
-   The `completed` status of each step in `MainLayout.vue`'s `steps` ref controls navigability and visual state.
-   The `FileTree.vue` component in the `LeftSidebar.vue` is intended to display the project's file structure. Its interaction with the steps (e.g., updating after a patch is applied in Step 4) will be a key part of future development.

## Asynchronous Project Context Generation

The project context, which is the text output displayed in the "Prepare Context" step (Step 1), is now generated asynchronously to improve user experience and UI responsiveness. The key aspects of this implementation are:

1.  **Automatic Regeneration**: Whenever there are relevant changes in the selected project's file and folder tree (e.g., toggling exclusions, changing ignore rules, or selecting a new project directory), the project context is automatically regenerated in a background goroutine on the Go side. This ensures that the UI does not freeze during potentially long operations.

2.  **Loading Indication**: While the context is being generated, the frontend displays a visual loading indicator (e.g., a spinner) in the area where the context text will appear. This informs the user that an operation is in progress.

3.  **Job Cancellation and Debouncing**: If multiple changes occur in quick succession, the system employs a debouncing mechanism. This means that a new generation job is not started for every single change. Instead, it waits for a short period of inactivity before triggering the generation. If new changes occur while a generation job is already running, the ongoing job is cancelled, and a new one is started with the latest state of the file tree and exclusion rules. This prevents unnecessary computation and ensures the final output reflects the most recent user selections.

4.  **Elimination of Manual Trigger**: Due to the automatic and reactive nature of context generation, the manual "Prepare Project Context & Proceed" button has been removed from Step 1. The context is always kept up-to-date or is in the process of being updated.

This asynchronous approach ensures that the application remains interactive and provides a smoother experience, especially when working with large project structures.

## Enhanced Context Generation and UI Feedback

The asynchronous project context generation has been further enhanced with progress reporting and output size limits to improve user experience and application stability.

### 1. Progress Reporting

To provide better feedback during long-running context generation, a progress reporting mechanism has been implemented:

-   **Backend (Go - `app.go`):**
    -   A new function `countProcessableItems` recursively traverses the project directory (respecting exclusions) to estimate the total number of items (directories, files to list, files to read content from) before actual generation begins.
    -   During context generation (`generateShotgunOutputWithProgress`):
        -   A `generationProgressState` struct tracks `processedItems` and `totalItems`.
        -   The `emitProgress` function is called periodically (after processing the root directory line, each tree entry, and each file content) to send a `shotgunContextGenerationProgress` Wails event.
        -   This event carries a payload like `{ "current": X, "total": Y }`.
-   **Frontend (Vue.js):**
    -   **`MainLayout.vue`**:
        -   Listens for the `shotgunContextGenerationProgress` Wails event.
        -   Updates a reactive `generationProgressData` ref (`{ current: 0, total: 0 }`).
        -   Resets `generationProgressData` before each new generation request.
    -   **`CentralPanel.vue`**:
        -   Receives `generationProgressData` as a prop.
        -   Passes it down to `Step1PrepareContext.vue`.
    -   **`steps/Step1PrepareContext.vue`**:
        -   Receives `generationProgress` as a prop.
        -   Displays a progress bar instead of a generic spinner.
        -   The progress bar's width is computed based on `generationProgress.current` and `generationProgress.total`.
        -   Displays text like "X / Y items" or "X / calculating..." if total is not yet known.

### 2. Output Size Limitation

To prevent excessive memory usage and overly large context outputs that might be problematic for LLMs or UI rendering:

-   **Backend (Go - `app.go`):**
    -   A constant `maxOutputSizeBytes` (e.g., 1MB) defines the maximum allowed size for the generated context string.
    -   A custom error `ErrContextTooLong` is defined.
    -   The `generateShotgunOutputWithProgress` function checks the accumulated output size at various stages (after adding the root directory, each tree line, and before/after appending file content).
    -   If the `maxOutputSizeBytes` is exceeded, the generation is halted, and `ErrContextTooLong` (wrapped with more details) is returned.
    -   This error is then emitted via the `shotgunContextError` Wails event to the frontend.
-   **Frontend (Vue.js):**
    -   The error message, including "context is too long", will be displayed in `Step1PrepareContext.vue` as part of the standard error handling for `shotgunContextError`.

These enhancements provide a more transparent and robust context generation process.

## 5. Cross-Platform Considerations

-   **Wails**: Natively supports building for Windows, macOS, and Linux from a single codebase.
-   **Go**: Standard library functions like `os` and `path/filepath` are cross-platform, handling path separators and OS-specific details.
-   **Frontend**: Web technologies (HTML, CSS, JS) are inherently cross-platform.

## 6. Simplicity and Minimal Libraries

-   **Go Backend**: Relies primarily on the Go standard library. Wails is the main external dependency.
-   **Vue Frontend**: Uses Vue 3, Vite, and Tailwind CSS. The UI components for the stepper, panels, and steps are custom built.
-   This approach aims to keep the application maintainable and focused, leveraging Tailwind CSS for styling efficiency.

## 7. Configuration Management and Custom Ignore Rules

-   **Configuration Storage**: Application settings, including custom ignore rules, are stored in a JSON file (`settings.json`) within the user's standard configuration directory (e.g., `~/.config/shotgun-code/settings.json` on Linux, `%APPDATA%\Shotgun Code\Config\settings.json` on Windows). This is managed using the `github.com/adrg/xdg` library.
-   **Default Custom Rules**: The `ignore.glob` file located at the root of the application's repository serves as the source for default custom ignore rules. These rules are embedded into the application binary at build time using Go's `embed` package.
-   **Loading Rules**: On startup, the application attempts to load `settings.json`. 
    - If the file exists and contains valid `customIgnoreRules`, these are used.
    - If the file doesn't exist, or if `customIgnoreRules` are empty/missing or invalid, the embedded default rules are used. The application will also attempt to create/update `settings.json` with these defaults (or the last valid loaded rules if applicable).
-   **Editing Custom Rules**: 
    - A "gear" icon (⚙️) next to the "Use custom rules" checkbox in the `LeftSidebar` opens a modal.
    - This modal allows users to view and edit the custom ignore rules in a textarea. The rules follow `.gitignore` pattern syntax.
    - Saving these rules updates the `settings.json` file. The application then recompiles these rules for internal use.
-   **Impact of Rule Changes**: 
    - Any modification to the custom ignore rules (via the modal and save) triggers a reload of the project file tree in the `LeftSidebar`.
    - This ensures that the `IsCustomIgnored` status of files and folders is updated according to the new rules.
    - Subsequently, if "Use custom rules" is active, the project context (Step 1 output) will also be regenerated to reflect these changes.
-   **`ignore.glob` in Project Directory**: The `ignore.glob` file that might exist within a user's selected project directory is **no longer used** for the "Use custom rules" feature. This feature now exclusively relies on the application-level configuration described above. The `.gitignore` file in the project directory continues to be used for the "Use .gitignore rules" feature.