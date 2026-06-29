package featureflag

import (
	"sort"
	"strings"
	"sync"
)

type Service struct {
	mu          sync.RWMutex
	featureList []string
}

func NewService() *Service {
	return &Service{featureList: make([]string, 0)}
}

func (s *Service) Get() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	features := append([]string(nil), s.featureList...)
	sort.Strings(features)
	return features
}

func (s *Service) Put(features []string) {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(features))
	for _, feature := range features {
		feature = strings.TrimSpace(feature)
		if feature == "" {
			continue
		}
		if _, ok := seen[feature]; ok {
			continue
		}
		seen[feature] = struct{}{}
		normalized = append(normalized, feature)
	}
	sort.Strings(normalized)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.featureList = normalized
}
