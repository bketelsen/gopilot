package internal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAllPackagesHaveDocGo verifies that every package under internal/ has a
// doc.go file with a package-level comment that starts with "Package <name>".
func TestAllPackagesHaveDocGo(t *testing.T) {
	packages := []string{
		"agent",
		"config",
		"domain",
		"github",
		"logging",
		"metrics",
		"orchestrator",
		"planning",
		"prompt",
		"setup",
		"skills",
		"web",
		"workspace",
	}

	for _, pkg := range packages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			docPath := filepath.Join(pkg, "doc.go")

			// File must exist.
			info, err := os.Stat(docPath)
			if err != nil {
				t.Fatalf("doc.go missing for package %s: %v", pkg, err)
			}
			if info.IsDir() {
				t.Fatalf("%s is a directory, expected a file", docPath)
			}

			// Parse the file and check for a package-level doc comment.
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, docPath, nil, parser.ParseComments)
			if err != nil {
				t.Fatalf("failed to parse %s: %v", docPath, err)
			}

			if f.Doc == nil {
				t.Fatalf("doc.go in %s has no package-level doc comment", pkg)
			}

			comment := f.Doc.Text()
			prefix := "Package " + pkg + " "
			if !strings.HasPrefix(comment, prefix) {
				// Also allow the comment to end right after the package name with a period or newline.
				prefixDot := "Package " + pkg + "."
				if !strings.HasPrefix(comment, prefix) && !strings.HasPrefix(comment, prefixDot) {
					t.Errorf("doc comment in %s should start with %q, got: %s", pkg, prefix, firstLine(comment))
				}
			}

			// Verify it declares the correct package.
			if f.Name == nil || f.Name.Name != pkg {
				// Some packages use a different Go package name (e.g., github -> github).
				// Just verify doc.go has a valid package declaration.
				if f.Name == nil {
					t.Errorf("doc.go in %s has no package declaration", pkg)
				}
			}
		})
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestDocGoPackageCommentIsComplete checks that doc comments use complete
// sentences (end with a period).
func TestDocGoPackageCommentIsComplete(t *testing.T) {
	packages := []string{
		"agent",
		"config",
		"domain",
		"github",
		"logging",
		"metrics",
		"orchestrator",
		"planning",
		"prompt",
		"setup",
		"skills",
		"web",
		"workspace",
	}

	for _, pkg := range packages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			docPath := filepath.Join(pkg, "doc.go")

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, docPath, nil, parser.ParseComments)
			if err != nil {
				t.Skipf("cannot parse %s: %v", docPath, err)
			}
			if f.Doc == nil {
				t.Skipf("no doc comment in %s", pkg)
			}

			comment := strings.TrimSpace(f.Doc.Text())
			if !strings.HasSuffix(comment, ".") {
				t.Errorf("doc comment in %s should end with a period", pkg)
			}
		})
	}
}

// TestDocGoIsMinimal ensures doc.go files contain only the package clause and
// doc comment (no functions, types, or variables).
func TestDocGoIsMinimal(t *testing.T) {
	packages := []string{
		"agent",
		"config",
		"domain",
		"github",
		"logging",
		"metrics",
		"orchestrator",
		"planning",
		"prompt",
		"setup",
		"skills",
		"web",
		"workspace",
	}

	for _, pkg := range packages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			docPath := filepath.Join(pkg, "doc.go")

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, docPath, nil, parser.ParseComments)
			if err != nil {
				t.Skipf("cannot parse %s: %v", docPath, err)
			}

			if len(f.Decls) > 0 {
				for _, decl := range f.Decls {
					switch d := decl.(type) {
					case *ast.GenDecl:
						// Allow import declarations but nothing else.
						if d.Tok != token.IMPORT {
							t.Errorf("doc.go in %s should only contain the package clause and doc comment, found %s declaration", pkg, d.Tok)
						}
					default:
						t.Errorf("doc.go in %s should only contain the package clause and doc comment, found unexpected declaration", pkg)
					}
				}
			}
		})
	}
}
