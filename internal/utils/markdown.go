package utils

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"wiki-go/internal/frontmatter"
	"wiki-go/internal/goldext"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// Custom HTML renderer for links
type pdfLinkRenderer struct {
	html.Config
}

// NewPDFLinkRenderer creates a new renderer
func NewLinkRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &pdfLinkRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

// RegisterFuncs implements NodeRenderer.RegisterFuncs
func (r *pdfLinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// Register the default HTML renderer for all nodes
	reg.Register(ast.KindLink, r.renderLink)
}

// Custom render function for links
func (r *pdfLinkRenderer) renderLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	var err error
	if !entering {
		_, err = w.WriteString("</a>")
		if err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	destination := string(node.(*ast.Link).Destination)
	text := string(node.Text(source))

	destinationLower := strings.ToLower(destination)
	if strings.HasPrefix(destinationLower, "/api/files/") && strings.HasSuffix(destinationLower, ".pdf") {
		destination = strings.TrimPrefix(destination, "/api/files")
		//Render as link to PDF viewer
		_, err = w.WriteString(`<a href="` + string(util.EscapeHTML([]byte(filepath.Dir(destination)))) + `?mode=pdf&file=` + filepath.Base(destination) + `">` + string(text) + `</a>`)
		if err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkSkipChildren, err
	}

	_, err = w.WriteString(`<a href="` + string(util.EscapeHTML([]byte(destination))) + `" target="_blank">` + string(text) + `</a>`)
	if err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}

// linkExtension is a goldmark.Extender
type pdfLinkExtension struct{}

// Extend implements goldmark.Extender
func (e *pdfLinkExtension) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(NewLinkRenderer(), 100),
	))
}

// RenderMarkdownFile reads a markdown file and returns its HTML representation
func RenderMarkdownFile(filePath string) ([]byte, error) {
	// Read the markdown file
	mdContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Get the directory path for the document
	docDir := filepath.Dir(filePath)

	// Convert to a relative path for URL construction
	relPath, err := filepath.Rel(filepath.Join("data", "documents"), docDir)
	if err != nil {
		// If we can't get a relative path, just use the directory name
		relPath = filepath.Base(docDir)
	}

	// Replace backslashes with forward slashes for URLs
	relPath = strings.ReplaceAll(relPath, "\\", "/")

	// Use the path-aware rendering function
	return RenderMarkdownWithPath(string(mdContent), relPath), nil
}

// RenderMarkdown converts markdown text to HTML
func RenderMarkdown(md string) []byte {
	return RenderMarkdownWithPath(md, "")
}

// RenderMarkdownWithPath converts markdown text to HTML with the current document path
func RenderMarkdownWithPath(md string, docPath string) []byte {
	// Check for frontmatter
	metadata, contentWithoutFrontmatter, hasFrontmatter := frontmatter.Parse(md)

	// If this has kanban layout, render as kanban with full goldext support
	if hasFrontmatter && metadata.Layout == "kanban" {
		// Create preprocessor functions (excluding frontmatter since it's already processed)
		var preprocessors []frontmatter.PreprocessorFunc
		var postProcessors []frontmatter.PostProcessorFunc

		// Add all goldext preprocessors (frontmatter will be a no-op since it's already processed)
		for _, preprocessor := range goldext.RegisteredPreprocessors {
			if preprocessor != nil {
				// Create a closure that captures the docPath for kanban rendering
				capturedPreprocessor := preprocessor
				capturedDocPath := docPath
				wrappedPreprocessor := func(md string, _ string) string {
					return capturedPreprocessor(md, capturedDocPath)
				}
				preprocessors = append(preprocessors, wrappedPreprocessor)
			}
		}

		// Add post-processors for mermaid and direction blocks
		postProcessors = append(postProcessors, func(html string) string {
			result := goldext.RestoreMermaidBlocks(html)
			result = goldext.RestoreDirectionBlocks(result)
			return result
		})

		kanbanHTML := frontmatter.RenderKanbanWithProcessors(contentWithoutFrontmatter, preprocessors, postProcessors)
		return []byte(kanbanHTML)
	}

	// If this has links layout, render as links document
	if hasFrontmatter && metadata.Layout == "links" {
		linksHTML, err := frontmatter.RenderLinks(contentWithoutFrontmatter)
		if err != nil {
			// If links rendering fails, fall back to regular markdown
			md = contentWithoutFrontmatter
		} else {
			return []byte(linksHTML)
		}
	}

	// If there's frontmatter but not kanban layout, use content without frontmatter
	if hasFrontmatter {
		md = contentWithoutFrontmatter
	}

	// Apply any custom extensions via pre-processing
	md = goldext.ProcessMarkdown(md, docPath)

	// Configure Goldmark with all needed extensions
	markdown := goldmark.New(
		// Enable common extensions
		goldmark.WithExtensions(
			extension.Table,         // Enable tables
			extension.Strikethrough, // Enable ~~strikethrough~~
			extension.Linkify,       // Auto-link URLs
			// extension.TaskList,    // Disabled - we use our own task list processor
			extension.Footnote,       // Enable footnotes
			extension.DefinitionList, // Enable definition lists
			extension.GFM,            // GitHub Flavored Markdown
			// MathJax is now handled via client-side JavaScript
			&pdfLinkExtension{},
		),
		// Parser options
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // Enable auto heading IDs
			parser.WithAttribute(),     // Enable attributes
		),
		// Renderer options
		goldmark.WithRendererOptions(
			html.WithUnsafe(), // Allow raw HTML in the markdown
			html.WithHardWraps(),
		),
	)

	// Create a buffer to store the rendered HTML
	var buf bytes.Buffer

	// Convert markdown to HTML
	if err := markdown.Convert([]byte(md), &buf); err != nil {
		// If there's an error, return an error message
		errMsg := []byte("<p>Error rendering markdown with Goldmark: " + err.Error() + "</p>")
		return errMsg
	}

	// Post-process: Restore Mermaid blocks that were replaced with placeholders
	htmlResult := goldext.RestoreMermaidBlocks(buf.String())

	// Post-process: Restore Direction blocks that were replaced with placeholders
	// This ensures RTL/LTR content is properly rendered with Markdown formatting
	htmlResult = goldext.RestoreDirectionBlocks(htmlResult)

	// Return the post-processed HTML
	return []byte(htmlResult)
}
