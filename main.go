package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/madison-c-evans/obsidian-vault-mcp/vault"
)

func main() {
	vaultPath := ""
	if len(os.Args) > 1 {
		vaultPath = os.Args[1]
	} else {
		vaultPath = os.Getenv("VAULT_PATH")
	}
	if vaultPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: obsidian-vault-mcp <vault-path>  (or set VAULT_PATH)")
		os.Exit(1)
	}

	v, err := vault.New(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: %v\n", err)
		os.Exit(1)
	}
	if err := v.BuildIndex(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: %v\n", err)
		os.Exit(1)
	}
	stats := v.GetStats()
	fmt.Fprintf(os.Stderr,
		"obsidian-vault MCP ready — %d notes, %d topics, %d edges, %d inline mentions\n",
		stats.NoteCount, stats.TopicCount, stats.EdgeCount, stats.MentionCount,
	)

	s := server.NewMCPServer("obsidian-vault", "1.0.0")
	registerTools(s, v)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: %v\n", err)
		os.Exit(1)
	}
}

func registerTools(s *server.MCPServer, v *vault.Vault) {
	s.AddTool(primeTool(), primeHandler(v))
	s.AddTool(searchTool(), searchHandler(v))
	s.AddTool(readTool(), readHandler(v))
	s.AddTool(graphTool(), graphHandler(v))
	s.AddTool(listTool(), listHandler(v))
	s.AddTool(createTool(), createHandler(v))
	s.AddTool(editTool(), editHandler(v))
	s.AddTool(updateMetadataTool(), updateMetadataHandler(v))
	s.AddTool(deleteTool(), deleteHandler(v))
	s.AddTool(renameTool(), renameHandler(v))
	s.AddTool(healthTool(), healthHandler(v))
	s.AddTool(reindexTool(), reindexHandler(v))
	s.AddTool(tasksTool(), tasksHandler(v))
	s.AddTool(tagsTool(), tagsHandler(v))
}

// ── Formatters ───────────────────────────────────────────────────────────────

func formatNoteSummary(n *vault.Note) string {
	tags := joinOrNone(n.Frontmatter.Tags)
	upstream := joinOrNone(n.Frontmatter.Upstream)
	status := n.Frontmatter.Status
	if status == "" {
		status = "(none)"
	}
	date := n.Frontmatter.Date
	if date == "" {
		date = "(none)"
	}
	noteType := "Note"
	if n.IsTopic {
		noteType = "Topic"
	}
	lines := []string{
		"**" + n.Title + "**",
		"  Path: " + n.RelativePath,
	}
	if n.Frontmatter.Description != "" {
		lines = append(lines, "  Description: "+n.Frontmatter.Description)
	}
	lines = append(lines,
		"  Tags: "+tags,
		"  Status: "+status,
		"  Upstream: "+upstream,
		"  Date: "+date,
		"  Type: "+noteType,
	)
	return strings.Join(lines, "\n")
}

func formatNoteFull(n *vault.Note) string {
	header := formatNoteSummary(n)
	if n.Content == "" {
		return header
	}
	return header + "\n\n---\n\n" + n.Content
}

func formatGraphCompact(ctx *vault.GraphContext) string {
	if ctx == nil {
		return ""
	}
	var parts []string
	if len(ctx.Ancestors) > 0 {
		parts = append(parts, "**Ancestors:** "+joinTitles(ctx.Ancestors))
	}
	if len(ctx.Descendants) > 0 {
		parts = append(parts, "**Descendants:** "+joinTitles(ctx.Descendants))
	}
	if len(ctx.Siblings) > 0 {
		parts = append(parts, "**Siblings:** "+joinTitles(ctx.Siblings))
	}
	if len(ctx.Mentions) > 0 {
		parts = append(parts, "**Mentions:** "+joinTitles(ctx.Mentions))
	}
	if len(ctx.MentionedBy) > 0 {
		parts = append(parts, "**Mentioned by:** "+joinTitles(ctx.MentionedBy))
	}
	return strings.Join(parts, "\n")
}

func joinTitles(notes []*vault.Note) string {
	titles := make([]string, len(notes))
	for i, n := range notes {
		titles[i] = n.Title
	}
	return strings.Join(titles, ", ")
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}

// ── Tool defs + handlers ─────────────────────────────────────────────────────

