# Jira Jet - Fast JIRA CLI Tool

A blazing fast command-line interface for JIRA operations. Written in Go, compiles to a single binary with zero runtime dependencies.

## Features

- **View tickets**: Fetch and display JIRA ticket information
- **Add comments**: Add comments to existing tickets
- **Update tickets**: Update ticket descriptions and epic/parent linking
- **Create tickets**: Create new tickets with epic linking support
- **Link tickets**: Create relationships between tickets (blocks, relates-to, duplicates, etc.)
- **Epic management**: List child tickets of an epic
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

# Link ticket to an epic
jet update PROJ-123 --epic PROJ-100

# Change parent ticket
jet update PROJ-123 --parent PROJ-200
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

### List epic children

```bash
# List child tickets of an epic
jet epic PROJ-100

# JSON format
jet epic PROJ-100 --format json

# Save to file
jet epic PROJ-100 --output children.txt
```

### Link tickets

```bash
# Create a "blocks" relationship
jet link PROJ-123 blocks PROJ-456

# Create a "relates to" relationship
jet link PROJ-123 relates-to PROJ-789

# Create a "duplicates" relationship
jet link PROJ-123 duplicates PROJ-999

# Use reverse relationships
jet link PROJ-456 is-blocked-by PROJ-123
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
- `--epic`: Epic key to link this ticket to
- `--parent`: Parent ticket key to link this ticket to

### `jet create`

Create a new ticket.

**Flags:**
- `--project, -p`: Project key (required)
- `--summary, -s`: Ticket summary/title (required)  
- `--description, -d`: Ticket description
- `--description-file`: Read description from file (use `-` for stdin)
- `--type, -t`: Issue type (default: Task)
- `--epic, -e`: Epic key to link this ticket to

### `jet epic EPIC-KEY`

List child tickets of an epic.

**Flags:**
- `--format`: Output format (`readable` or `json`)
- `--output, -o`: Output file (default: stdout)

### `jet link TICKET-KEY RELATIONSHIP TICKET-KEY`

Create a link between two tickets with a specified relationship.

**Common Relationships:**
- `blocks` / `is-blocked-by`: One ticket blocks another
- `relates-to`: Tickets are related
- `duplicates` / `is-duplicated-by`: One ticket duplicates another
- `clones` / `is-cloned-by`: One ticket is a clone of another
- `causes` / `is-caused-by`: One ticket causes another

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

# List epic children and save as JSON
jet epic ABC-100 --format json --output epic-children.json

# Link a ticket to a different epic
jet update ABC-456 --epic ABC-200

# Link two tickets with a relationship
jet link ABC-123 blocks ABC-456
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