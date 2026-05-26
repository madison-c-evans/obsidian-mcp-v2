# obsidian-vault-mcp

A Go [Model Context Protocol](https://modelcontextprotocol.io) server that
indexes an [Obsidian](https://obsidian.md) markdown vault into an in-memory
note + ontology graph and exposes it to MCP clients (Claude Code, Claude
Desktop, etc.) over stdio. On startup it walks the vault, parses YAML
frontmatter and inline `[[wikilinks]]`, and builds two graphs — an **ontology
graph** from `upstream:` parent links and a **mentions graph** from inline
links — then serves search, traversal, read, and write tools against that
index.

## Vault model

Each note is a markdown file with a YAML frontmatter block followed by a
markdown body:

```markdown
---
tags:
  - notes
external-links:
  - https://example.com/spec
upstream:
  - "[[Auth and Identity]]"
date: 2025-03-14
status: TODO
description: How session tokens are minted and rotated.
---

Body content with inline [[wikilinks]] to other notes.
```

### Frontmatter fields

All fields are optional. Unknown fields are dropped on write (the serializer
re-emits only the canonical schema, so editing a note through the server
normalizes its frontmatter).

| Field            | Type            | Meaning |
|------------------|-----------------|---------|
| `tags`           | list of strings | Free-form tags. The special tag `topic` marks a note as a topic page. |
| `external-links` | list of strings | URLs / references to material outside the vault. |
| `upstream`       | list of `[[wikilinks]]` (or a single wikilink) | Parent notes/topics. Defines the ontology graph. Plain titles passed via the MCP tools are wrapped in `[[ ]]` on write. |
| `date`           | string (YYYY-MM-DD) | Creation / reference date. Used by the health check to flag stale TODOs (older than 7 days). |
| `status`         | string          | Workflow status, e.g. `TODO`, `sprout`, `done`. Used as a filter in `vault_search` / `vault_list`. |
| `description`    | string          | Short (10–20 word) summary. Primarily used on topic pages; weighted nearly as highly as title in search so descriptive topics surface for related queries. |

### Topics and the ontology graph

A note is treated as a **topic** when either:

- its file lives under the `topics/` directory at the vault root, or
- its frontmatter `tags` list contains `topic`.

`vault_create` with `is_topic: true` places the file in `topics/`. All other
notes are created at the vault root.

The **ontology graph** is built from `upstream`: each entry of a note's
`upstream` list is a parent. The server computes, for every note, the set of
topics reachable by walking the upstream chain — this is what powers the
`topic:` filter on `vault_search` and `vault_list` (a note is "under" a topic
if that topic appears anywhere in its upstream closure).

### Inline mentions

Any `[[wikilink]]` in a note's **body** (not the frontmatter) that resolves to
another note in the vault becomes a directed edge in the **mentions graph**.
The optional `[[Target|Alias]]` form is supported — the alias is ignored, the
target is what's indexed. Self-links and links to non-existent notes are
skipped. This graph is independent of `upstream` and is queried via the
`vault_graph` tool's `mentions` direction.

### Indexing rules

- Only `*.md` files are indexed.
- The walker skips dotfile directories (e.g. `.obsidian/`), `node_modules/`,
  `images/`, and `pdfs/`.
- A note's **title** is its filename without the `.md` extension. All
  cross-references (`upstream`, inline `[[wikilinks]]`) are resolved by title.
- Title lookups in MCP tools are fuzzy — case-insensitive, with substring and
  word-overlap fallbacks — so callers do not need exact casing.

## MCP tools

The server registers 11 tools. All return human-readable markdown.

| Tool                    | Purpose |
|-------------------------|---------|
| `vault_search`          | Full-text + fuzzy search over titles, descriptions, and content, with optional `topic` / `status` / `tag` filters and graph-context expansion. |
| `vault_read`            | Return the full frontmatter summary plus markdown body of a single note. |
| `vault_graph`           | Traverse the ontology graph from a note (`ancestors`, `descendants`, `siblings`, `mentions`, or `all`) up to a given depth. |
| `vault_list`            | List notes in the vault, filtered by `topic`, `status`, `tag`, and `type` (`all` / `topics` / `notes`). |
| `vault_create`          | Create a new note (vault root) or topic (under `topics/`) with the given frontmatter and optional body. |
| `vault_edit`            | Edit a note's body: `replace` the whole body, `append`, or `replace_section` under a specific heading. |
| `vault_update_metadata` | Update frontmatter fields (`tags`, `status`, `upstream`, `external_links`, `description`) on an existing note. |
| `vault_delete`          | Delete a note and report any other notes that still reference it as `upstream` (so the caller can fix broken links). |
| `vault_rename`          | Rename a note's file and cascade the new title through all `upstream` references and inline `[[wikilinks]]` in other notes. |
| `vault_health`          | Health report: broken `upstream` links, orphan notes, stale TODOs (>7 days old), and empty notes. |
| `vault_reindex`         | Rebuild the in-memory index from disk. |

> After editing notes directly in Obsidian (or any other external tool), call
> `vault_reindex` to pick up the changes — the index is built once at startup
> and only refreshed by writes performed through the server's own tools.

## Build

Requires Go 1.26 or newer. With an older `go` binary, the toolchain directive
in `go.mod` will trigger an automatic download of the required version on
first invocation.

```sh
go build -o obsidian-vault-mcp .
```

## Run

The server speaks MCP over stdio. The vault path can be passed as the first
positional argument or via the `VAULT_PATH` environment variable.

```sh
./obsidian-vault-mcp /path/to/your/vault
# or
VAULT_PATH=/path/to/your/vault ./obsidian-vault-mcp
```

On startup the server prints a one-line summary to stderr:

```
obsidian-vault MCP ready — 142 notes, 18 topics, 213 edges, 76 inline mentions
```

## MCP client configuration

Add an entry to your MCP client's `mcpServers` config. For Claude Desktop the
config lives at `~/Library/Application Support/Claude/claude_desktop_config.json`
on macOS or `%APPDATA%\Claude\claude_desktop_config.json` on Windows; Claude
Code uses `~/.claude.json` (or a project-local `.mcp.json`). Point `command`
at the built binary and pass the vault path as a positional argument:

```json
{
  "mcpServers": {
    "obsidian-vault": {
      "command": "/absolute/path/to/obsidian-vault-mcp",
      "args": ["/absolute/path/to/your/vault"]
    }
  }
}
```

Alternatively, pass the vault path via environment:

```json
{
  "mcpServers": {
    "obsidian-vault": {
      "command": "/absolute/path/to/obsidian-vault-mcp",
      "env": {
        "VAULT_PATH": "/absolute/path/to/your/vault"
      }
    }
  }
}
```

Restart the client after editing the config; the 11 `vault_*` tools should
appear in the tool list.
