package main

import (
	"slices"
	"strings"
)

type lineMatch struct{ left, right int }
type lineEdit struct {
	start, end int
	lines      []string
}

// Match equal lines in order. Prefixes and suffixes keep typical Markdown edits
// cheap. For wide rewrites, use ordered anchors to bound time and memory.
func matchLines(left, right []string) []lineMatch {
	prefix := 0
	for prefix < min(len(left), len(right)) && left[prefix] == right[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < min(len(left), len(right))-prefix &&
		left[len(left)-suffix-1] == right[len(right)-suffix-1] {
		suffix++
	}
	matches := make([]lineMatch, 0, prefix+suffix)
	for i := range prefix {
		matches = append(matches, lineMatch{i, i})
	}
	a, b := left[prefix:len(left)-suffix], right[prefix:len(right)-suffix]
	if len(a) > 0 && len(b) > 0 && int64(len(a)+1)*int64(len(b)+1) <= 1_000_000 {
		width := len(b) + 1
		table := make([]int, (len(a)+1)*width)
		for i := len(a) - 1; i >= 0; i-- {
			for j := len(b) - 1; j >= 0; j-- {
				cell := i*width + j
				if a[i] == b[j] {
					table[cell] = 1 + table[cell+width+1]
				} else {
					table[cell] = max(table[cell+1], table[cell+width])
				}
			}
		}
		for i, j := 0, 0; i < len(a) && j < len(b); {
			switch {
			case a[i] == b[j]:
				matches = append(matches, lineMatch{prefix + i, prefix + j})
				i++
				j++
			case table[(i+1)*width+j] >= table[i*width+j+1]:
				i++
			default:
				j++
			}
		}
	} else if len(a) > 0 && len(b) > 0 {
		positions := make(map[string][]int)
		for i, line := range b {
			positions[line] = append(positions[line], i)
		}
		next := 0
		for i, line := range a {
			indexes := positions[line]
			index, _ := slices.BinarySearch(indexes, next)
			if index < len(indexes) {
				matches = append(matches, lineMatch{prefix + i, prefix + indexes[index]})
				next = indexes[index] + 1
			}
		}
	}
	for i := suffix; i > 0; i-- {
		matches = append(matches, lineMatch{len(left) - i, len(right) - i})
	}
	return matches
}

func lineChanges(base, updated []string) []lineEdit {
	var edits []lineEdit
	left, right := 0, 0
	matches := matchLines(base, updated)
	matches = append(matches, lineMatch{len(base), len(updated)})
	for _, match := range matches {
		if left != match.left || right != match.right {
			edits = append(edits, lineEdit{left, match.left, updated[right:match.right]})
		}
		left, right = match.left+1, match.right+1
	}
	return edits
}

func before(a, b lineEdit) bool {
	// Insertions touching a replacement boundary are independent. Two insertions
	// at the same position must be combined.
	return a.end < b.start || (a.end == b.start && a.start != b.end)
}

func applyLines(base []string, start, end int, edits []lineEdit) []string {
	var out []string
	for _, edit := range edits {
		out = append(out, base[start:edit.start]...)
		out = append(out, edit.lines...)
		start = edit.end
	}
	return append(out, base[start:end]...)
}

// Keep every line in both overlapping changes in each side's original order.
// Equal lines can share a position, including when one side is a subset.
func combineLines(out, current, incoming []string) []string {
	if slices.Equal(current, incoming) {
		return append(out, current...)
	}
	left, right := 0, 0
	for _, match := range matchLines(current, incoming) {
		out = append(out, current[left:match.left]...)
		out = append(out, incoming[right:match.right]...)
		out = append(out, current[match.left])
		left, right = match.left+1, match.right+1
	}
	out = append(out, current[left:]...)
	return append(out, incoming[right:]...)
}

func markdownLines(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func mergeMarkdown(base, current, incoming string) string {
	switch {
	case current == base:
		return incoming
	case incoming == base || current == incoming:
		return current
	}
	baseLines := markdownLines(base)
	left := lineChanges(baseLines, markdownLines(current))
	right := lineChanges(baseLines, markdownLines(incoming))
	out := make([]string, 0, len(baseLines))
	position := 0
	for len(left) > 0 || len(right) > 0 {
		var edit lineEdit
		switch {
		case len(right) == 0 || (len(left) > 0 && before(left[0], right[0])):
			edit, left = left[0], left[1:]
		case len(left) == 0 || before(right[0], left[0]):
			edit, right = right[0], right[1:]
		default:
			start := min(left[0].start, right[0].start)
			end := max(left[0].end, right[0].end)
			i, j := 1, 1
			for {
				oldEnd := end
				for i < len(left) && left[i].start < end {
					end = max(end, left[i].end)
					i++
				}
				for j < len(right) && right[j].start < end {
					end = max(end, right[j].end)
					j++
				}
				if end == oldEnd {
					break
				}
			}
			out = append(out, baseLines[position:start]...)
			out = combineLines(out, applyLines(baseLines, start, end, left[:i]),
				applyLines(baseLines, start, end, right[:j]))
			position, left, right = end, left[i:], right[j:]
			continue
		}
		out = append(out, baseLines[position:edit.start]...)
		out = append(out, edit.lines...)
		position = edit.end
	}
	return strings.Join(append(out, baseLines[position:]...), "\n")
}
