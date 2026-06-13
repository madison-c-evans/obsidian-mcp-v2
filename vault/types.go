package vault

type Frontmatter struct {
	Tags          []string
	ExternalLinks []string
	Upstream      []string
	Date          string
	Status        string
	Description   string
}

type Note struct {
	Title        string
	FileName     string
	FilePath     string
	RelativePath string
	Frontmatter  Frontmatter
	Content      string
	IsTopic      bool
}

type GraphContext struct {
	Ancestors   []*Note
	Descendants []*Note
	Siblings    []*Note
	Mentions    []*Note
	MentionedBy []*Note
}

type BrokenRef struct {
	Note      string
	BrokenRef string
}

type HealthReport struct {
	BrokenUpstream []BrokenRef
	Orphans        []string
	StaleTodos     []string
	EmptyNotes     []string
}

type SearchOptions struct {
	Scope        string
	Topic        string
	Status       string
	Tag          string
	IncludeGraph bool
	GraphDepth   int
	MaxResults   int
}

type SearchResult struct {
	Note         *Note
	Score        float64
	MatchType    string
	Excerpt      string
	GraphContext *GraphContext
}

type CreateOptions struct {
	Title       string
	Upstream    []string
	Tags        []string
	HasTags     bool
	Status      string
	Content     string
	IsTopic     bool
	Description string
	HasDesc     bool
}

type FrontmatterUpdate struct {
	Tags             []string
	HasTags          bool
	ExternalLinks    []string
	HasExternalLinks bool
	Upstream         []string
	HasUpstream      bool
	Status           string
	HasStatus        bool
	Description      string
	HasDesc          bool
}

type Stats struct {
	NoteCount    int
	TopicCount   int
	EdgeCount    int
	MentionCount int
}

type TagCount struct {
	Tag   string
	Count int
}
