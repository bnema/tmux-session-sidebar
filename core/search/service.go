package search

import (
	"sort"
	"strings"
)

type Item struct {
	ID    string
	Label string
}

type MatchResult struct {
	Item  Item
	Score int
}

const (
	scoreExact       = 400
	scorePrefix      = 300
	scoreSubstring   = 200
	scoreSubsequence = 100
)

func Match(items []Item, query string) []MatchResult {
	query = strings.ToLower(strings.TrimSpace(query))
	results := make([]MatchResult, 0, len(items))
	for _, item := range items {
		score := scoreItem(item, query)
		if score == 0 {
			continue
		}
		results = append(results, MatchResult{Item: item, Score: score})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Item.Label < results[j].Item.Label
		}
		return results[i].Score > results[j].Score
	})
	return results
}

func scoreItem(item Item, query string) int {
	if query == "" {
		return scoreSubstring
	}
	label := strings.ToLower(item.Label)
	switch {
	case label == query:
		return scoreExact
	case strings.HasPrefix(label, query):
		return scorePrefix
	case strings.Contains(label, query):
		return scoreSubstring
	case isSubsequence(label, query):
		return scoreSubsequence
	default:
		return 0
	}
}

func isSubsequence(label string, query string) bool {
	queryRunes := []rune(query)
	if len(queryRunes) == 0 {
		return true
	}
	qi := 0
	for _, r := range label {
		if queryRunes[qi] == r {
			qi++
			if qi == len(queryRunes) {
				return true
			}
		}
	}
	return false
}
