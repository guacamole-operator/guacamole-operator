package set

// Difference of two sets. Returned are elements in s1
// that are not in s2.
func Difference(s1, s2 Set) Set {
	diff := New()
	for x := range s1.set {
		if _, found := s2.set[x]; !found {
			diff.set[x] = struct{}{}
		}
	}
	return diff
}
