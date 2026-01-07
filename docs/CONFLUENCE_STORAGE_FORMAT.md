# Confluence Storage Format Guide

## What is Confluence Storage Format?

Confluence storage format is an XML-based format that Confluence uses internally to store page content. It's similar to XHTML but includes Confluence-specific macros and elements.

When you create or update pages via the API, Confluence expects content in this storage format, not plain Markdown or HTML.

## Why Not Plain Markdown?

**Important**: Confluence doesn't natively support Markdown. When you paste Markdown into Confluence (or send it via the API), it's treated as plain text and displayed in a code block.

This is a common issue when AI tools try to use `jet` to create pages - they often provide Markdown content which then appears incorrectly formatted in Confluence.

**Solution**: Use the `jet con convert` command to convert Markdown to Confluence storage format before creating or updating pages.

## Using the Converter

The `jet con convert` command converts Markdown to Confluence storage format:

### Basic Usage

```bash
# Convert a file
jet con convert README.md

# Convert from stdin
cat README.md | jet con convert

# Save to a file
jet con convert README.md --output content.html
```

### Piping to Create/Update

The real power comes from piping the converted content directly to create or update commands:

```bash
# Create a new page from Markdown
jet con convert README.md | jet con create "Documentation" --space ENG --file -

# Update an existing page from Markdown
jet con convert README.md | jet con update 123456 --content-file -

# Multi-step workflow
cat my-notes.md | jet con convert | jet con update 789012 --content-file -
```

## Supported Markdown Elements

### Headers

**Markdown:**
```markdown
# Header 1
## Header 2
### Header 3
#### Header 4
##### Header 5
###### Header 6
```

**Confluence Storage:**
```xml
<h1>Header 1</h1>
<h2>Header 2</h2>
<h3>Header 3</h3>
<h4>Header 4</h4>
<h5>Header 5</h5>
<h6>Header 6</h6>
```

### Text Formatting

**Markdown:**
```markdown
**bold text**
__also bold__

*italic text*
_also italic_

`inline code`
```

**Confluence Storage:**
```xml
<strong>bold text</strong>
<strong>also bold</strong>

<em>italic text</em>
<em>also italic</em>

<code>inline code</code>
```

### Code Blocks

Code blocks are converted to Confluence's code macro with syntax highlighting.

**Markdown:**
````markdown
```python
def hello():
    print("Hello, World!")
```
````

**Confluence Storage:**
```xml
<ac:structured-macro ac:name="code">
  <ac:parameter ac:name="language">python</ac:parameter>
  <ac:plain-text-body><![CDATA[def hello():
    print("Hello, World!")]]></ac:plain-text-body>
</ac:structured-macro>
```

**Supported Languages:**
python, javascript, java, go, rust, bash, shell, sql, json, xml, yaml, html, css, typescript, ruby, php, swift, kotlin, scala, and many more.

If no language is specified, it defaults to "none" (plain text).

### Links

**Markdown:**
```markdown
[Link text](https://example.com)
[Another link](https://example.com "With title")
```

**Confluence Storage:**
```xml
<a href="https://example.com">Link text</a>
<a href="https://example.com" title="With title">Another link</a>
```

### Lists

#### Unordered Lists

**Markdown:**
```markdown
- Item 1
- Item 2
  - Nested item 2.1
  - Nested item 2.2
- Item 3
```

**Confluence Storage:**
```xml
<ul>
  <li>Item 1</li>
  <li>Item 2
    <ul>
      <li>Nested item 2.1</li>
      <li>Nested item 2.2</li>
    </ul>
  </li>
  <li>Item 3</li>
</ul>
```

#### Ordered Lists

**Markdown:**
```markdown
1. First item
2. Second item
3. Third item
   1. Nested item
```

**Confluence Storage:**
```xml
<ol>
  <li>First item</li>
  <li>Second item</li>
  <li>Third item
    <ol>
      <li>Nested item</li>
    </ol>
  </li>
</ol>
```

### Tables

