package highlight

import (
	"sort"
	"strings"
)

// Fragment stores a segment of text with an emphasis flag.
type Fragment struct {
	Text       string
	Emphasized bool
}

// BuildFragments splits the provided line by highlight ranges.
func BuildFragments(line string, spans [][2]int) []Fragment {
	if len(spans) == 0 {
		return []Fragment{{Text: line}}
	}

	sort.Slice(spans, func(i, j int) bool {
		return spans[i][0] < spans[j][0]
	})

	fragments := make([]Fragment, 0, len(spans)*2+1)
	cursor := 0
	for _, span := range spans {
		start := clamp(span[0], 0, len(line))
		end := clamp(span[1], 0, len(line))
		if start > cursor {
			fragments = appendFragment(fragments, Fragment{Text: line[cursor:start]})
		}
		if end > start {
			fragments = appendFragment(fragments, Fragment{Text: line[start:end], Emphasized: true})
		}
		cursor = end
	}
	if cursor < len(line) {
		fragments = appendFragment(fragments, Fragment{Text: line[cursor:]})
	}
	return fragments
}

func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func appendFragment(list []Fragment, frag Fragment) []Fragment {
	if frag.Text == "" {
		return list
	}
	// Merge with previous fragment if the emphasis flag matches.
	if len(list) > 0 {
		last := &list[len(list)-1]
		if last.Emphasized == frag.Emphasized {
			last.Text = last.Text + frag.Text
			return list
		}
	}
	list = append(list, frag)
	return list
}

// String renders the fragments into plain text, ignoring emphasis.
func String(frags []Fragment) string {
	var b strings.Builder
	for _, f := range frags {
		b.WriteString(f.Text)
	}
	return b.String()
}
