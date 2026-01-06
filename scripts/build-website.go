// Build script for ooo website
// Generates the samples section from the samples directory
//
// Usage: go run scripts/build-website.go
package main

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Sample metadata
type Sample struct {
	Folder      string
	Title       string
	Description string
	Files       []string // List of .go files (empty means single main.go)
}

const githubIcon = `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>`

const copyIcon = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>`

func main() {
	projectRoot := findProjectRoot()
	samplesDir := filepath.Join(projectRoot, "samples")
	indexFile := filepath.Join(projectRoot, "public", "index.html")

	// Discover samples dynamically
	samples, err := discoverSamples(samplesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering samples: %v\n", err)
		os.Exit(1)
	}

	// Read current index.html
	content, err := os.ReadFile(indexFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", indexFile, err)
		os.Exit(1)
	}

	// Generate samples HTML
	var samplesHTML bytes.Buffer
	for _, sample := range samples {
		html, err := generateSampleCard(samplesDir, sample)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			continue
		}
		samplesHTML.WriteString(html)
	}

	// Replace content between markers
	startMarker := "<!-- SAMPLES_START -->"
	endMarker := "<!-- SAMPLES_END -->"

	startIdx := bytes.Index(content, []byte(startMarker))
	endIdx := bytes.Index(content, []byte(endMarker))

	if startIdx == -1 || endIdx == -1 {
		fmt.Fprintf(os.Stderr, "Error: Could not find %s and %s markers in %s\n", startMarker, endMarker, indexFile)
		os.Exit(1)
	}

	// Build new content
	var newContent bytes.Buffer
	newContent.Write(content[:startIdx+len(startMarker)])
	newContent.WriteString("\n")
	newContent.Write(samplesHTML.Bytes())
	newContent.Write(content[endIdx:])

	// Write back
	if err := os.WriteFile(indexFile, newContent.Bytes(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", indexFile, err)
		os.Exit(1)
	}

	fmt.Printf("Updated %s with %d samples\n", indexFile, len(samples))
}

func findProjectRoot() string {
	// Try to find project root from current directory or script location
	candidates := []string{
		".",
		"..",
		"/root/go/src/github.com/benitogf/ooo",
	}

	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "samples")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "public", "index.html")); err == nil {
				abs, _ := filepath.Abs(dir)
				return abs
			}
		}
	}

	fmt.Fprintln(os.Stderr, "Error: Could not find project root")
	os.Exit(1)
	return ""
}

// discoverSamples finds all sample directories and reads their metadata from README.md
func discoverSamples(samplesDir string) ([]Sample, error) {
	entries, err := os.ReadDir(samplesDir)
	if err != nil {
		return nil, err
	}

	var samples []Sample
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		folderName := entry.Name()
		readmeFile := filepath.Join(samplesDir, folderName, "README.md")

		// Find all .go files in the sample directory
		goFiles, err := findGoFiles(filepath.Join(samplesDir, folderName))
		if err != nil || len(goFiles) == 0 {
			continue
		}

		// Read title and description from README.md
		title, description := parseReadme(readmeFile, folderName)

		samples = append(samples, Sample{
			Folder:      folderName,
			Title:       title,
			Description: description,
			Files:       goFiles,
		})
	}

	// Sort by folder name (which starts with number)
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Folder < samples[j].Folder
	})

	return samples, nil
}

// findGoFiles returns all .go files in a directory, sorted
func findGoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, entry.Name())
		}
	}

	sort.Strings(files)
	return files, nil
}

// parseReadme extracts title and description from a README.md file
// Expected format:
// # Title
//
// Description text
func parseReadme(readmeFile, folderName string) (title, description string) {
	// Default title from folder name
	title = formatFolderName(folderName)
	description = "Sample code"

	content, err := os.ReadFile(readmeFile)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			// Extract title without the # prefix
			rawTitle := strings.TrimPrefix(line, "# ")
			// Prepend folder number
			parts := strings.SplitN(folderName, "_", 2)
			if len(parts) > 0 {
				title = parts[0] + " - " + rawTitle
			} else {
				title = rawTitle
			}
			// Look for description in next non-empty line
			for j := i + 1; j < len(lines); j++ {
				descLine := strings.TrimSpace(lines[j])
				if descLine != "" && !strings.HasPrefix(descLine, "#") {
					description = descLine
					break
				}
			}
			break
		}
	}

	return
}

// formatFolderName converts folder name like "01_basic_server" to "01 - Basic Server"
func formatFolderName(name string) string {
	parts := strings.SplitN(name, "_", 2)
	if len(parts) < 2 {
		return name
	}
	// Convert snake_case to Title Case
	words := strings.Split(parts[1], "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return parts[0] + " - " + strings.Join(words, " ")
}

func generateSampleCard(samplesDir string, sample Sample) (string, error) {
	// Check if this is a multi-file sample
	if len(sample.Files) > 1 {
		return generateMultiFileSampleCard(samplesDir, sample)
	}

	// Single file sample (original behavior)
	fileName := "main.go"
	if len(sample.Files) == 1 {
		fileName = sample.Files[0]
	}

	filePath := filepath.Join(samplesDir, sample.Folder, fileName)
	code, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("could not read %s: %v", filePath, err)
	}

	// Remove build directives and package comment lines at the top
	codeStr := filterCode(string(code))

	// Apply syntax highlighting
	highlighted := highlightGo(codeStr)

	return fmt.Sprintf(`      <div class="sample-card">
        <div class="sample-header" onclick="this.parentElement.classList.toggle('open')">
          <div class="sample-info">
            <h4>%s</h4>
            <p>%s</p>
          </div>
          <div class="sample-links">
            <a href="https://github.com/benitogf/ooo/tree/master/samples/%s" target="_blank" onclick="event.stopPropagation()">
              %s
              GitHub
            </a>
          </div>
          <span class="sample-toggle">▼</span>
        </div>
        <div class="sample-code">
          <div class="code-header">
            <span class="code-dot red"></span>
            <span class="code-dot yellow"></span>
            <span class="code-dot green"></span>
            <span class="code-title">%s</span>
            <button class="copy-btn" onclick="copyCode(this)">
              %s
              Copy
            </button>
          </div>
          <pre><code>%s</code></pre>
        </div>
      </div>

