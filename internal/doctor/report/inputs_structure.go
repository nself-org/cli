package report

// inputs_structure.go: public structure names are never scrub literals (lead
// ruling R7).
//
// Purpose: a default such as ${POSTGRES_PASSWORD:-postgres} would make the
// literal pass redact every "postgres" service, image and log name, and the
// bundle would be useless for support. A learned value that equals (case
// insensitive) a compose service name, a container_name or a path segment of
// an image repository of the stack is public structure: it is dropped from the
// literal set and counted in one manifest note. The pattern and assignment
// passes still apply to it.
// Inputs: the compose documents already parsed (docker-compose.yml, fragments).
// Outputs: a name set on the input set; the dropped count.
// Constraints: only the spelling that equals a structure name is dropped; the
//         other spellings of the value stay.

import (
	"fmt"
	"strings"
)

// noteStructure records the structure names of one compose document.
func (in *inputSet) noteStructure(doc any) {
	root, _ := doc.(map[string]any)
	services, _ := root["services"].(map[string]any)
	for name, v := range services {
		in.addStructure(name)
		svc, _ := v.(map[string]any)
		if cn, ok := svc["container_name"].(string); ok {
			in.addStructure(cn)
		}
		if img, ok := svc["image"].(string); ok {
			for _, seg := range imageSegments(img) {
				in.addStructure(seg)
			}
		}
	}
}

func (in *inputSet) addStructure(name string) {
	if name = strings.ToLower(strings.TrimSpace(name)); name != "" && !strings.Contains(name, "$") {
		if in.structure == nil {
			in.structure = map[string]bool{}
		}
		in.structure[name] = true
	}
}

// imageSegments returns the path segments of an image reference's repository:
// "ghcr.io/org/app:1.2@sha256:x" gives ghcr.io, org, app.
func imageSegments(img string) []string {
	if i := strings.Index(img, "@"); i >= 0 {
		img = img[:i]
	}
	if i := strings.LastIndex(img, ":"); i > strings.LastIndex(img, "/") {
		img = img[:i]
	}
	return strings.Split(img, "/")
}

// dropStructureValues removes the structure-equal spellings of the learned
// values (only those: a URL-credential value keeps its Basic-auth forms) and
// returns how many values lost a spelling.
func (in *inputSet) dropStructureValues() int {
	kept := in.values[:0]
	dropped := 0
	for _, sv := range in.values {
		var vs []string
		for _, v := range sv.Variants {
			if !in.structure[strings.ToLower(v)] {
				vs = append(vs, v)
			}
		}
		if len(vs) != len(sv.Variants) {
			dropped++
		}
		if len(vs) > 0 {
			sv.Variants = vs
			kept = append(kept, sv)
		}
	}
	in.values = kept
	return dropped
}

// structureNote is the manifest note for n dropped values, or "".
func structureNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d learned values equal service/image names and were not used as literals", n)
}
