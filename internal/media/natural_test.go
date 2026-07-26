package media

import (
	"sort"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	values := []string{"10.jpg", "2.jpg", "001.jpg", "1.jpg", "set/20.jpg", "set/3.jpg"}
	sort.Slice(values, func(i int, j int) bool { return NaturalLess(values[i], values[j]) })
	want := []string{"1.jpg", "001.jpg", "2.jpg", "10.jpg", "set/3.jpg", "set/20.jpg"}
	for index := range want {
		if values[index] != want[index] {
			t.Fatalf("natural order = %#v, want %#v", values, want)
		}
	}
}
