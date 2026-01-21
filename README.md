# Shotgun App

![Shotgun App Banner](https://github.com/user-attachments/assets/6dd15389-4ad9-493a-a0e7-9813eb143e38)

Shotgun is a desktop app for preparing project context and working with LLMs.  
It collects selected files into a structured context, can auto‑select relevant files, and can send prompts directly to OpenAI, Gemini, or OpenRouter.

---

## What Shotgun Does
Shotgun turns your repository into an LLM‑friendly context:
- builds a file tree and collects file contents;
- respects `.gitignore` and custom rules;
- shows size and token counts per file/folder;
- outputs XML‑like blocks `<file path="...">...</file>`.

---

## Key Features

### 🧠 Auto Context
- **Auto‑Context**: the LLM selects relevant files based on your task.
- **Repo Scan**: optional `shotgun_reposcan.md` for high‑level architecture context.

### ⚡ Large‑project performance
- **Multiple project roots**: add several root folders in one workspace.
- **Project tree** with manual selection.
- **Symlink traversal** with cycle protection.
- **Offline token counting** (`o200k_base`) without API calls.
- **Context size limit** via `settings.json` (`maxContextBytes`, default 50MB).
- **Auto context generation** with debounce + queue (no constant restarts).

### 🔌 Integrations
Supported providers:
- OpenAI
- Google Gemini
- OpenRouter

You can pick a model and run requests directly from the app.

---

## Workflow

### Step 1: Prepare Context
1. Add one or more project roots.
2. Select files manually or use Auto‑Context.
3. Context is generated automatically with progress.
4. **Generated Project Context** shows an exact token count.

### Step 2: Compose & Execute
1. Describe your task.
2. Choose a prompt template.
3. Execute the request or copy the prompt.

### Step 3: History
History of requests, responses, and call parameters.

---

## UI
- **Resizable sidebar** (drag to resize).
- **Resizable bottom console**.

---

## Installation & Run

### Requirements
- **Go ≥ 1.20**
- **Node.js LTS**
- **Wails CLI**  
  `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### Build & Run
```bash
git clone https://github.com/glebkudr/shotgun_code
cd shotgun_code

# Install frontend dependencies
cd frontend
npm install
cd ..

# Dev mode (hot reload)
wails dev

# Production build
wails build
```
Binaries are placed in `build/bin/`.

---

## Configuration

### Settings file
Uses the XDG path `shotgun-code/settings.json`.  
On Linux this is typically:
```
~/.config/shotgun-code/settings.json
```

Example:
```json
{
  "maxContextBytes": 50000000,
  "customIgnoreRules": "...",
  "customPromptRules": "..."
}
```

### Key options
- **`maxContextBytes`** — context size limit (default 50MB).
- **`customIgnoreRules`** — file exclusion rules.
- **`customPromptRules`** — global prompt rules.

---

## Output Format

```xml
<file path="backend/main.go">
package main
...
</file>

<file path="frontend/src/App.vue">
<template>
...
</template>
</file>
```

---

## License

Shotgun is developed and maintained by **Curly's Technology Tmi**.

### 1. Free for small teams and non‑commercial use
You may use Shotgun for free if:
- your annual revenue is **below $1M**;
- you do **not** use the code to build a competing product.

### 2. Commercial license
If your revenue exceeds $1M, a commercial license is required.  
Contact: **glebkudr@gmail.com**  
See `LICENSE.md` for details.

---

*Shotgun — Load, Aim, Blast your code into the future.*