`, sample.Title, sample.Description, sample.Folder, githubIcon, fileName, copyIcon, highlighted), nil
}

func generateMultiFileSampleCard(samplesDir string, sample Sample) (string, error) {
	var filesHTML bytes.Buffer

	for _, fileName := range sample.Files {
		filePath := filepath.Join(samplesDir, sample.Folder, fileName)
		code, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("could not read %s: %v", filePath, err)
		}

		codeStr := filterCode(string(code))
		highlighted := highlightGo(codeStr)

		filesHTML.WriteString(fmt.Sprintf(`          <div class="sample-file">
            <div class="sample-file-header" onclick="this.parentElement.classList.toggle('file-open')">
              <span class="sample-file-name">%s</span>
              <span class="sample-file-toggle">▼</span>
            </div>
            <div class="sample-file-code">
              <div class="code-header">
                <span class="code-dot red"></span>
                <span class="code-dot yellow"></span>
                <span class="code-dot green"></span>
                <span class="code-title">%s</span>
                <button class="copy-btn" onclick="copyCode(this)">
                  %s
                  Copy
                </button>
              </div>
              <pre><code>%s</code></pre>
            </div>
          </div>
`, fileName, fileName, copyIcon, highlighted))
	}

	return fmt.Sprintf(`      <div class="sample-card multi-file">
        <div class="sample-header" onclick="this.parentElement.classList.toggle('open')">
          <div class="sample-info">
            <h4>%s</h4>
            <p>%s</p>
          </div>
          <div class="sample-links">
            <a href="https://github.com/benitogf/ooo/tree/master/samples/%s" target="_blank" onclick="event.stopPropagation()">
              %s
              GitHub
            </a>
          </div>
          <span class="sample-toggle">▼</span>
        </div>
        <div class="sample-code sample-files">
