# Shotgun App

![Shotgun App Banner](https://github.com/user-attachments/assets/6dd15389-4ad9-493a-a0e7-9813eb143e38)

> ## 🚀 **New Version Available: [ShotgunPro](https://shotgunpro.dev)**
> 
> We've built an improved version of this application with enhanced features including:
> - **Code Indexing** for faster and more accurate context retrieval
> - **Context Distillation** to optimize your prompts
> - **LLM API Integration** built-in, plus the ability to copy-paste prompts to web LLMs
> 
> **[Try ShotgunPro at https://shotgunpro.dev](https://shotgunpro.dev)**
> 
> *Note: This original version of Shotgun remains fully functional and will continue to be available.*

---

**Tired of Cursor cutting off context, missing your files, and spitting out empty responses?**

**Shotgun** is the bridge between your local codebase and the world's most powerful LLMs.
It doesn't just copy files; it **intelligently packages your project context** and can **execute prompts directly** against OpenAI (GPT-4o/GPT-5), Google Gemini, or OpenRouter.

> **Stop copy-pasting 50 files manually.**
> 1. Select your repo.
> 2. Let AI pick the relevant files (Auto-Context).
> 3. Blast the payload directly to the model or copy it for use in Cursor/Windsurf.

---

## 1. What Shotgun Does
Shotgun is a desktop power-tool that **explodes your project into a structured payload** designed for AI reasoning.

It has evolved from a simple "context dumper" into a full-fledged **LLM Client for Codebases**:
*   **Smart Selection:** Uses AI ("Auto-Context") to analyze your task and automatically select only the relevant files from your tree.
*   **Direct Execution:** Configurable API integration with **OpenAI**, **Gemini**, and **OpenRouter**.
*   **Prompt Engineering:** Built-in templates for different roles (Developer, Architect, Bug Hunter).
*   **History & Audit:** Keeps a full log of every prompt sent and response received.

---

## 2. Key Features

### 🧠 AI-Powered Context
*   **Auto-Context:** Don't know which files are needed for a bug fix? Type your task, and Shotgun uses an LLM to scan your tree and select the relevant files for you.
*   **Repo Scan:** supplement context retrieval with a `shotgun_reposcan.md` summary of your architecture to give the LLM high-level awareness before diving into code.

### ⚡ Workflow Speed
*   **Fast Tree Scan:** Go + Wails backend scans thousands of files in milliseconds.
*   **Interactive Tree:** Manually toggle files/folders or use `.gitignore` and custom rule sets to filter noise.
*   **One-Click Blast:** Generate a massive context payload instantly.

### 🔌 Direct Integrations
*   **OpenAI:** Support for GPT-4o and experimental support for **GPT-5** family models.
*   **Google Gemini:** Native integration for Gemini 2.5/3 Pro & Flash.
*   **OpenRouter:** Access hundreds of LLM's via a unified API.

### 🛠 Developer Experience
*   **Prompt Templates:** Switch modes easily (e.g., "Find Bug" vs "Refactor" vs "Write Docs").
*   **History Tracking:** Never lose a generated patch. Browse past prompts, responses, and raw API payloads.
*   **Privacy Focused:** Your code goes only to the API provider you choose. No intermediate servers.

---

## 3. The Workflow

Shotgun guides you through a 3-step process:

### Step 1: Prepare Context
*   **Select Project:** Open your local repository.
*   **Filter:** Use the checkbox tree, `.gitignore`, or the **Auto-Context** button to define the scope.
*   **Repo Scan:** Edit or load the high-level repository summary for better AI grounding.
*   **Result:** A structured XML-like dump of your selected codebase.

### Step 2: Compose & Execute
*   **Define Task:** Describe what you need (e.g., "Refactor the auth middleware to use JWT").
*   **Select Template:** Choose a persona (Dev, Architect, QA).
*   **Execute:** Click **"Execute Prompt"** to send it to the configured LLM API immediately, OR copy the full payload to your clipboard for use in external tools like ChatGPT or Cursor.

### Step 3: History & Apply
*   **Review:** View the AI's response alongside your original prompt.
*   **Diffs:** The AI output is optimized for `diff` generation.
*   **Audit:** Inspect raw API calls for debugging or token usage analysis.

---

## 4. Installation

### Prerequisites
*   **Go ≥ 1.20**
*   **Node.js LTS**
*   **Wails CLI:** `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### Clone & Build
```bash
git clone https://github.com/glebkudr/shotgun_code
cd shotgun_code

# Install frontend dependencies
cd frontend
npm install
cd ..

# Run in Development Mode (Hot Reload)
wails dev

# Build Production Binary
wails build
```
*Binaries will be located in `build/bin/`.*

---

## 5. Configuration

### LLM Setup
Click the **Settings** (gear icon) in the app to configure providers:
1.  **Provider:** Select OpenAI, Gemini, or OpenRouter.
2.  **API Key:** Paste your key (stored locally).
3.  **Model:** Select your preferred model (e.g., `gpt-4o`, `gemini-2.5-pro`, `claude-3.5-sonnet`).

### Custom Rules
You can define global excludes (like `node_modules`, `dist`, `.git`) and custom prompt instructions that are appended to every request.

---

## 6. Output Format

Shotgun generates context optimized for LLM parsing:

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

This format allows models to understand file boundaries perfectly, enabling accurate multi-file refactoring suggestions.

---

## 7. ⚖️ License & Usage

My name is Gleb Curly, and I am an indie developer making software for a living.

Shotgun is developed and maintained by **Curly's Technology Tmi**. 

This project uses a **Community License** model:

### 1. Free for Small Teams & Non-Commercial Use
You can use Shotgun for free (including modification and internal use) if:
- Your company/team generates **less than $1M USD** in annual revenue.
- You do **not** use the code to build a competing public product.

### 2. Commercial License (Enterprise)
If your annual revenue exceeds **$1M USD**, you are required to purchase a commercial license with a pretty reasonable price.

Please contact me at **glebkudr@gmail.com** for pricing.

See [LICENSE.md](LICENSE.md) for the full legal text.

---

*Shotgun – Load, Aim, Blast your code into the future.*