**Markdown:**
```markdown
| Header 1 | Header 2 | Header 3 |
|----------|----------|----------|
| Cell 1   | Cell 2   | Cell 3   |
| Cell 4   | Cell 5   | Cell 6   |
```

**Confluence Storage:**
```xml
<table>
  <thead>
    <tr>
      <th>Header 1</th>
      <th>Header 2</th>
      <th>Header 3</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>Cell 1</td>
      <td>Cell 2</td>
      <td>Cell 3</td>
    </tr>
    <tr>
      <td>Cell 4</td>
      <td>Cell 5</td>
      <td>Cell 6</td>
    </tr>
  </tbody>
</table>
```

### Blockquotes

**Markdown:**
```markdown
> This is a blockquote
> with multiple lines
```

**Confluence Storage:**
```xml
<blockquote>
  <p>This is a blockquote
  with multiple lines</p>
</blockquote>
```

### Horizontal Rules

**Markdown:**
```markdown
---
```

**Confluence Storage:**
```xml
<hr>
```

## Advanced: Writing Storage Format Directly

For features not available in Markdown, you can write Confluence storage format directly.

### Info/Note/Warning Panels

These colored panels are Confluence-specific and not available in Markdown.

**Info Panel (Blue):**
```xml
<ac:structured-macro ac:name="info">
  <ac:rich-text-body>
    <p>This is an informational message</p>
  </ac:rich-text-body>
</ac:structured-macro>
```

**Note Panel (Yellow):**
```xml
<ac:structured-macro ac:name="note">
  <ac:rich-text-body>
    <p>This is an important note</p>
  </ac:rich-text-body>
</ac:structured-macro>
```

**Warning Panel (Red):**
```xml
<ac:structured-macro ac:name="warning">
  <ac:rich-text-body>
    <p>This is a warning message</p>
  </ac:rich-text-body>
</ac:structured-macro>
```

**Tip Panel (Green):**
```xml
<ac:structured-macro ac:name="tip">
  <ac:rich-text-body>
    <p>This is a helpful tip</p>
  </ac:rich-text-body>
</ac:structured-macro>
```

### Table of Contents

```xml
<ac:structured-macro ac:name="toc">
  <ac:parameter ac:name="maxLevel">3</ac:parameter>
  <ac:parameter ac:name="minLevel">1</ac:parameter>
</ac:structured-macro>
```

### Page Include

Include another page's content:

```xml
<ac:structured-macro ac:name="include">
  <ac:parameter ac:name="">
    <ri:page ri:content-title="Page Title" />
  </ac:parameter>
</ac:structured-macro>
```

### Expand Macro (Collapsible Section)

```xml
<ac:structured-macro ac:name="expand">
  <ac:parameter ac:name="title">Click to expand</ac:parameter>
  <ac:rich-text-body>
    <p>Hidden content goes here</p>
  </ac:rich-text-body>
</ac:structured-macro>
```

### Status Macros

```xml
<!-- Green status -->
<ac:structured-macro ac:name="status">
  <ac:parameter ac:name="colour">Green</ac:parameter>
  <ac:parameter ac:name="title">COMPLETE</ac:parameter>
</ac:structured-macro>

<!-- Yellow status -->
<ac:structured-macro ac:name="status">
  <ac:parameter ac:name="colour">Yellow</ac:parameter>
  <ac:parameter ac:name="title">IN PROGRESS</ac:parameter>
</ac:structured-macro>

<!-- Red status -->
<ac:structured-macro ac:name="status">
  <ac:parameter ac:name="colour">Red</ac:parameter>
  <ac:parameter ac:name="title">BLOCKED</ac:parameter>
</ac:structured-macro>
```

## Combining Markdown and Direct Storage Format

You can use the converter for most content and add Confluence-specific macros manually:

```bash
# 1. Convert markdown
jet con convert README.md > content.html

# 2. Edit content.html to add info panels or other macros

# 3. Create/update page
jet con create "My Page" --space ENG --file content.html
```

Or use a script to insert macros:

