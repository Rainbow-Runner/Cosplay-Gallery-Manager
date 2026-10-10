// Package media contains GalleryItem-level media ordering helpers.
package media

import (
	"strconv"
	"strings"
	"unicode"
)

// NaturalLess compares normalized relative paths with digit runs interpreted
// numerically. It is deterministic and intentionally locale-independent.
func NaturalLess(left string, right string) bool {
	l := []rune(strings.ToLower(left))
	r := []rune(strings.ToLower(right))
	for li, ri := 0, 0; li < len(l) && ri < len(r); {
		if unicode.IsDigit(l[li]) && unicode.IsDigit(r[ri]) {
			lnext := digitEnd(l, li)
			rnext := digitEnd(r, ri)
			lnumber := strings.TrimLeft(string(l[li:lnext]), "0")
			rnumber := strings.TrimLeft(string(r[ri:rnext]), "0")
			if lnumber == "" {
				lnumber = "0"
			}
			if rnumber == "" {
				rnumber = "0"
			}
			if len(lnumber) != len(rnumber) {
				return len(lnumber) < len(rnumber)
			}
			if lnumber != rnumber {
				return lnumber < rnumber
			}
			if lnext-li != rnext-ri {
				return lnext-li < rnext-ri
			}
			li, ri = lnext, rnext
			continue
		}
		if l[li] != r[ri] {
			return l[li] < r[ri]
		}
		li++
		ri++
		if li == len(l) || ri == len(r) {
			return li == len(l) && ri != len(r)
		}
	}
	return strconv.Quote(left) < strconv.Quote(right)
}

func digitEnd(value []rune, start int) int {
	end := start
	for end < len(value) && unicode.IsDigit(value[end]) {
		end++
	}
	return end
}
