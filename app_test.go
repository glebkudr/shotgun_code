package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitignore "github.com/sabhiram/go-gitignore"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()

	// .gitignore ignoring docs/ and node_modules/
	gitignoreContent := "docs/\nnode_modules/\n*.log\n"
	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(gitignoreContent), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	// Normal files
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	// Normal folder
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "app.js"), []byte("console.log('app');\n"), 0644); err != nil {
		t.Fatalf("failed to write app.js: %v", err)
	}
	// Gitignored log file inside src
	if err := os.WriteFile(filepath.Join(srcDir, "debug.log"), []byte("debug info\n"), 0644); err != nil {
		t.Fatalf("failed to write debug.log: %v", err)
	}
	// Custom rule file inside src
	if err := os.WriteFile(filepath.Join(srcDir, "secret.key"), []byte("SECRET_KEY_DATA\n"), 0644); err != nil {
		t.Fatalf("failed to write secret.key: %v", err)
	}

	// Gitignored directory docs/
	docsDir := filepath.Join(tempDir, "docs")
	docsSubDir := filepath.Join(docsDir, "sub")
	if err := os.MkdirAll(docsSubDir, 0755); err != nil {
		t.Fatalf("failed to create docs/sub dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "readme.md"), []byte("# Docs Readme\n"), 0644); err != nil {
		t.Fatalf("failed to write docs/readme.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsSubDir, "spec.md"), []byte("# Spec\n"), 0644); err != nil {
		t.Fatalf("failed to write docs/sub/spec.md: %v", err)
	}
	// Custom rule file inside gitignored docs/
	if err := os.WriteFile(filepath.Join(docsDir, "credentials.env"), []byte("API_SECRET=12345\n"), 0644); err != nil {
		t.Fatalf("failed to write docs/credentials.env: %v", err)
	}

	return tempDir
}

func findNodeByRelPath(nodes []*FileNode, relPath string) *FileNode {
	for _, n := range nodes {
		if n.RelPath == relPath {
			return n
		}
		if n.IsDir && len(n.Children) > 0 {
			if found := findNodeByRelPath(n.Children, relPath); found != nil {
				return found
			}
		}
	}
	return nil
}

func TestBuildTreeRecursive_SelectiveGitignoreAndCustomPriority(t *testing.T) {
	tempDir := setupTestProject(t)

	gitIgn, err := gitignore.CompileIgnoreFile(filepath.Join(tempDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to compile gitignore: %v", err)
	}

	// Custom ignore rule: *.key and *.env
	customIgn := gitignore.CompileIgnoreLines("*.key", "*.env")

	nodes, err := buildTreeRecursive(context.Background(), tempDir, tempDir, gitIgn, customIgn, 0)
	if err != nil {
		t.Fatalf("buildTreeRecursive failed: %v", err)
	}

	// 1. Verify docs/ is present and has IsGitignored = true
	docsNode := findNodeByRelPath(nodes, "docs")
	if docsNode == nil {
		t.Fatalf("expected docs folder to be present in tree")
	}
	if !docsNode.IsGitignored {
		t.Errorf("expected docs folder to have IsGitignored=true, got false")
	}

	// 2. Verify docs/ has children populated so the user can expand and browse it
	if len(docsNode.Children) == 0 {
		t.Fatalf("expected docs folder to have children populated, got 0")
	}

	// 3. Verify docs/readme.md is present and gitignored
	readmeNode := findNodeByRelPath(nodes, filepath.Join("docs", "readme.md"))
	if readmeNode == nil {
		t.Fatalf("expected docs/readme.md to be present in tree")
	}
	if !readmeNode.IsGitignored {
		t.Errorf("expected docs/readme.md to have IsGitignored=true")
	}

	// 4. Verify docs/sub is present, gitignored, and has children
	subNode := findNodeByRelPath(nodes, filepath.Join("docs", "sub"))
	if subNode == nil {
		t.Fatalf("expected docs/sub to be present in tree")
	}
	if !subNode.IsGitignored {
		t.Errorf("expected docs/sub to have IsGitignored=true")
	}
	if len(subNode.Children) == 0 {
		t.Fatalf("expected docs/sub to have children populated")
	}

	// 5. Verify docs/sub/spec.md is present and gitignored
	specNode := findNodeByRelPath(nodes, filepath.Join("docs", "sub", "spec.md"))
	if specNode == nil {
		t.Fatalf("expected docs/sub/spec.md to be present in tree")
	}
	if !specNode.IsGitignored {
		t.Errorf("expected docs/sub/spec.md to have IsGitignored=true")
	}

	// 6. Verify custom rules priority:
	// docs/credentials.env and src/secret.key match custom rules and MUST NOT be in the tree (completely hidden)
	if secretNode := findNodeByRelPath(nodes, filepath.Join("docs", "credentials.env")); secretNode != nil {
		t.Errorf("expected docs/credentials.env to be hidden by custom rules, but it was found in tree")
	}
	if keyNode := findNodeByRelPath(nodes, filepath.Join("src", "secret.key")); keyNode != nil {
		t.Errorf("expected src/secret.key to be hidden by custom rules, but it was found in tree")
	}

	// 7. Test when custom rules are disabled (customIgn == nil)
	nodesNoCustom, err := buildTreeRecursive(context.Background(), tempDir, tempDir, gitIgn, nil, 0)
	if err != nil {
		t.Fatalf("buildTreeRecursive without custom rules failed: %v", err)
	}
	if secretNode := findNodeByRelPath(nodesNoCustom, filepath.Join("docs", "credentials.env")); secretNode == nil {
		t.Errorf("expected docs/credentials.env to be present when custom rules are disabled")
	}
	if keyNode := findNodeByRelPath(nodesNoCustom, filepath.Join("src", "secret.key")); keyNode == nil {
		t.Errorf("expected src/secret.key to be present when custom rules are disabled")
	}
}

func TestGenerateShotgunOutput_SelectiveInclusionAndCustomPriority(t *testing.T) {
	tempDir := setupTestProject(t)

	app := &App{
		useCustomIgnore:             true,
		currentCustomIgnorePatterns: gitignore.CompileIgnoreLines("*.key", "*.env"),
	}

	t.Run("Selective inclusion of single file in gitignored folder", func(t *testing.T) {
		// Only docs/readme.md is selected (included); docs/sub and all others excluded
		excludedPaths := []string{
			"main.go",
			"src",
			filepath.Join("docs", "sub"),
		}

		output, err := app.generateShotgunOutputWithProgress(context.Background(), tempDir, excludedPaths)
		if err != nil {
			t.Fatalf("generateShotgunOutputWithProgress failed: %v", err)
		}

		if !strings.Contains(output, "readme.md") {
			t.Errorf("expected output to contain readme.md, got:\n%s", output)
		}
		if !strings.Contains(output, "# Docs Readme") {
			t.Errorf("expected output to contain content of readme.md")
		}
		if strings.Contains(output, "spec.md") {
			t.Errorf("expected output NOT to contain spec.md")
		}
		if strings.Contains(output, "main.go") {
			t.Errorf("expected output NOT to contain main.go")
		}
	})

	t.Run("Selective inclusion of subfolder inside gitignored folder", func(t *testing.T) {
		// docs/sub is included; docs/readme.md and others excluded
		excludedPaths := []string{
			"main.go",
			"src",
			filepath.Join("docs", "readme.md"),
		}

		output, err := app.generateShotgunOutputWithProgress(context.Background(), tempDir, excludedPaths)
		if err != nil {
			t.Fatalf("generateShotgunOutputWithProgress failed: %v", err)
		}

		if !strings.Contains(output, "spec.md") {
			t.Errorf("expected output to contain spec.md")
		}
		if !strings.Contains(output, "# Spec") {
			t.Errorf("expected output to contain content of spec.md")
		}
		if strings.Contains(output, "readme.md") {
			t.Errorf("expected output NOT to contain excluded readme.md")
		}
	})

	t.Run("Custom rules absolute priority prevents output even if not in excludedPaths", func(t *testing.T) {
		// Even if excludedPaths is empty (user tried to include everything)
		var excludedPaths []string

		output, err := app.generateShotgunOutputWithProgress(context.Background(), tempDir, excludedPaths)
		if err != nil {
			t.Fatalf("generateShotgunOutputWithProgress failed: %v", err)
		}

		// Custom rule files must NEVER appear in output
		if strings.Contains(output, "secret.key") || strings.Contains(output, "SECRET_KEY_DATA") {
			t.Errorf("custom-ignored secret.key was leaked in shotgun output")
		}
		if strings.Contains(output, "credentials.env") || strings.Contains(output, "API_SECRET=12345") {
			t.Errorf("custom-ignored credentials.env was leaked in shotgun output")
		}
	})
}

func TestCountProcessableItems_RespectsCustomRules(t *testing.T) {
	tempDir := setupTestProject(t)

	app := &App{
		useCustomIgnore:             true,
		currentCustomIgnorePatterns: gitignore.CompileIgnoreLines("*.key", "*.env"),
	}

	countWithCustom, err := app.countProcessableItems(context.Background(), tempDir, map[string]bool{})
	if err != nil {
		t.Fatalf("countProcessableItems failed: %v", err)
	}

	appNoCustom := &App{
		useCustomIgnore: false,
	}
	countNoCustom, err := appNoCustom.countProcessableItems(context.Background(), tempDir, map[string]bool{})
	if err != nil {
		t.Fatalf("countProcessableItems without custom failed: %v", err)
	}

	// countNoCustom should be strictly greater than countWithCustom because secret.key and credentials.env are included
	if countNoCustom <= countWithCustom {
		t.Errorf("expected countNoCustom (%d) > countWithCustom (%d)", countNoCustom, countWithCustom)
	}
}
