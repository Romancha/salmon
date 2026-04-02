# bear-salmon-notes — OpenClaw Skill

> **Prefer the MCP server.** The `salmon-mcp` binary provides the same functionality as a proper MCP tool integration — no shell commands, no approval prompts. See the [MCP Server section](../../../../README.md#mcp-server) in the main README for setup. This skill remains available as a curl-based alternative.

OpenClaw skill for interacting with [Bear](https://bear.app) notes through the [Salmon Hub](https://github.com/romancha/salmon) consumer API.

## Prerequisites

- A running Salmon Hub instance
- A consumer token provisioned by the hub operator
- `curl` and `jq` installed on the host

## Installation

### Option 1: ClawHub (recommended)

Install from [ClawHub](https://clawhub.ai):

```bash
clawhub install bear-salmon-notes
```

### Option 2: Manual

Copy the skill folder to your OpenClaw skills directory:

```bash
cp -r tools/consumer/openclaw/bear-salmon-notes ~/.openclaw/workspace/skills/bear-salmon-notes
```

Or create a symlink for development:

```bash
ln -s "$(pwd)/tools/consumer/openclaw/bear-salmon-notes" ~/.openclaw/workspace/skills/bear-salmon-notes
```

## Configuration

Add the following to `~/.openclaw/openclaw.json`:

```json
{
  "skills": {
    "entries": {
      "bear-salmon-notes": {
        "enabled": true,
        "env": {
          "SALMON_HUB_URL": "https://salmon.example.com",
          "SALMON_CONSUMER_TOKEN": "your-consumer-token"
        }
      }
    }
  }
}
```

Replace the URL and token with your actual values.

## Obtaining a consumer token

The hub operator configures consumer tokens via the `SALMON_HUB_CONSUMER_TOKENS` environment variable on the hub server. Ask your hub operator to provision a token for your OpenClaw instance.

## Usage

Once installed and configured, start a new OpenClaw session. The skill activates automatically when you ask about Bear notes. Examples:

- "Show my recent notes"
- "Search notes for project roadmap"
- "Create a note titled Meeting Notes with tag work"
- "What's the sync status?"
- "List all tags"
- "Trash the note about old project"

## Security notes

- Never put your token directly in `SKILL.md`
- Store tokens only in `openclaw.json` env config
- Encrypted notes are read-only (the API returns 403 for write operations)
- All write operations go through an async queue — changes appear in Bear after the next bridge sync cycle