func primeTool() mcp.Tool {
	return mcp.NewTool("vault_prime",
		mcp.WithDescription(`Orient yourself in this vault before doing anything else. Returns a one-shot
overview: vault stats, the topic ontology (curated hubs with descriptions,
biggest areas first, showing how topics nest), and the status vocabulary
actually in use. Call this at the start of a session to learn the vault's
shape — what areas exist and how they relate — without having to probe with
repeated list/search calls. After priming, use vault_search to retrieve and
vault_graph to traverse.`),
	)
}

func primeHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		data := v.GetPrimeData()
		s := data.Stats

		parts := []string{
			fmt.Sprintf("## Vault: %d notes, %d topics, %d upstream edges, %d inline mentions",
				s.NoteCount, s.TopicCount, s.EdgeCount, s.MentionCount),
			"",
			"An Obsidian knowledge vault structured as an ontology graph. Each note carries " +
				"frontmatter: `upstream` (parent topics/notes — the hierarchy), `tags`, `status`, " +
				"and an optional `description`. Inline `[[wikilinks]]` form a secondary mention graph. " +
				"Topics live in `topics/` and act as curated hubs summarizing an area.",
			"",
			"Retrieval: `vault_search` (description-weighted, graph-expanded) is the primary tool. " +
				"`vault_graph` traverses from a note (ancestors/descendants/siblings/mentions). " +
				"`vault_read` returns a note's full body.",
			"",
		}

		parts = append(parts, "### Topic map (largest areas first)")
		if len(data.Topics) == 0 {
			parts = append(parts, "(no topics yet)")
		} else {
			for _, t := range data.Topics {
				noun := "notes"
			if t.NoteCount == 1 {
				noun = "note"
			}
			line := fmt.Sprintf("- **%s** (%d %s)", t.Title, t.NoteCount, noun)
				if t.Description != "" {
					line += " — " + t.Description
				}
				if len(t.ParentTopics) > 0 {
					line += "  _[under: " + strings.Join(t.ParentTopics, ", ") + "]_"
				}
				parts = append(parts, line)
			}
		}
		parts = append(parts, "")

		parts = append(parts, "### Status vocabulary in use")
		if len(data.Statuses) == 0 {
			parts = append(parts, "(no statuses set)")
		} else {
			var statusParts []string
			for _, st := range data.Statuses {
				statusParts = append(statusParts, fmt.Sprintf("%s (%d)", st.Status, st.Count))
			}
			parts = append(parts, strings.Join(statusParts, ", "))
		}

		return mcp.NewToolResultText(strings.Join(parts, "\n")), nil
	}
}

func searchTool() mcp.Tool {
	return mcp.NewTool("vault_search",
		mcp.WithDescription(`Search the vault for notes by title, frontmatter description, or content.
Returns relevant notes with their position in the knowledge graph
(ancestors, descendants, siblings). Topic pages carry a short curated
description in frontmatter — matches against that field are weighted
nearly as highly as title matches, so a query like "auth flows" can land
on the "Auth and Identity" topic via its description even when the title
doesn't contain the words. Use this as the primary retrieval tool — it
combines fuzzy matching with ontology-aware graph expansion.`),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query (matched against titles and content)")),
		mcp.WithString("scope", mcp.Description("Where to search. Default: all"), mcp.Enum("title", "content", "all")),
		mcp.WithString("topic", mcp.Description("Only return notes under this topic (traverses upstream chain)")),
		mcp.WithString("status", mcp.Description("Filter by status (e.g. TODO, sprout)")),
		mcp.WithString("tag", mcp.Description("Filter by tag")),
		mcp.WithBoolean("include_graph", mcp.Description("Expand graph context for each result. Default: true")),
		mcp.WithNumber("graph_depth", mcp.Description("Graph traversal depth. Default: 1")),
		mcp.WithNumber("max_results", mcp.Description("Max results to return. Default: 10")),
	)
}

func searchHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		opts := vault.SearchOptions{
			Scope:        req.GetString("scope", "all"),
			Topic:        req.GetString("topic", ""),
			Status:       req.GetString("status", ""),
			Tag:          req.GetString("tag", ""),
			IncludeGraph: req.GetBool("include_graph", true),
			GraphDepth:   req.GetInt("graph_depth", 1),
			MaxResults:   req.GetInt("max_results", 10),
		}
		results := v.Search(query, opts)
		if len(results) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf(`No results found for %q.`, query)), nil
		}

		// Build a deduped topic legend.
		legendMap := map[string]*vault.Note{}
		for _, r := range results {
			for _, t := range v.GetTopicsOf(r.Note.Title) {
				legendMap[t.Title] = t
			}
		}

		var legend string
		if len(legendMap) > 0 {
			lines := []string{"### Topics referenced"}
			for _, t := range legendMap {
				desc := ""
				if t.Frontmatter.Description != "" {
					desc = " — " + t.Frontmatter.Description
				}
				lines = append(lines, "- **"+t.Title+"**"+desc)
			}
			legend = strings.Join(lines, "\n") + "\n\n"
		}

		var blocks []string
		for i, r := range results {
			parts := []string{
				fmt.Sprintf("### %d. %s  (score: %.2f, match: %s)", i+1, r.Note.Title, r.Score, r.MatchType),
				formatNoteSummary(r.Note),
			}
			if r.GraphContext != nil {
				compact := formatGraphCompact(r.GraphContext)
				if compact != "" {
					parts = append(parts, "", compact)
				}
			}
			blocks = append(blocks, strings.Join(parts, "\n"))
		}

		body := fmt.Sprintf(`## Search: %q  (%d results)`+"\n\n", query, len(results)) +
			legend + strings.Join(blocks, "\n\n---\n\n")
		return mcp.NewToolResultText(body), nil
	}
}

func readTool() mcp.Tool {
	return mcp.NewTool("vault_read",
		mcp.WithDescription("Read the full content of a note (frontmatter + body). Title is fuzzy-matched."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Note title (fuzzy matched)")),
	)
}

func readHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		n := v.ResolveNote(title)
		if n == nil {
			return mcp.NewToolResultText(fmt.Sprintf(`Note not found: %q`, title)), nil
		}
		return mcp.NewToolResultText(formatNoteFull(n)), nil
	}
}

func graphTool() mcp.Tool {
	return mcp.NewTool("vault_graph",
		mcp.WithDescription(`Explore the ontology graph from a specific note. Shows ancestors (upstream
chain toward topics), descendants (notes that list this as upstream), and
siblings (notes sharing the same parent).`),
		mcp.WithString("title", mcp.Required(), mcp.Description("Note title to explore from")),
		mcp.WithString("direction", mcp.Description("Traversal direction. 'mentions' shows inline [[wikilink]] connections. Default: all"),
			mcp.Enum("ancestors", "descendants", "siblings", "mentions", "all")),
		mcp.WithNumber("depth", mcp.Description("Max traversal depth. Default: 2")),
	)
}

func graphHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		n := v.ResolveNote(title)
		if n == nil {
			return mcp.NewToolResultText(fmt.Sprintf(`Note not found: %q`, title)), nil
		}
		depth := req.GetInt("depth", 2)
		direction := req.GetString("direction", "all")

		parts := []string{"## Graph: " + n.Title, ""}

		if direction == "ancestors" || direction == "all" {
			parts = append(parts, "### Ancestors (upstream)")
			ancestors := v.GetAncestors(n.Title, depth)
			if len(ancestors) == 0 {
				parts = append(parts, "(root node)")
			} else {
				for _, a := range ancestors {
					typ := "Note"
					if a.IsTopic {
						typ = "Topic"
					}
					status := a.Frontmatter.Status
					if status == "" {
						status = "n/a"
					}
					parts = append(parts, fmt.Sprintf("- **%s** (%s) — status: %s", a.Title, typ, status))
				}
			}
			parts = append(parts, "")
		}

		if direction == "descendants" || direction == "all" {
			parts = append(parts, "### Descendants (downstream)")
			descendants := v.GetDescendants(n.Title, depth)
			if len(descendants) == 0 {
				parts = append(parts, "(leaf node)")
			} else {
				for _, c := range descendants {
					status := c.Frontmatter.Status
					if status == "" {
						status = "n/a"
					}
					parts = append(parts, fmt.Sprintf("- **%s** — status: %s", c.Title, status))
				}
			}
			parts = append(parts, "")
		}

		if direction == "siblings" || direction == "all" {
			parts = append(parts, "### Siblings (same parent)")
			siblings := v.GetSiblings(n.Title)
			if len(siblings) == 0 {
				parts = append(parts, "(none)")
			} else {
				for _, s := range siblings {
					status := s.Frontmatter.Status
					if status == "" {
						status = "n/a"
					}
					parts = append(parts, fmt.Sprintf("- **%s** — status: %s", s.Title, status))
				}
			}
			parts = append(parts, "")
		}

		if direction == "mentions" || direction == "all" {
			parts = append(parts, "### Mentions (inline [[links]] from this note)")
			mentions := v.GetMentions(n.Title)
			if len(mentions) == 0 {
				parts = append(parts, "(none)")
			} else {
				for _, m := range mentions {
					parts = append(parts, "- **"+m.Title+"**")
				}
			}
			parts = append(parts, "")

			parts = append(parts, "### Mentioned by (other notes link here)")
			mentionedBy := v.GetMentionedBy(n.Title)
			if len(mentionedBy) == 0 {
				parts = append(parts, "(none)")
			} else {
				for _, m := range mentionedBy {
					parts = append(parts, "- **"+m.Title+"**")
				}
			}
		}

		return mcp.NewToolResultText(strings.Join(parts, "\n")), nil
	}
}

