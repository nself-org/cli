package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type commandRegistry struct {
	Commands []struct {
		Path      string            `json:"path"`
		Name      string            `json:"name"`
		ExitCodes map[string]string `json:"exit_codes"`
	} `json:"commands"`
}

type exitCodeCommand struct {
	Path      string
	Name      string
	ExitCodes map[string]string
}

func writeExitCodesPage(path string, check bool) (bool, error) {
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	page := string(current)

	classesTable := "| Code | Class | Meaning |\n" +
		"|------|-------|---------|\n" +
		"| 0 | ok | Success. |\n" +
		"| 1 | user | Invalid input or usage. The fix is on the caller's side. This is also the default for any error with no more specific class. |\n" +
		"| 2 | infra | The host, runtime or a dependency failed (Docker down, port taken, database not running, backup failed). Retrying after fixing the host may succeed. |\n" +
		"| 3 | auth | A licence, credential or entitlement failure. |\n" +
		"| 4 | destructive_blocked | A safety gate refused a destructive action. |\n" +
		"| 10-12 | state | Reserved for documented state codes on success paths (v1.5 mode only): 10 unhealthy or failed, 11 transitional, 12 warnings only. |"

	registryBytes, err := os.ReadFile("../../.github/command-registry.json")
	if err != nil {
		registryBytes, err = os.ReadFile(".github/command-registry.json")
		if err != nil {
			return false, fmt.Errorf("read command registry: %v", err)
		}
	}

	var reg commandRegistry
	if err := json.Unmarshal(registryBytes, &reg); err != nil {
		return false, fmt.Errorf("parse command registry: %v", err)
	}

	stateMap, err := exitCodeToState()
	if err != nil {
		return false, err
	}

	var stateCodes strings.Builder
	stateCodes.WriteString("| Command | Exit | `data.state` | Meaning |\n")
	stateCodes.WriteString("|---------|------|--------------|---------|\n")

	var targetCommands []exitCodeCommand

	for _, cmd := range reg.Commands {
		if cmd.Path == "nself status" || cmd.Path == "nself doctor" {
			targetCommands = append(targetCommands, exitCodeCommand{
				Path:      cmd.Path,
				Name:      cmd.Name,
				ExitCodes: cmd.ExitCodes,
			})
		}
	}

	sort.Slice(targetCommands, func(i, j int) bool {
		return targetCommands[i].Path < targetCommands[j].Path
	})

	for _, cmd := range targetCommands {
		name := cmd.Name

		var codes []int
		for k := range cmd.ExitCodes {
			c, _ := strconv.Atoi(k)
			codes = append(codes, c)
		}
		sort.Ints(codes)

		if _, hasZero := cmd.ExitCodes["0"]; !hasZero {
			switch name {
			case "status":
				fmt.Fprintf(&stateCodes, "| `%s` | 0 | `%s` | every service is healthy |\n", name, stateMap[0])
			case "doctor":
				fmt.Fprintf(&stateCodes, "| `%s` | 0 | `%s` | every check passed |\n", name, stateMap[0])
			}
		}

		for _, c := range codes {
			st := stateMap[c]
			stStr := ""
			if st != "" {
				stStr = "`" + st + "`"
			}
			fmt.Fprintf(&stateCodes, "| `%s` | %d | %s | %s |\n", name, c, stStr, cmd.ExitCodes[strconv.Itoa(c)])
		}
	}

	page = replaceOrInsert(page, "classes", classesTable)
	page = replaceOrInsert(page, "state-codes", strings.TrimSpace(stateCodes.String()))

	if page == string(current) {
		return false, nil
	}
	if check {
		return true, nil
	}
	return true, os.WriteFile(path, []byte(page), 0o644)
}

func replaceOrInsert(body, section, content string) string {
	begin := beginGenerated(section)
	end := endGenerated(section)

	if strings.Contains(body, begin) {
		return betweenReplace(body, begin, end, "\n"+content+"\n")
	}

	lines := strings.Split(body, "\n")
	var out []string

	inClasses := false
	inState := false

	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if section == "classes" && strings.HasPrefix(l, "| Code | Class | Meaning |") {
			inClasses = true
			out = append(out, begin)
			out = append(out, content)
			out = append(out, end)
			continue
		}
		if inClasses {
			if !strings.HasPrefix(l, "|") {
				inClasses = false
				out = append(out, l)
			}
			continue
		}

		if section == "state-codes" && strings.HasPrefix(l, "| Command | Exit | `data.state` | Meaning |") {
			inState = true
			out = append(out, begin)
			out = append(out, content)
			out = append(out, end)
			continue
		}
		if inState {
			if !strings.HasPrefix(l, "|") {
				inState = false
				out = append(out, l)
			}
			continue
		}

		out = append(out, l)
	}

	return strings.Join(out, "\n")
}

func betweenReplace(body, start, end, newContent string) string {
	idxStart := strings.Index(body, start)
	if idxStart == -1 {
		return body
	}
	idxEnd := strings.Index(body[idxStart:], end)
	if idxEnd == -1 {
		return body
	}
	idxEnd += idxStart

	return body[:idxStart+len(start)] + newContent + body[idxEnd:]
}

func exitCodeToState() (map[int]string, error) {
	path := "../../cmd/commands/config_json_types.go"
	b, err := os.ReadFile(path)
	if err != nil {
		b, err = os.ReadFile("cmd/commands/config_json_types.go")
		if err != nil {
			return nil, fmt.Errorf("read config_json_types.go: %v", err)
		}
	}
	content := string(b)

	constMap := make(map[string]string)
	reConst := regexp.MustCompile(`(state[a-zA-Z0-9_]+)\s*=\s*"([^"]+)"`)
	for _, m := range reConst.FindAllStringSubmatch(content, -1) {
		constMap[m[1]] = m[2]
	}

	res := make(map[int]string)
	res[0] = constMap["stateOK"]

	reCase := regexp.MustCompile(`case\s+(state[a-zA-Z0-9_]+):\s*\n\s*return\s+(\d+)`)
	for _, m := range reCase.FindAllStringSubmatch(content, -1) {
		code, _ := strconv.Atoi(m[2])
		res[code] = constMap[m[1]]
	}
	if len(res) < 2 {
		return nil, fmt.Errorf("failed to parse state codes")
	}
	return res, nil
}
