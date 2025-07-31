# Jira Jet - Fast JIRA CLI Tool

A blazing fast command-line interface for JIRA operations. Written in Go, compiles to a single binary with zero runtime dependencies.

## Features

- **View tickets**: Fetch and display JIRA ticket information
- **Add comments**: Add comments to existing tickets
- **Update tickets**: Update ticket descriptions and other fields
- **Create tickets**: Create new tickets with epic linking support
- **Multiple output formats**: Human-readable or JSON output
- **File input**: Read descriptions and comments from files
- **Zero dependencies**: Single binary, no runtime dependencies

## Installation

### Build from source

```bash
# Clone or download the project
cd jira-jet

# Build the binary
go build -o jet

# Install globally (optional)
sudo mv jet /usr/local/bin/
# OR
go install
```

## Configuration

Set up your JIRA credentials using environment variables or a config file.

### Environment Variables

```bash
export JIRA_URL="https://yourcompany.atlassian.net"
export JIRA_EMAIL="your.email@company.com"
export JIRA_API_TOKEN="your-api-token"
```

For JIRA Server/Data Center, use username instead of email:
```bash
export JIRA_USERNAME="your-username"
```

### Config File

Create `~/.jira_config`:

```ini
[jira]
url = https://yourcompany.atlassian.net
email = your.email@company.com
token = your-api-token
```

## Usage

### View a ticket

```bash
# Human-readable format (default)
jet view PROJ-123

# Using full JIRA URL
jet view https://company.atlassian.net/browse/PROJ-123

# JSON format
jet view PROJ-123 --format json

# Save to file
jet view PROJ-123 --output ticket.txt
```

### Add a comment

```bash
# Direct comment
jet comment PROJ-123 "This is my comment"

# From file
jet comment PROJ-123 --file comment.txt

# From stdin
echo "My comment" | jet comment PROJ-123 --file -
```

### Update a ticket

```bash
# Update description
jet update PROJ-123 --description "New description"

# Update from file
jet update PROJ-123 --description-file description.txt

# Update from stdin
echo "New description" | jet update PROJ-123 --description-file -
```

### Create a ticket

```bash
# Basic ticket
jet create --project PROJ --summary "Fix the bug"

# With description
jet create --project PROJ --summary "New feature" --description "Add user authentication"

# With epic link
jet create --project PROJ --summary "Sub-task" --epic PROJ-100

# With custom issue type
jet create --project PROJ --summary "Bug report" --type Bug

# Description from file
jet create --project PROJ --summary "Feature" --description-file spec.md
```

## Commands

### `jet view TICKET-KEY|URL`

Fetch and display ticket information. Accepts either a ticket key (e.g., PROJ-123) or a full JIRA URL.

**Flags:**
- `--format`: Output format (`readable` or `json`)
- `--output, -o`: Output file (default: stdout)

### `jet comment TICKET-KEY [COMMENT]`

Add a comment to a ticket.

**Flags:**
- `--file, -f`: Read comment from file (use `-` for stdin)

### `jet update TICKET-KEY`

Update ticket fields.

**Flags:**
- `--description`: New description text
- `--description-file`: Read description from file (use `-` for stdin)

### `jet create`

Create a new ticket.

**Flags:**
- `--project, -p`: Project key (required)
- `--summary, -s`: Ticket summary/title (required)  
- `--description, -d`: Ticket description
- `--description-file`: Read description from file (use `-` for stdin)
- `--type, -t`: Issue type (default: Task)
- `--epic, -e`: Epic key to link this ticket to

## Examples

```bash
# View a ticket in JSON and save to file
jet view ABC-123 --format json --output ticket.json

# Add a multi-line comment from file
jet comment ABC-123 --file <<EOF
This is a multi-line comment.

It can contain formatting and
multiple paragraphs.
EOF

# Create a bug report with description from file
jet create --project ABC --summary "Login fails on Safari" --type Bug --description-file bug-report.md

# Update ticket description from stdin
cat new-description.txt | jet update ABC-123 --description-file -
```

## Security

### Credential Storage
- Config files are automatically created with secure permissions (0600)
- API tokens are stored in plaintext - keep config files secure
- Use environment variables for better security in shared environments
- Never commit credentials to version control

### Network Security
- All connections use HTTPS with TLS 1.2+ and secure cipher suites
- URL validation ensures only trusted JIRA domains are accepted
- Self-update command requires explicit confirmation for security

### Best Practices
- Use API tokens instead of passwords
- Regularly rotate your API tokens
- Limit API token scope to minimum required permissions
- Monitor JIRA access logs for suspicious activity

## Error Handling

The tool provides clear error messages for common issues:

- **Authentication errors**: Check your credentials and API token
- **Permission errors**: Verify you have access to the project/ticket
- **Not found errors**: Confirm the ticket/project exists
- **Network errors**: Check your JIRA URL and network connection

## Development

Built with:
- Go 1.21+
- [Cobra](https://github.com/spf13/cobra) for CLI framework
- Standard library for HTTP and JSON handling

### Quick Start

```bash
# Clone the repository
git clone git@github.com:drakeaharper/ira-jet.git
cd ira-jet

# Build the binary
go build -o jet

# Run tests
go test ./...

# Install locally
go install
```

### Project Structure

```
.
├── cmd/          # Command implementations
├── internal/     # Internal packages
│   ├── config/   # Configuration handling
│   └── jira/     # JIRA API client
├── main.go       # Entry point
└── jet           # Compiled binary
```

## License

MIT License - see LICENSE file for details.

## Contributing

Pull requests are welcome! For major changes, please open an issue first to discuss what you would like to change.