func listTool() mcp.Tool {
	return mcp.NewTool("vault_list",
		mcp.WithDescription("List notes in the vault with optional filters (topic, status, tag, type)."),
		mcp.WithString("topic", mcp.Description("Only notes under this topic (traverses upstream)")),
		mcp.WithString("status", mcp.Description("Filter by status")),
		mcp.WithString("tag", mcp.Description("Filter by tag")),
		mcp.WithString("type", mcp.Description("Filter by type. Default: all"), mcp.Enum("all", "topics", "notes")),
	)
}

func listHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		typeFilter := req.GetString("type", "all")

		var results []*vault.Note
		if typeFilter == "topics" {
			results = v.ListTopics()
		} else {
			results = v.ListNotes(vault.ListFilters{
				Topic:  req.GetString("topic", ""),
				Status: req.GetString("status", ""),
				Tag:    req.GetString("tag", ""),
			})
			if typeFilter == "notes" {
				filtered := results[:0]
				for _, n := range results {
					if !n.IsTopic {
						filtered = append(filtered, n)
					}
				}
				results = filtered
			}
		}

		if len(results) == 0 {
			return mcp.NewToolResultText("No notes found matching filters."), nil
		}

		stats := v.GetStats()
		var lines []string
		for _, n := range results {
			label := ""
			if n.IsTopic {
				label = " [TOPIC]"
			}
			desc := ""
			if n.Frontmatter.Description != "" {
				desc = "\n    " + n.Frontmatter.Description
			}
			status := n.Frontmatter.Status
			if status == "" {
				status = "n/a"
			}
			upstream := strings.Join(n.Frontmatter.Upstream, ", ")
			if upstream == "" {
				upstream = "none"
			}
			lines = append(lines, fmt.Sprintf("- **%s**%s — status: %s, upstream: %s%s",
				n.Title, label, status, upstream, desc))
		}

		header := fmt.Sprintf("## Vault (%d matching)  |  %d notes, %d topics, %d edges, %d mentions\n\n",
			len(results), stats.NoteCount, stats.TopicCount, stats.EdgeCount, stats.MentionCount)
		return mcp.NewToolResultText(header + strings.Join(lines, "\n")), nil
	}
}

func createTool() mcp.Tool {
	return mcp.NewTool("vault_create",
		mcp.WithDescription(`Create a new note or topic. Notes go in the vault root; topics go in topics/.
Upstream accepts plain titles — they are stored as [[wikilinks]].`),
		mcp.WithString("title", mcp.Required(), mcp.Description("Title of the new note (becomes the file name)")),
		mcp.WithArray("upstream", mcp.Description("Parent note/topic titles"), mcp.WithStringItems()),
		mcp.WithArray("tags", mcp.Description("Tags (default: ['notes'])"), mcp.WithStringItems()),
		mcp.WithString("status", mcp.Description("Status (default: TODO)")),
		mcp.WithString("content", mcp.Description("Initial markdown body")),
		mcp.WithBoolean("is_topic", mcp.Description("Create as a topic (placed in topics/ folder)")),
		mcp.WithString("description", mcp.Description(
			"Short summary (10–20 words) of what this note covers. Primarily used on topic pages — topics get an empty `description:` line by default; pass a value here to fill it in at creation time. Used by retrieval to match queries against curated summaries.")),
	)
}

func createHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		opts := vault.CreateOptions{
			Title:    title,
			Upstream: req.GetStringSlice("upstream", nil),
			Status:   req.GetString("status", ""),
			Content:  req.GetString("content", ""),
			IsTopic:  req.GetBool("is_topic", false),
		}
		args := req.GetArguments()
		if _, ok := args["tags"]; ok {
			opts.Tags = req.GetStringSlice("tags", nil)
			opts.HasTags = true
		}
		if _, ok := args["description"]; ok {
			opts.Description = req.GetString("description", "")
			opts.HasDesc = true
		}

		entry, err := v.CreateNote(opts)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(
			"Created: " + entry.RelativePath + "\n\n" + formatNoteSummary(entry),
		), nil
	}
}

func editTool() mcp.Tool {
	return mcp.NewTool("vault_edit",
		mcp.WithDescription(`Edit a note's markdown body. Three operations:
  - replace: overwrite the entire body
  - append: add content to the end
  - replace_section: replace content under a specific ## heading (preserves the heading, replaces body until next heading of same or higher level)`),
		mcp.WithString("title", mcp.Required(), mcp.Description("Note title (fuzzy matched)")),
		mcp.WithString("operation", mcp.Required(),
			mcp.Description("replace = overwrite body, append = add to end, replace_section = replace under a heading"),
			mcp.Enum("replace", "append", "replace_section")),
		mcp.WithString("content", mcp.Required(),
			mcp.Description("New body (replace), text to add (append), or new section body (replace_section)")),
		mcp.WithString("heading",
			mcp.Description("Target heading for replace_section (e.g. 'Architecture Overview'). Required when operation is replace_section.")),
	)
}

func editHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		operation, err := req.RequireString("operation")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var updated *vault.Note
		switch operation {
		case "replace":
			updated, err = v.EditNoteContent(title, content)
		case "append":
			updated, err = v.AppendToNote(title, content)
		case "replace_section":
			heading := req.GetString("heading", "")
			if heading == "" {
				return mcp.NewToolResultError("'heading' is required for replace_section operation"), nil
			}
			updated, err = v.EditNoteSection(title, heading, content)
		default:
			return mcp.NewToolResultError("unknown operation: " + operation), nil
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(
			fmt.Sprintf("Updated: %s (%s)\n\n%s", updated.Title, operation, formatNoteFull(updated)),
		), nil
	}
}

func updateMetadataTool() mcp.Tool {
	return mcp.NewTool("vault_update_metadata",
		mcp.WithDescription("Update a note's frontmatter fields (tags, status, upstream, external-links)."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Note title (fuzzy matched)")),
		mcp.WithString("status", mcp.Description("New status value")),
		mcp.WithArray("tags", mcp.Description("New tags array"), mcp.WithStringItems()),
		mcp.WithArray("upstream", mcp.Description("New upstream parent titles (plain, no [[ ]])"), mcp.WithStringItems()),
		mcp.WithArray("external_links", mcp.Description("New external links"), mcp.WithStringItems()),
		mcp.WithString("description", mcp.Description(
			"Short summary (10–20 words) of what this note covers. Primarily used on topic pages. Pass an empty string to clear.")),
	)
}

func updateMetadataHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		args := req.GetArguments()
		updates := vault.FrontmatterUpdate{}
		if _, ok := args["tags"]; ok {
			updates.Tags = req.GetStringSlice("tags", nil)
			updates.HasTags = true
		}
		if _, ok := args["upstream"]; ok {
			updates.Upstream = req.GetStringSlice("upstream", nil)
			updates.HasUpstream = true
		}
		if _, ok := args["external_links"]; ok {
			updates.ExternalLinks = req.GetStringSlice("external_links", nil)
			updates.HasExternalLinks = true
		}
		if _, ok := args["status"]; ok {
			updates.Status = req.GetString("status", "")
			updates.HasStatus = true
		}
		if _, ok := args["description"]; ok {
			updates.Description = req.GetString("description", "")
			updates.HasDesc = true
		}

		updated, err := v.UpdateFrontmatter(title, updates)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(
			"Updated metadata: " + updated.Title + "\n\n" + formatNoteSummary(updated),
		), nil
	}
}

