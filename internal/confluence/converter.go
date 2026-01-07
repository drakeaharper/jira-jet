package confluence

import (
	"fmt"
	"io"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// MarkdownToStorage converts Markdown to Confluence storage format
func MarkdownToStorage(md string) string {
	// Create parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.Tables
	p := parser.NewWithExtensions(extensions)

	// Parse markdown to AST
	doc := markdown.Parse([]byte(md), p)

	// Create custom renderer for Confluence
	renderer := NewConfluenceRenderer()

	// Render to Confluence storage format
	return string(markdown.Render(doc, renderer))
}

// ConfluenceRenderer is a custom renderer for Confluence storage format
type ConfluenceRenderer struct {
	*html.Renderer
}

// NewConfluenceRenderer creates a new Confluence renderer
func NewConfluenceRenderer() *ConfluenceRenderer {
	// Configure HTML renderer with Confluence-specific options
	opts := html.RendererOptions{
		Flags: html.CommonFlags | html.HrefTargetBlank,
	}

	return &ConfluenceRenderer{
		Renderer: html.NewRenderer(opts),
	}
}

// RenderNode customizes rendering for Confluence-specific elements
func (r *ConfluenceRenderer) RenderNode(w io.Writer, node ast.Node, entering bool) ast.WalkStatus {
	switch n := node.(type) {
	case *ast.CodeBlock:
		return r.renderCodeBlock(w, n, entering)
	default:
		return r.Renderer.RenderNode(w, node, entering)
	}
}

// renderCodeBlock converts code blocks to Confluence code macros
func (r *ConfluenceRenderer) renderCodeBlock(w io.Writer, node *ast.CodeBlock, entering bool) ast.WalkStatus {
	if entering {
		lang := string(node.Info)
		if lang == "" {
			lang = "none"
		}

		// Write Confluence code macro
		fmt.Fprintf(w, `<ac:structured-macro ac:name="code">`)
		fmt.Fprintf(w, `<ac:parameter ac:name="language">%s</ac:parameter>`, lang)
		fmt.Fprintf(w, `<ac:plain-text-body><![CDATA[`)
		w.Write(node.Literal)
		fmt.Fprintf(w, `]]></ac:plain-text-body>`)
		fmt.Fprintf(w, `</ac:structured-macro>`)
		fmt.Fprintf(w, "\n")
	}
	return ast.GoToNext
}

// renderTable uses standard HTML table rendering (Confluence supports HTML tables)
func (r *ConfluenceRenderer) renderTable(w io.Writer, node *ast.Table, entering bool) ast.WalkStatus {
	return r.Renderer.RenderNode(w, node, entering)
}
