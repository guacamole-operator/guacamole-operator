package set

import "golang.org/x/exp/maps"

type Set struct {
	set map[string]struct{}
}

func New() Set {
	s1 := Set{}
	s1.set = make(map[string]struct{})
	return s1
}

func FromSlice(v []string) Set {
	s := New()

	for _, x := range v {
		s.Add(x)
	}

	return s
}

func (s *Set) Add(v string) {
	s.set[v] = struct{}{}
}

func (s *Set) Has(v string) bool {
	if _, ok := s.set[v]; ok {
		return true
	}

	return false
}

func (s *Set) ToSlice() []string {
	return maps.Keys(s.set)
}
