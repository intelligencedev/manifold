package documents

import (
	"regexp"
	"strings"
)

// BoundaryDetector provides language-specific boundary detection for intelligent chunking.
type BoundaryDetector struct {
	lang Language
}

// NewBoundaryDetector creates a new boundary detector for the given language.
func NewBoundaryDetector(lang Language) *BoundaryDetector {
	return &BoundaryDetector{lang: lang}
}

// IsBoundary checks if the given line represents a logical boundary for the language.
func (b *BoundaryDetector) IsBoundary(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	switch b.lang {
	case Go:
		return b.isGoBoundary(trimmed)
	case Python:
		return b.isPythonBoundary(trimmed)
	case JavaScript, TypeScript:
		return b.isJavaScriptBoundary(trimmed)
	case Java:
		return b.isJavaBoundary(trimmed)
	case CSharp:
		return b.isCSharpBoundary(trimmed)
	case Rust:
		return b.isRustBoundary(trimmed)
	case Cpp, C:
		return b.isCppBoundary(trimmed)
	case Markdown:
		return b.isMarkdownBoundary(trimmed)
	case JSON:
		return b.isJSONBoundary(trimmed)
	case YAML:
		return b.isYAMLBoundary(trimmed)
	case HTML, XML:
		return b.isHTMLBoundary(trimmed)
	case CSS:
		return b.isCSSBoundary(trimmed)
	case SQL:
		return b.isSQLBoundary(trimmed)
	case Shell:
		return b.isShellBoundary(trimmed)
	default:
		return false
	}
}

// Go boundary detection
func (b *BoundaryDetector) isGoBoundary(line string) bool {
	// Function definitions
	if strings.HasPrefix(line, "func ") {
		return true
	}
	// Type definitions
	if strings.HasPrefix(line, "type ") {
		return true
	}
	// Interface definitions
	if strings.HasPrefix(line, "type ") && strings.Contains(line, "interface") {
		return true
	}
	// Struct definitions
	if strings.HasPrefix(line, "type ") && strings.Contains(line, "struct") {
		return true
	}
	// Package declarations
	if strings.HasPrefix(line, "package ") {
		return true
	}
	// Import blocks
	if strings.HasPrefix(line, "import ") {
		return true
	}
	// Variable/constant blocks
	if strings.HasPrefix(line, "var ") || strings.HasPrefix(line, "const ") {
		return true
	}
	return false
}

// Python boundary detection
func (b *BoundaryDetector) isPythonBoundary(line string) bool {
	// Class definitions
	if strings.HasPrefix(line, "class ") {
		return true
	}
	// Function definitions (including async)
	if strings.HasPrefix(line, "def ") || strings.HasPrefix(line, "async def ") {
		return true
	}
	// Import statements
	if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
		return true
	}
	// Decorators (major ones)
	if strings.HasPrefix(line, "@") {
		return true
	}
	// if __name__ == "__main__"
	if strings.Contains(line, "__name__") && strings.Contains(line, "__main__") {
		return true
	}
	return false
}

// JavaScript/TypeScript boundary detection
func (b *BoundaryDetector) isJavaScriptBoundary(line string) bool {
	// Function declarations
	if strings.HasPrefix(line, "function ") || strings.HasPrefix(line, "async function ") {
		return true
	}
	// Class declarations
	if strings.HasPrefix(line, "class ") {
		return true
	}
	// Export statements
	if strings.HasPrefix(line, "export ") {
		return true
	}
	// Import statements
	if strings.HasPrefix(line, "import ") {
		return true
	}
	// Interface/type definitions (TypeScript)
	if b.lang == TypeScript {
		if strings.HasPrefix(line, "interface ") || strings.HasPrefix(line, "type ") {
			return true
		}
	}
	// Const/let/var declarations at top level
	if strings.HasPrefix(line, "const ") || strings.HasPrefix(line, "let ") || strings.HasPrefix(line, "var ") {
		return true
	}
	return false
}

// Java boundary detection
func (b *BoundaryDetector) isJavaBoundary(line string) bool {
	// Class declarations
	if strings.Contains(line, "class ") && (strings.HasPrefix(line, "public ") || strings.HasPrefix(line, "private ") || strings.HasPrefix(line, "protected ")) {
		return true
	}
	// Interface declarations
	if strings.Contains(line, "interface ") {
		return true
	}
	// Method declarations
	if (strings.HasPrefix(line, "public ") || strings.HasPrefix(line, "private ") || strings.HasPrefix(line, "protected ")) && strings.Contains(line, "(") {
		return true
	}
	// Package declarations
	if strings.HasPrefix(line, "package ") {
		return true
	}
	// Import statements
	if strings.HasPrefix(line, "import ") {
		return true
	}
	return false
}

// C# boundary detection
func (b *BoundaryDetector) isCSharpBoundary(line string) bool {
	// Namespace declarations
	if strings.HasPrefix(line, "namespace ") {
		return true
	}
	// Class declarations
	if strings.Contains(line, "class ") && (strings.HasPrefix(line, "public ") || strings.HasPrefix(line, "private ") || strings.HasPrefix(line, "internal ")) {
		return true
	}
	// Interface declarations
	if strings.Contains(line, "interface ") {
		return true
	}
	// Method declarations
	if (strings.HasPrefix(line, "public ") || strings.HasPrefix(line, "private ") || strings.HasPrefix(line, "protected ")) && strings.Contains(line, "(") {
		return true
	}
	// Using statements
	if strings.HasPrefix(line, "using ") {
		return true
	}
	return false
}

