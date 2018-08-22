package mlapp

import "k8s.io/api/core/v1"

type keyPathSorted []v1.KeyToPath

func (s keyPathSorted) Len() int {
	return len(s)
}

func (s keyPathSorted) Less(i, j int) bool {
	first := s[i]
	second := s[j]
	return first.Key < second.Key
}

func (s keyPathSorted) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
