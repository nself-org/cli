package portable

import "sort"

// Sort puts every array into its canonical order and replaces nil slices by
// empty ones, so the marshalled manifest is the same for equal content.
//
// Purpose: Constitution 4.4 deterministic output (sorted keys and arrays).
// Constraints: PK keeps the key's column order (it is data, not a set);
// maps need no work because encoding/json marshals them in key order.
func (m *Manifest) Sort() {
	m.DB.Schemas = sortedStrings(m.DB.Schemas)
	if m.DB.Tables == nil {
		m.DB.Tables = []Table{}
	}
	sort.SliceStable(m.DB.Tables, func(i, j int) bool {
		a, b := m.DB.Tables[i], m.DB.Tables[j]
		return a.Schema < b.Schema || (a.Schema == b.Schema && a.Name < b.Name)
	})
	for i := range m.DB.Tables {
		if m.DB.Tables[i].PK == nil {
			m.DB.Tables[i].PK = []string{}
		}
	}
	if m.Auth.HashAlgorithms == nil {
		m.Auth.HashAlgorithms = map[string]int{}
	}
	m.Auth.ResetRequired = sortedStrings(m.Auth.ResetRequired)
	if m.Storage.Buckets == nil {
		m.Storage.Buckets = []Bucket{}
	}
	sort.SliceStable(m.Storage.Buckets, func(i, j int) bool { return m.Storage.Buckets[i].Name < m.Storage.Buckets[j].Name })
	if m.Storage.Objects == nil {
		m.Storage.Objects = []Object{}
	}
	sort.SliceStable(m.Storage.Objects, func(i, j int) bool {
		a, b := m.Storage.Objects[i], m.Storage.Objects[j]
		return a.Bucket < b.Bucket || (a.Bucket == b.Bucket && a.Key < b.Key)
	})
	if m.Exemptions == nil {
		m.Exemptions = []Exemption{}
	}
	sort.SliceStable(m.Exemptions, func(i, j int) bool {
		a, b := m.Exemptions[i], m.Exemptions[j]
		return a.Kind < b.Kind || (a.Kind == b.Kind && a.ID < b.ID)
	})
	if m.SourceCounts == nil {
		m.SourceCounts = []SourceCount{}
	}
	sort.SliceStable(m.SourceCounts, func(i, j int) bool {
		a, b := m.SourceCounts[i], m.SourceCounts[j]
		return a.Kind < b.Kind || (a.Kind == b.Kind && a.ID < b.ID)
	})
	m.Compat = sortedStrings(m.Compat)
	if m.Files == nil {
		m.Files = []File{}
	}
	sort.SliceStable(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
}

// sortedStrings returns a sorted copy of s, never nil.
func sortedStrings(s []string) []string {
	out := append([]string{}, s...)
	sort.Strings(out)
	return out
}