func deleteTool() mcp.Tool {
	return mcp.NewTool("vault_delete",
		mcp.WithDescription(`Delete a note from the vault. Reports any notes that referenced the deleted
note as upstream (these become broken links — consider updating them).`),
		mcp.WithString("title", mcp.Required(), mcp.Description("Note title to delete (fuzzy matched)")),
	)
}

func deleteHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		res, err := v.DeleteNote(title)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		parts := []string{"Deleted: **" + res.Deleted + "**"}
		if len(res.AffectedNotes) > 0 {
			parts = append(parts, "",
				fmt.Sprintf(`**Warning:** %d note(s) still reference %q as upstream:`,
					len(res.AffectedNotes), res.Deleted))
			for _, n := range res.AffectedNotes {
				parts = append(parts, "  - "+n)
			}
			parts = append(parts, "", "Consider updating their upstream to avoid broken links.")
		}
		return mcp.NewToolResultText(strings.Join(parts, "\n")), nil
	}
}

func renameTool() mcp.Tool {
	return mcp.NewTool("vault_rename",
		mcp.WithDescription(`Rename a note. Renames the file and cascades the change across the vault:
updates all upstream references and inline [[wikilinks]] in other notes.`),
		mcp.WithString("title", mcp.Required(), mcp.Description("Current note title (fuzzy matched)")),
		mcp.WithString("new_title", mcp.Required(), mcp.Description("New title for the note")),
	)
}

func renameHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		newTitle, err := req.RequireString("new_title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		res, err := v.RenameNote(title, newTitle)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		parts := []string{"Renamed to: **" + res.Renamed + "**"}
		if len(res.UpdatedRefs) > 0 {
			parts = append(parts, "",
				fmt.Sprintf("Updated references in %d note(s):", len(res.UpdatedRefs)))
			for _, n := range res.UpdatedRefs {
				parts = append(parts, "  - "+n)
			}
		}
		return mcp.NewToolResultText(strings.Join(parts, "\n")), nil
	}
}

func healthTool() mcp.Tool {
	return mcp.NewTool("vault_health",
		mcp.WithDescription(`Run a health check on the vault. Reports: broken upstream links (pointing to
notes that don't exist), orphan notes (no upstream, not a topic, no children),
stale TODOs (older than 7 days), and empty notes (frontmatter only).`),
	)
}

func healthHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		report := v.GetHealth()
		stats := v.GetStats()
		parts := []string{
			"## Vault Health Report",
			fmt.Sprintf("%d notes, %d topics, %d upstream edges, %d inline mentions",
				stats.NoteCount, stats.TopicCount, stats.EdgeCount, stats.MentionCount),
			"",
		}

		parts = append(parts, fmt.Sprintf("### Broken Upstream Links (%d)", len(report.BrokenUpstream)))
		if len(report.BrokenUpstream) == 0 {
			parts = append(parts, "None — all upstream references resolve.")
		} else {
			for _, b := range report.BrokenUpstream {
				parts = append(parts, fmt.Sprintf(`- **%s** → %q (does not exist)`, b.Note, b.BrokenRef))
			}
		}
		parts = append(parts, "")

		parts = append(parts, fmt.Sprintf("### Orphan Notes (%d)", len(report.Orphans)))
		if len(report.Orphans) == 0 {
			parts = append(parts, "None — all notes are connected to the graph.")
		} else {
			for _, o := range report.Orphans {
				parts = append(parts, "- "+o)
			}
			parts = append(parts, "_These notes have no upstream and no children — consider linking them to a topic._")
		}
		parts = append(parts, "")

		parts = append(parts, fmt.Sprintf("### Stale TODOs (%d)", len(report.StaleTodos)))
		if len(report.StaleTodos) == 0 {
			parts = append(parts, "None — all TODOs are recent.")
		} else {
			for _, s := range report.StaleTodos {
				parts = append(parts, "- "+s)
			}
		}
		parts = append(parts, "")

		parts = append(parts, fmt.Sprintf("### Empty Notes (%d)", len(report.EmptyNotes)))
		if len(report.EmptyNotes) == 0 {
			parts = append(parts, "None — all notes have content.")
		} else {
			for _, e := range report.EmptyNotes {
				parts = append(parts, "- "+e)
			}
		}

		return mcp.NewToolResultText(strings.Join(parts, "\n")), nil
	}
}