// Rust boundary detection
func (b *BoundaryDetector) isRustBoundary(line string) bool {
	// Function definitions
	if strings.HasPrefix(line, "fn ") || strings.HasPrefix(line, "pub fn ") {
		return true
	}
	// Struct definitions
	if strings.HasPrefix(line, "struct ") || strings.HasPrefix(line, "pub struct ") {
		return true
	}
	// Enum definitions
	if strings.HasPrefix(line, "enum ") || strings.HasPrefix(line, "pub enum ") {
		return true
	}
	// Trait definitions
	if strings.HasPrefix(line, "trait ") || strings.HasPrefix(line, "pub trait ") {
		return true
	}
	// Implementation blocks
	if strings.HasPrefix(line, "impl ") {
		return true
	}
	// Module declarations
	if strings.HasPrefix(line, "mod ") || strings.HasPrefix(line, "pub mod ") {
		return true
	}
	// Use statements
	if strings.HasPrefix(line, "use ") {
		return true
	}
	return false
}

// C/C++ boundary detection
func (b *BoundaryDetector) isCppBoundary(line string) bool {
	// Function definitions (simple heuristic)
	if strings.Contains(line, "(") && strings.Contains(line, ")") && strings.Contains(line, "{") {
		return true
	}
	// Class definitions
	if strings.HasPrefix(line, "class ") {
		return true
	}
	// Struct definitions
	if strings.HasPrefix(line, "struct ") {
		return true
	}
	// Include statements
	if strings.HasPrefix(line, "#include ") {
		return true
	}
	// Namespace declarations
	if strings.HasPrefix(line, "namespace ") {
		return true
	}
	return false
}

// Markdown boundary detection
func (b *BoundaryDetector) isMarkdownBoundary(line string) bool {
	// Headers
	if strings.HasPrefix(line, "#") {
		return true
	}
	// Code blocks
	if strings.HasPrefix(line, "```") {
		return true
	}
	return false
}

// JSON boundary detection
func (b *BoundaryDetector) isJSONBoundary(line string) bool {
	// Top-level object/array start
	if line == "{" || line == "[" {
		return true
	}
	return false
}

// YAML boundary detection
func (b *BoundaryDetector) isYAMLBoundary(line string) bool {
	// Top-level keys (no indentation)
	if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
		return true
	}
	// Document separators
	if line == "---" || line == "..." {
		return true
	}
	return false
}

// HTML/XML boundary detection
func (b *BoundaryDetector) isHTMLBoundary(line string) bool {
	// Opening tags for major elements
	majorTags := []string{"<html", "<head", "<body", "<div", "<section", "<article", "<header", "<footer", "<nav", "<main"}
	for _, tag := range majorTags {
		if strings.HasPrefix(line, tag) {
			return true
		}
	}
	return false
}

// CSS boundary detection
func (b *BoundaryDetector) isCSSBoundary(line string) bool {
	// CSS selectors (simple heuristic)
	if strings.Contains(line, "{") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
		return true
	}
	// Media queries
	if strings.HasPrefix(line, "@media") {
		return true
	}
	return false
}

// SQL boundary detection
func (b *BoundaryDetector) isSQLBoundary(line string) bool {
	upperLine := strings.ToUpper(line)
	// Major SQL statements
	sqlKeywords := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP", "WITH"}
	for _, keyword := range sqlKeywords {
		if strings.HasPrefix(upperLine, keyword+" ") {
			return true
		}
	}
	return false
}

// Shell boundary detection
func (b *BoundaryDetector) isShellBoundary(line string) bool {
	// Function definitions
	if strings.Contains(line, "()") && strings.Contains(line, "{") {
		return true
	}
	// Shebang
	if strings.HasPrefix(line, "#!") {
		return true
	}
	return false
}

// GetImportantPatterns returns regex patterns for preserving important code structures
func (b *BoundaryDetector) GetImportantPatterns() []*regexp.Regexp {
	switch b.lang {
	case Go:
		return []*regexp.Regexp{
			regexp.MustCompile(`^func\s+\w+`),             // Function definitions
			regexp.MustCompile(`^type\s+\w+\s+struct`),    // Struct definitions
			regexp.MustCompile(`^type\s+\w+\s+interface`), // Interface definitions
		}
	case Python:
		return []*regexp.Regexp{
			regexp.MustCompile(`^class\s+\w+`),       // Class definitions
			regexp.MustCompile(`^def\s+\w+`),         // Function definitions
			regexp.MustCompile(`^async\s+def\s+\w+`), // Async function definitions
		}
	case JavaScript, TypeScript:
		return []*regexp.Regexp{
			regexp.MustCompile(`^function\s+\w+`), // Function definitions
			regexp.MustCompile(`^class\s+\w+`),    // Class definitions
			regexp.MustCompile(`^export\s+`),      // Export statements
		}
	default:
		return nil
	}
}
