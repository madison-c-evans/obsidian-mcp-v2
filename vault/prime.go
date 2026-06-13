package vault

import "sort"

// TopicSummary describes a topic hub for the prime overview: its curated
// description, any parent topics it nests under, and how many notes sit under
// it (transitively, via the topic closure).
type TopicSummary struct {
	Title        string
	Description  string
	ParentTopics []string
	NoteCount    int
}

// StatusCount is a status value present in the vault and how many notes carry it.
type StatusCount struct {
	Status string
	Count  int
}

// PrimeData is the one-shot orientation payload: vault stats, the topic
// ontology (biggest areas first), and the status vocabulary actually in use.
type PrimeData struct {
	Stats    Stats
	Topics   []TopicSummary
	Statuses []StatusCount
}

func (v *Vault) GetPrimeData() PrimeData {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Transitive member count per topic (exclude the topic's own self-entry).
	counts := make(map[string]int)
	for noteTitle, topics := range v.topicsOf {
		for topic := range topics {
			if topic == noteTitle {
				continue
			}
			counts[topic]++
		}
	}

	var topics []TopicSummary
	statusCounts := make(map[string]int)
	for title, n := range v.notes {
		if isTemplatePath(n.RelativePath) {
			continue
		}
		if n.IsTopic {
			var parents []string
			for _, up := range n.Frontmatter.Upstream {
				if p, ok := v.notes[up]; ok && p.IsTopic {
					parents = append(parents, up)
				}
			}
			sort.Strings(parents)
			topics = append(topics, TopicSummary{
				Title:        title,
				Description:  n.Frontmatter.Description,
				ParentTopics: parents,
				NoteCount:    counts[title],
			})
			continue
		}
		if s := n.Frontmatter.Status; s != "" {
			statusCounts[s]++
		}
	}

	sort.Slice(topics, func(i, j int) bool {
		if topics[i].NoteCount != topics[j].NoteCount {
			return topics[i].NoteCount > topics[j].NoteCount
		}
		return topics[i].Title < topics[j].Title
	})

	statuses := make([]StatusCount, 0, len(statusCounts))
	for s, c := range statusCounts {
		statuses = append(statuses, StatusCount{Status: s, Count: c})
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Count != statuses[j].Count {
			return statuses[i].Count > statuses[j].Count
		}
		return statuses[i].Status < statuses[j].Status
	})

	return PrimeData{Stats: v.getStatsLocked(), Topics: topics, Statuses: statuses}
}