func tasksTool() mcp.Tool {
	return mcp.NewTool("vault_tasks",
		mcp.WithDescription(`Extract Markdown checkbox tasks ("- [ ]" / "- [x]") from notes
across the vault. Filter by status, by topic (traverses upstream chain
like vault_list/vault_search), or by tag. Read-only.`),
		mcp.WithString("status", mcp.Description("Which tasks to include. Default: open"),
			mcp.Enum("open", "done", "all")),
		mcp.WithString("topic", mcp.Description("Only tasks in notes under this topic (traverses upstream chain)")),
		mcp.WithString("tag", mcp.Description("Only tasks in notes carrying this tag")),
	)
}

func tasksHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		status := req.GetString("status", "open")
		filters := vault.TaskFilters{
			Status: status,
			Topic:  req.GetString("topic", ""),
			Tag:    req.GetString("tag", ""),
		}
		tasks := v.ListTasks(filters)

		header := fmt.Sprintf("## Tasks (status: %s", status)
		if filters.Topic != "" {
			header += ", topic: " + filters.Topic
		}
		if filters.Tag != "" {
			header += ", tag: " + filters.Tag
		}
		header += fmt.Sprintf(")  —  %d total", len(tasks))

		if len(tasks) == 0 {
			return mcp.NewToolResultText(header + "\n\nNo tasks match the filters."), nil
		}

		type group struct {
			title string
			path  string
			items []vault.Task
		}
		order := []string{}
		groups := map[string]*group{}
		for _, t := range tasks {
			g, ok := groups[t.NoteTitle]
			if !ok {
				g = &group{title: t.NoteTitle, path: t.NotePath}
				groups[t.NoteTitle] = g
				order = append(order, t.NoteTitle)
			}
			g.items = append(g.items, t)
		}

		parts := []string{header, ""}
		for _, title := range order {
			g := groups[title]
			parts = append(parts, fmt.Sprintf("### %s  _(%s)_", g.title, g.path))
			for _, t := range g.items {
				box := "[ ]"
				if t.Done {
					box = "[x]"
				}
				parts = append(parts, "- "+box+" "+t.Text)
			}
			parts = append(parts, "")
		}
		return mcp.NewToolResultText(strings.TrimRight(strings.Join(parts, "\n"), "\n")), nil
	}
}

func tagsTool() mcp.Tool {
	return mcp.NewTool("vault_tags",
		mcp.WithDescription(`List every tag in the vault with how many notes carry it. Sorted by count
descending, tie-broken alphabetically. Optionally filter to notes whose
upstream chain reaches a given topic.`),
		mcp.WithString("topic", mcp.Description("Only count notes under this topic (traverses upstream)")),
	)
}

func tagsHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		topic := req.GetString("topic", "")
		counts := v.ListTags(topic)
		if len(counts) == 0 {
			if topic != "" {
				return mcp.NewToolResultText(fmt.Sprintf("No tags found under topic %q.", topic)), nil
			}
			return mcp.NewToolResultText("No tags found."), nil
		}

		header := fmt.Sprintf("## Tags (%d)", len(counts))
		if topic != "" {
			header = fmt.Sprintf("## Tags under %q (%d)", topic, len(counts))
		}
		lines := []string{header, ""}
		for _, tc := range counts {
			lines = append(lines, fmt.Sprintf("- %s (%d)", tc.Tag, tc.Count))
		}
		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}
}

func reindexTool() mcp.Tool {
	return mcp.NewTool("vault_reindex",
		mcp.WithDescription("Rebuild the vault index. Use after external edits (e.g. in Obsidian) to pick up changes."),
	)
}

func reindexHandler(v *vault.Vault) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := v.BuildIndex(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		stats := v.GetStats()
		return mcp.NewToolResultText(fmt.Sprintf(
			"Vault re-indexed: %d notes, %d topics, %d edges, %d inline mentions.",
			stats.NoteCount, stats.TopicCount, stats.EdgeCount, stats.MentionCount,
		)), nil
	}
}
