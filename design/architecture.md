# Shotgun Architecture

## 1. Overview
Shotgun is a desktop application built with **Wails v2** and a **Vue 3** frontend.  
Stack:
- **Backend**: Go (file scanning, tree building, context generation, tokenization).
- **Frontend**: Vue + Vite + Tailwind (UI, state, event handling).
- **Integration**: Wails bridges Go methods to JS.

Key capabilities:
- multiple project roots in a single workspace;
- **offline** token counting via `o200k_base`;
- recursive symlink traversal with cycle protection;
- context size limit configured via `settings.json`;
- auto generation with debounce + queue.

---

## 2. Backend (Go)

### 2.1 Core structures

- **`App`** (see `app.go`):
  - `contextGenerator *ContextGenerator`
  - `fileWatcher *Watchman`
  - `settings AppSettings`
  - `projectGitignores map[string]*gitignore.GitIgnore`
  - `tokenEncoder *openaitokenizer.Encoder`

- **`AppSettings`**:
  - `CustomIgnoreRules`
  - `CustomPromptRules`
  - `LLMSettings`
  - `ProjectRoots`
  - `MaxContextBytes` — context size limit (default 50MB)

- **`FileNode`**:
  - `Name`, `Path`, `RelPath`, `RootPath`, `RootLabel`
  - `SizeBytes`, `TokensApprox`
  - `IsDir`, `Children`
  - `IsGitignored`, `IsCustomIgnored`

---

### 2.2 Tokenization

- Uses **`github.com/tmc/tokencount/openaitokenizer`** with `o200k_base`.
- **`CountTokens(text string)`** is exposed to the frontend via Wails.
- Token counts are used for:
  - the tree (`TokensApprox` in `FileNode`);
  - the generated context (`shotgunContextGenerated`).

---

### 2.3 File scanning & tree

**`ListFiles(rootDirs []string)`**:
- accepts **multiple project roots**;
- compiles `.gitignore` per root;
- builds `FileNode` tree via `buildTreeRecursive`;
- supports **symlinked directories** with cycle protection (`visited` + `resolveDirPath`).

**`computeDirStats`**:
- aggregates size/tokens for directories, including ignored ones;
- respects symlinks and cycle protection.

---

### 2.4 Context generation

**`ContextGenerator`** runs generation in background and manages cancellation.  
Flow:
- `RequestShotgunContextGeneration(...)`  
→ `generateShotgunOutputWithProgress(...)`

**`generateShotgunOutputWithProgress`**:
- builds tree + file contents in `<file path="...">...</file>` format;
- enforces `MaxContextBytes` (default 50MB);
- emits `shotgunContextGenerationProgress`;
- on success emits `shotgunContextGenerated` with:
  - `output` (string),
  - `tokens` (exact token count).

---

### 2.5 Watchman (fsnotify)

`Watchman`:
- watches **multiple roots**;
- respects `.gitignore` and custom rules;
- emits `projectFilesChanged` on changes.

---

## 3. Frontend (Vue)

### 3.1 Main components

- **`MainLayout.vue`**
  - main layout (sidebar + central panel + bottom console);
  - **resizable sidebar**;
  - handles `shotgunContextGenerated`, `shotgunContextError`, `shotgunContextGenerationProgress`;
  - auto context generation with debounce + queue:
    - `CONTEXT_GENERATION_DEBOUNCE_MS = 2000`;
    - new changes do not cancel the current generation, they are queued.

- **`LeftSidebar.vue`**
  - roots selection, file tree, `.gitignore`/custom rules toggles.

- **`CentralPanel.vue`**
  - renders steps 1–3.

- **`Step1PrepareContext.vue`**
  - progress bar;
  - context output with **exact tokens**;
  - Repo Scan tokenized via `CountTokens`.

- **`Step2ComposePrompt.vue`**
  - task input, rules, prompt templates;
  - prompt preview;
  - LLM execution.

- **`Step3ExecutePrompt.vue`**
  - history of prompts/responses.

- **`LargeTextViewer.vue`**
  - if exact tokens are not provided, uses `CountTokens`.

---

## 4. Data flow

1. User selects roots → `ListFiles` builds the tree.  
2. UI updates exclusions → `RequestShotgunContextGeneration`.  
3. Go generates context → progress + result (`output` + `tokens`).  
4. UI renders context and tokens.  
5. File changes via `Watchman` trigger tree reload + auto regeneration.

---

## 5. Auto‑Context

Auto‑Context uses an LLM to select paths based on the task.  
UI applies the selection and triggers regeneration.

---

## 6. Context size limit

Configured in `settings.json`:
```json
{ "maxContextBytes": 50000000 }
```
If missing or ≤ 0, the default 50MB is used.

---

## 7. UI notes
- **Resizable sidebar** (drag to resize).
- **Resizable bottom console**.
- Token counts are **exact**, via offline tokenizer.
