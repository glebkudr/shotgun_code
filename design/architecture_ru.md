# Архитектура Shotgun

## 1. Обзор
Shotgun — десктопное приложение на **Wails v2** с фронтендом на **Vue 3**.  
Стек:
- **Backend**: Go (сканирование файлов, построение дерева, генерация контекста, токенизация).
- **Frontend**: Vue + Vite + Tailwind (UI, управление состоянием, подписка на события).
- **Интеграция**: Wails связывает Go‑методы и JS.

Ключевые особенности:
- несколько корневых папок в одном workspace;
- подсчёт токенов **оффлайн** через `o200k_base`;
- рекурсивный обход симлинков с защитой от циклов;
- лимит размера контекста настраивается через `settings.json`;
- авто‑генерация контекста с дебаунсом и очередью.

---

## 2. Backend (Go)

### 2.1 Основные структуры

- **`App`** (см. `app.go`):
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
  - `MaxContextBytes` — лимит размера контекста (по умолчанию 50MB)

- **`FileNode`**:
  - `Name`, `Path`, `RelPath`, `RootPath`, `RootLabel`
  - `SizeBytes`, `TokensApprox`
  - `IsDir`, `Children`
  - `IsGitignored`, `IsCustomIgnored`

---

### 2.2 Токенизация

- Используется **`github.com/tmc/tokencount/openaitokenizer`** с кодировкой `o200k_base`.
- Метод **`CountTokens(text string)`** доступен на фронтенде через Wails.
- Токены считаются:
  - для дерева (`TokensApprox` у `FileNode`);
  - для итогового контекста (`shotgunContextGenerated`).

---

### 2.3 Сканирование файлов и дерево

**`ListFiles(rootDirs []string)`**:
- принимает **несколько корней проекта**;
- для каждого корня компилирует `.gitignore`;
- строит дерево `FileNode` через `buildTreeRecursive`;
- поддерживает **симлинки на директории** с защитой от циклов (`visited` + `resolveDirPath`).

**`computeDirStats`**:
- считает размер/токены для каталогов, в т.ч. игнорируемых;
- учитывает симлинки и циклы.

---

### 2.4 Генерация контекста

**`ContextGenerator`** запускает генерацию в фоне и управляет отменой предыдущих задач.  
Основной путь:
- `RequestShotgunContextGeneration(...)`  
→ `generateShotgunOutputWithProgress(...)`

**`generateShotgunOutputWithProgress`**:
- строит дерево + контент в формате `<file path="...">...</file>`;
- лимитируется `MaxContextBytes` (дефолт 50MB);
- эмитит `shotgunContextGenerationProgress`;
- при успехе отправляет `shotgunContextGenerated` с:
  - `output` (строка),
  - `tokens` (точный подсчёт токенов).

---

### 2.5 Watchman (fsnotify)

`Watchman`:
- следит за изменениями в **нескольких корнях**;
- применяет `.gitignore` и custom rules;
- при изменениях эмитит `projectFilesChanged`.

---

## 3. Frontend (Vue)

### 3.1 Главные компоненты

- **`MainLayout.vue`**
  - основной layout (сайдбар + центральная панель + нижняя консоль);
  - **сайдбар регулируемый по ширине**;
  - обработка `shotgunContextGenerated`, `shotgunContextError`, `shotgunContextGenerationProgress`;
  - авто‑генерация контекста с дебаунсом и очередью:
    - `CONTEXT_GENERATION_DEBOUNCE_MS = 2000`;
    - новые события не отменяют текущую генерацию, а ставятся в очередь.

- **`LeftSidebar.vue`**
  - выбор корней, файл‑дерево, переключатели `.gitignore`/custom rules.

- **`CentralPanel.vue`**
  - рендерит шаги 1–3.

- **`Step1PrepareContext.vue`**
  - прогресс генерации;
  - отображение контекста и **точных токенов**;
  - Repo Scan с токенизацией через `CountTokens`.

- **`Step2ComposePrompt.vue`**
  - ввод задачи, правила, шаблоны промптов;
  - предпросмотр итогового промпта;
  - запуск запросов к LLM.

- **`Step3ExecutePrompt.vue`**
  - история запросов и ответов.

- **`LargeTextViewer.vue`**
  - если нет точного значения, считает токены через `CountTokens`.

---

## 4. Поток данных

1. Пользователь выбирает корни → `ListFiles` строит дерево.  
2. UI обновляет исключения → `RequestShotgunContextGeneration`.  
3. Go генерирует контекст → прогресс + результат (`output` + `tokens`).  
4. UI показывает контекст и токены.  
5. Изменения файлов через `Watchman` инициируют reload дерева и авто‑регенерацию.

---

## 5. Авто‑контекст

Auto‑Context (LLM) использует дерево проекта и выбранную задачу, возвращает список путей.  
UI применяет выбор и запускает регенерацию контекста.

---

## 6. Ограничение размера контекста

Лимит задаётся в `settings.json`:
```json
{ "maxContextBytes": 50000000 }
```
Если параметр не задан или ≤ 0 — используется дефолт 50MB.

---

## 7. UI‑особенности
- **Сайдбар** регулируется по ширине перетаскиванием.
- **Нижняя консоль** регулируется по высоте.
- Токены показываются **точно**, через офлайн‑токенайзер.