%s        </div>
      </div>

`, sample.Title, sample.Description, sample.Folder, githubIcon, filesHTML.String()), nil
}

func filterCode(code string) string {
	lines := strings.Split(code, "\n")
	var filteredLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip build directives and generated code comments
		if strings.HasPrefix(trimmed, "//go:build") ||
			strings.HasPrefix(trimmed, "// +build") ||
			strings.HasPrefix(trimmed, "// Code generated") ||
			strings.HasPrefix(trimmed, "// Package ") ||
			strings.HasPrefix(trimmed, "// This ") ||
			strings.HasPrefix(trimmed, "// -") {
			continue
		}
		filteredLines = append(filteredLines, line)
	}
	return strings.TrimSpace(strings.Join(filteredLines, "\n"))
}

func highlightGo(code string) string {
	// First escape HTML
	code = html.EscapeString(code)

	// Process line by line to handle comments properly
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		// Check if line has a comment
		commentIdx := strings.Index(line, "//")
		if commentIdx >= 0 {
			// Process code part and comment part separately
			codePart := line[:commentIdx]
			commentPart := line[commentIdx:]
			lines[i] = highlightCodePart(codePart) + `<span class="comment">` + commentPart + `</span>`
		} else {
			lines[i] = highlightCodePart(line)
		}
	}

	return strings.Join(lines, "\n")
}

func highlightCodePart(code string) string {
	// Keywords (must be done before types to avoid conflicts)
	keywords := []string{
		"package", "import", "func", "return", "if", "else", "for", "range",
		"var", "const", "type", "struct", "interface", "map", "chan", "go",
		"defer", "select", "case", "default", "break", "continue", "switch",
		"nil", "true", "false",
	}

	// Strings (double quotes) - &#34; is the escaped quote
	stringPattern := regexp.MustCompile(`&#34;[^&#]*&#34;`)
	code = stringPattern.ReplaceAllStringFunc(code, func(s string) string {
		return `<span class="string">` + s + `</span>`
	})

	// Backtick strings
	backtickPattern := regexp.MustCompile("`[^`]*`")
	code = backtickPattern.ReplaceAllStringFunc(code, func(s string) string {
		return `<span class="string">` + s + `</span>`
	})

	// Keywords
	for _, kw := range keywords {
		pattern := regexp.MustCompile(`\b(` + kw + `)\b`)
		code = pattern.ReplaceAllString(code, `<span class="keyword">$1</span>`)
	}

	// Types (common Go types and ooo types) - avoid matching inside strings
	types := []string{
		"Server", "Storage", "Object", "RawMessage", "Request", "Client",
		"SubscribeConfig", "SubscribeListEvents", "RemoteConfig", "Path", "Meta",
		"Todo", "Book", "LogEntry",
		"error", "int", "bool", "byte", "float64",
	}
	for _, t := range types {
		// Only match if not inside a span already
		pattern := regexp.MustCompile(`([^">])\b(` + t + `)\b`)
		code = pattern.ReplaceAllString(code, `$1<span class="type">$2</span>`)
		// Also match at start of line
		pattern2 := regexp.MustCompile(`^(` + t + `)\b`)
		code = pattern2.ReplaceAllString(code, `<span class="type">$1</span>`)
	}

	// Function calls (word followed by opening paren)
	funcPattern := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\(`)
	code = funcPattern.ReplaceAllStringFunc(code, func(match string) string {
		name := match[:len(match)-1]
		// Skip if it's a keyword
		for _, kw := range keywords {
			if name == kw {
				return match
			}
		}
		// Skip if already wrapped in a span or part of span
		if strings.Contains(name, "span") || strings.Contains(name, "class") {
			return match
		}
		return `<span class="function">` + name + `</span>(`
	})

	return code
}