```bash
# Add an info panel at the top of converted content
echo '<ac:structured-macro ac:name="info"><ac:rich-text-body><p>This page was auto-generated</p></ac:rich-text-body></ac:structured-macro>' > temp.html
jet con convert README.md >> temp.html
jet con create "My Page" --space ENG --file temp.html
rm temp.html
```

## Troubleshooting

### Content Shows as Code Block

**Problem**: Your content appears in a gray code block instead of being formatted.

**Cause**: You provided Markdown directly instead of Confluence storage format.

**Solution**: Use `jet con convert` to convert Markdown before sending to Confluence:

```bash
# Wrong - sends markdown directly
echo "# My Header" | jet con create "My Page" --space ENG --file -

# Correct - converts markdown first
echo "# My Header" | jet con convert | jet con create "My Page" --space ENG --file -
```

### Version Conflict Error

**Problem**: Error message "version conflict - page was modified by another user"

**Cause**: Someone else updated the page after you fetched it but before you tried to update it.

**Solution**: Simply run your update command again. The tool automatically fetches the latest version:

```bash
# Just retry the command
jet con update 123456 --title "New Title"
```

### Permission Denied

**Problem**: "access denied - you may not have permission to update this page"

**Cause**: Your Confluence user doesn't have edit permissions for the page or space.

**Solution**:
- Check with your Confluence administrator
- Verify you have edit permissions in the space
- Make sure you're using the correct API token

### Code Block Language Not Highlighting

**Problem**: Code in code blocks doesn't have syntax highlighting.

**Cause**: The language specified isn't recognized by Confluence.

**Solution**: Use one of the supported language names:
- Common: `python`, `javascript`, `java`, `go`, `bash`, `sql`, `json`, `xml`, `yaml`
- Full list available in Confluence documentation

### Empty Content After Conversion

**Problem**: Converted content is empty or malformed.

**Cause**: Input markdown might be invalid or have unsupported elements.

**Solution**:
1. Check your markdown is valid
2. Test with simple markdown first:
   ```bash
   echo "# Test Header" | jet con convert
   ```
3. If complex markdown fails, try simplifying it

## Best Practices

1. **Use the converter**: Always use `jet con convert` for Markdown content
2. **Test locally**: Save converted content to a file and inspect it before uploading
3. **Start simple**: Begin with basic markdown and add complexity gradually
4. **Use macros sparingly**: Confluence macros are powerful but can make content harder to edit
5. **Version control**: Keep your source markdown in git and convert when needed
6. **Validate content**: Check that converted content looks correct in Confluence after uploading

## Examples

### Creating Documentation from Markdown

```bash
# Complete workflow
jet con convert docs/api-reference.md | \
  jet con create "API Reference" --space DOCS --file -
```

### Updating Multiple Pages

```bash
# Update several related pages
for page_id in 123456 789012 345678; do
  jet con convert README.md | \
    jet con update $page_id --content-file -
done
```

### Mixed Content (Markdown + Macros)

```bash
# Create with info panel + markdown content
{
  echo '<ac:structured-macro ac:name="info"><ac:rich-text-body>'
  echo '<p>This documentation is auto-generated from the repository README</p>'
  echo '</ac:rich-text-body></ac:structured-macro>'
  jet con convert README.md
} | jet con create "Project Documentation" --space ENG --file -
```

## Resources

- [Confluence Storage Format Documentation](https://confluence.atlassian.com/doc/confluence-storage-format-790796544.html)
- [Confluence Macros](https://confluence.atlassian.com/doc/macros-139387.html)
- [Confluence API Documentation](https://developer.atlassian.com/cloud/confluence/rest/v2/)
- [Jira Jet GitHub Repository](https://github.com/drakeaharper/jira-jet)

## Getting Help

If you encounter issues not covered in this guide:

1. Check that your markdown is valid
2. Try the conversion locally and inspect the output
3. Test with simple content first
4. Verify your Confluence permissions
5. Check the Jira Jet GitHub issues for similar problems

For bugs or feature requests, open an issue on the [GitHub repository](https://github.com/drakeaharper/jira-jet/issues).
