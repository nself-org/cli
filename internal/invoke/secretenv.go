package invoke

// Purpose: keep secret request values off the child argv (D-0468). The
// invoker renders each secret value as a placeholder, hands the values to the
// child in the SecretsEnv environment variable, and the child puts them back
// before cobra parses its argv (RestoreSecretArgv).
// Inputs: an argv with secret positions marked; the child's os.Args.
// Outputs: placeholder argv plus a SecretBundle; the restored argv and values.
// Constraints: argv is readable by every local user through ps, the environment
// of a process only by the same uid and root. The placeholder is
// "nself-invoke-secret:<nonce>:<n>", nonce = 16 random hex chars per child, n
// = index into the bundle values. A flag keeps its "--name=" prefix and only
// the text after it is replaced. The child unsets SecretsEnv at once so no
// grandchild inherits it, and ignores it unless InvokedByEnv is set. Any
// mismatch (bad JSON, bad index, a value unused or used twice) is E421.
// MaxSecretEnvBytes caps the bundle per OS (secretenv_limit_*.go).
// Mounted plugin calls with restored secret values are refused with E421
// before proxying: the plugin protocol has no hand-off for the bundle yet
// (D-0651). A free-form plugin command (no declared
// args or flags) is also not hidden: nothing marks its words secret, so
// Request.Argv goes on the child argv as given (argv.go).

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// placeholderPrefix starts every secret placeholder.
const placeholderPrefix = "nself-invoke-secret:"

// SecretBundle is the secret values of one child run: Values[n] is what the
// placeholder with index n stands for.
type SecretBundle struct {
	Nonce  string   `json:"nonce"`
	Values []string `json:"values"`
}

// hideSecrets swaps the text of argv[i][prefix:] for a placeholder for every
// i in marks (prefix = bytes to keep). Empty values stay as they are.
func hideSecrets(argv []string, marks map[int]int) ([]string, *SecretBundle, error) {
	var b *SecretBundle
	for i := range argv {
		prefix, ok := marks[i]
		if !ok || prefix > len(argv[i]) || len(argv[i]) == prefix {
			continue
		}
		if b == nil {
			var raw [8]byte
			if _, err := rand.Read(raw[:]); err != nil {
				return nil, nil, errs.New("E422", "could not hide a secret value").WithWhy("the system random source failed")
			}
			b = &SecretBundle{Nonce: hex.EncodeToString(raw[:])}
		}
		b.Values = append(b.Values, argv[i][prefix:])
		argv[i] = argv[i][:prefix] + placeholderPrefix + b.Nonce + ":" + strconv.Itoa(len(b.Values)-1)
	}
	if b != nil {
		if js, err := json.Marshal(b); err != nil || len(js) > MaxSecretEnvBytes {
			return nil, nil, badRequest("the secret values are larger than %d bytes", MaxSecretEnvBytes)
		}
	}
	return argv, b, nil
}

// scrubSet is the list runChild scrubs from a child's stderr: the request
// secrets plus the values of the bundle as rendered (an integer flag sent as
// 007 reaches the child as 7, which is what the child can echo). Deduplicated,
// longest first.
func scrubSet(secrets []string, b *SecretBundle) []string {
	all := append([]string{}, secrets...)
	if b != nil {
		all = append(all, b.Values...)
	}
	seen := make(map[string]bool, len(all))
	out := all[:0]
	for _, v := range all {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// withSecretsEnv adds the SecretsEnv entry for b (when it holds values) to env.
func withSecretsEnv(env []string, b *SecretBundle) ([]string, error) {
	if b == nil || len(b.Values) == 0 {
		return env, nil
	}
	js, err := json.Marshal(b)
	if err != nil {
		return nil, errs.New("E422", "could not encode the secret values").WithWhy(err.Error())
	}
	return append(env, SecretsEnv+"="+string(js)), nil
}

// RestoreSecretArgv is the child side of the placeholder scheme. It always
// unsets SecretsEnv. It returns argv unchanged unless the invoker started this
// process (InvokedByEnv set) and passed a bundle; then every placeholder
// element, or the text after "--name=", is replaced by its value. values lists
// every restored secret, for masking in the command log.
func RestoreSecretArgv(argv []string) (out []string, values []string, err error) {
	raw, present := os.LookupEnv(SecretsEnv)
	_ = os.Unsetenv(SecretsEnv)
	if !present || raw == "" || os.Getenv(InvokedByEnv) == "" {
		return argv, nil, nil
	}
	fail := func(why string) ([]string, []string, error) {
		return argv, nil, errs.New("E421", "the secret values passed to this command are unusable").WithWhy(why)
	}
	var b SecretBundle
	if jerr := json.Unmarshal([]byte(raw), &b); jerr != nil || b.Nonce == "" {
		return fail("the " + SecretsEnv + " value is not a valid secret bundle")
	}
	used := make([]bool, len(b.Values))
	lead := placeholderPrefix + b.Nonce + ":"
	out = make([]string, len(argv))
	for i, a := range argv {
		prefix, ph := "", a
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, lead) {
			if name, rest, ok := strings.Cut(a, "="); ok {
				prefix, ph = name+"=", rest
			}
		}
		if !strings.HasPrefix(ph, lead) {
			out[i] = a
			continue
		}
		n, aerr := strconv.Atoi(ph[len(lead):])
		if aerr != nil || n < 0 || n >= len(b.Values) || strconv.Itoa(n) != ph[len(lead):] {
			return fail("an argument refers to a secret value that was not passed")
		}
		if used[n] {
			return fail("a secret value is referenced twice")
		}
		used[n] = true
		out[i] = prefix + b.Values[n]
	}
	for _, u := range used {
		if !u {
			return fail("a secret value was passed but no argument uses it")
		}
	}
	return out, b.Values, nil
}
