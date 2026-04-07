package inspect

import "github.com/zhenninglang/mantis/internal/session"

const defaultPickCount = 3
const minMessages = 10

// SelectSessions picks up to n sessions suitable for analysis, from newest to oldest.
// Filters out subagent sessions and sessions with too few messages.
// Prefers diversity across models and projects.
func SelectSessions(all []session.Session, n int) []session.Session {
	if n <= 0 {
		n = defaultPickCount
	}

	// first pass: filter candidates
	var candidates []session.Session
	for i := range all {
		s := &all[i]
		if len(s.Messages) < minMessages {
			continue
		}
		// skip subagent sessions (they have callingSessionId in first event)
		if isSubagent(s) {
			continue
		}
		candidates = append(candidates, *s)
	}

	if len(candidates) <= n {
		return candidates
	}

	// pick with diversity: try different models and projects
	var picked []session.Session
	seenModel := map[string]bool{}
	seenProject := map[string]bool{}

	// first round: prefer unique model+project combos
	for i := range candidates {
		if len(picked) >= n {
			break
		}
		if !seenModel[candidates[i].Settings.Model] || !seenProject[candidates[i].Project] {
			picked = append(picked, candidates[i])
			seenModel[candidates[i].Settings.Model] = true
			seenProject[candidates[i].Project] = true
		}
	}

	// fill remaining slots with newest sessions
	for i := range candidates {
		if len(picked) >= n {
			break
		}
		found := false
		for j := range picked {
			if picked[j].Meta.ID == candidates[i].Meta.ID {
				found = true
				break
			}
		}
		if !found {
			picked = append(picked, candidates[i])
		}
	}

	return picked
}

func isSubagent(s *session.Session) bool {
	events, err := session.ParseAllEvents(s.FilePath)
	if err != nil || len(events) == 0 {
		return false
	}
	return events[0].CallingSessionID != ""
}
