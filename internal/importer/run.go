package importer

// Purpose: preflight, extraction, ordered load, verification and durable report.
// Inputs: source and started target. Outputs: pass only after zero mismatches.
// Constraints: plan creates no directories; upstream errors cannot expose secrets.
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer/load"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/portable"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var started = load.Started
var targetChecks = checkTarget
var lockTarget = acquireLock

// acquireLock holds the import advisory lock on a dedicated target connection until release is
// called, so two imports cannot both pass the emptiness check.
func acquireLock(ctx context.Context, cfg *config.Config) (func(), error) {
	db, err := load.Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = load.TryLock(ctx, db); err != nil {
		_ = db.Close(ctx)
		return nil, err
	}
	return func() { _ = db.Close(context.Background()) }, nil
}

// checkTarget runs every read-only target preflight that needs no bundle, so --plan and the real
// run refuse the same targets: emptiness (E519) and, when the source has users, auth.users. The
// bundle-dependent checks (metadata conflicts, function and type collisions) need the dump and
// run in load.Apply before its first write.
func checkTarget(ctx context.Context, cfg *config.Config, d Detection) error {
	db, err := load.Connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close(ctx) }()
	store, err := load.Store(cfg)
	if err != nil {
		return err
	}
	if err = load.Empty(ctx, db, d.Schemas, store != nil); err != nil {
		return err
	}
	return load.AuthPreflight(ctx, db, d.SourceUsers)
}

// planObjects runs the object-store preflights of the real run (E529 missing store, E519 existing
// keys, D-0723) during --plan, so a plan never reports pass for a bundle the run would refuse.
func planObjects(ctx context.Context, cfg *config.Config, d Detection, skip bool) error {
	if skip || len(d.Objects) == 0 {
		return nil
	}
	store, err := load.Store(cfg)
	if err != nil {
		return err
	}
	m := portable.Manifest{Storage: portable.Storage{Objects: d.Objects}}
	if err = load.StoragePreflight(m, store, skip); err != nil || store == nil {
		return err
	}
	return load.StorageCollisions(ctx, m, store, skip)
}
func Run(ctx context.Context, cfg *config.Config, s Source, in Input, opts RunOptions) (Report, error) {
	report := Report{Result: "failed", Tables: []TableReport{}, Exemptions: []portable.Exemption{}, Mismatches: []verify.Mismatch{}, UntranslatedPolicies: []Policy{}, CompatShims: []string{}, Hasura: HasuraReport{Diffs: []verify.Mismatch{}}}
	if s == nil {
		return report, errs.New("E524", "source not registered")
	}
	report.Source = s.Name()
	registered := false
	for _, source := range Sources() {
		if source.Name() == s.Name() {
			registered = true
		}
	}
	if !registered {
		return report, errs.New("E524", "source not registered")
	}
	if cfg == nil {
		return report, errs.New("E521", "target config required")
	}
	dir := opts.ProjectDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return report, errs.New("E521", "target project directory unavailable")
		}
	}
	if err := started(ctx, dir); err != nil {
		return report, err
	}
	if !opts.Plan {
		release, e := lockTarget(ctx, cfg)
		if e != nil {
			return report, e
		}
		defer release()
	}
	in.Target = cfg
	detection, err := s.Detect(ctx, in)
	if err != nil {
		return report, publicError(err, "E517", "source detection failed")
	}
	if err = targetChecks(ctx, cfg, detection); err != nil {
		return report, err
	}
	if opts.Plan {
		if err = planObjects(ctx, cfg, detection, opts.SkipStorage); err != nil {
			return report, err
		}
	}
	report.Detection = &detection
	report.Steps = []string{"extract snapshot", "check portable bundle", "restore pre-data and data", "load auth users", "load storage", "restore post-data", "apply additive metadata", "verify zero mismatches"}
	if opts.Plan {
		report.Result = "pass"
		return report, nil
	}
	nonce := make([]byte, 8)
	if _, err = rand.Read(nonce); err != nil {
		return report, errs.New("E520", "run identifier unavailable")
	}
	report.RunID = time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(nonce)
	root := filepath.Join(dir, ".nself", "import", report.RunID)
	stage := filepath.Join(root, "stage")
	if err = os.MkdirAll(root, 0o700); err != nil {
		return report, errs.New("E520", "import directory unavailable")
	}
	defer func() {
		if report.Result == "pass" && !opts.KeepStage {
			_ = os.RemoveAll(stage)
		}
	}()
	extracted, err := s.Extract(ctx, in, stage)
	report.UntranslatedPolicies = append(report.UntranslatedPolicies, extracted.UntranslatedPolicies...)
	if err != nil {
		err = publicError(err, "E520", "source extraction failed")
	} else {
		err = runBundle(ctx, cfg, stage, opts, &report)
	}
	b, e := json.MarshalIndent(report, "", "  ")
	if e == nil {
		e = os.WriteFile(filepath.Join(root, "report.json"), append(b, '\n'), 0o600)
	}
	if e != nil {
		return report, errs.New("E521", "import report write failed")
	}
	return report, withReportHint(err, report.RunID, report.Wrote)
}
func runBundle(ctx context.Context, cfg *config.Config, stage string, opts RunOptions, report *Report) error {
	bundle, err := portable.Open(stage)
	if err != nil {
		return errs.New("E520", "extracted bundle failed integrity checks")
	}
	m := bundle.Manifest()
	progress := &load.Progress{}
	defer func() { report.Wrote, report.LastStep = progress.Wrote, progress.Step }()
	exemptions, err := load.Apply(ctx, cfg, bundle, load.Options{SkipStorage: opts.SkipStorage, Progress: progress})
	if err != nil {
		return err
	}
	report.CompatShims = append(report.CompatShims, m.Compat...)
	report.Exemptions = append(m.Exemptions, exemptions...)
	m.Exemptions = report.Exemptions
	db, err := load.Connect(ctx, cfg)
	if err != nil {
		return errs.New("E522", "target database unavailable for verification")
	}
	defer func() { _ = db.Close(ctx) }()
	store, err := load.Store(cfg)
	if err != nil {
		return errs.New("E522", "target store unavailable for verification")
	}
	users, err := verify.ReadUsers(bundle)
	if err != nil {
		return errs.New("E522", "bundle users unavailable")
	}
	metadata, err := verify.ReadMetadata(bundle)
	if err != nil {
		return errs.New("E522", "bundle metadata unavailable")
	}
	live := &verify.Live{DB: db, Config: cfg, Store: store, Users: users, Metadata: metadata, HasAuth: verify.HasMember(bundle, "auth/users.jsonl")}
	report.Mismatches, err = verify.Compare(ctx, live, m)
	if err != nil {
		return errs.New("E522", "target verification failed")
	}
	for _, t := range m.DB.Tables {
		got := live.Observed.Tables[t.Schema+"."+t.Name]
		report.Tables = append(report.Tables, TableReport{Schema: t.Schema, Name: t.Name, SourceRows: t.Rows, TargetRows: got.Rows, Equal: t.Rows == got.Rows && t.Hash == got.Hash})
	}
	report.Users = UsersReport{Source: m.Auth.Users, Target: len(live.Observed.Users), Equal: m.Auth.Users == len(live.Observed.Users), ResetRequired: []ResetUser{}}
	for _, u := range users {
		if u.HashAlgorithm != "bcrypt" && u.HashAlgorithm != "none" {
			report.Users.ResetRequired = append(report.Users.ResetRequired, ResetUser{ID: u.ID, Email: deref(u.Email)})
		}
	}
	report.Storage = StorageReport{SourceObjects: len(m.Storage.Objects), TargetObjects: len(live.Observed.Objects), Equal: len(m.Storage.Objects) == len(live.Observed.Objects)}
	report.Hasura.Equal = true
	for _, diff := range report.Mismatches {
		if diff.Kind == "permission" || diff.Kind == "relationship" || diff.Kind == "table_not_tracked" {
			report.Hasura.Equal = false
			report.Hasura.Diffs = append(report.Hasura.Diffs, diff)
		}
	}
	if len(report.Mismatches) > 0 {
		report.Result = "mismatch"
		return errs.New("E523", "import verification found mismatches")
	}
	report.Result = "pass"
	return nil
}

// safeMessages are the fixed, credential-free texts a source or loader may surface unchanged.
var safeMessages = []string{"source requires an environment variable name", "source environment variable is empty", "source connection configuration invalid", "source database unreachable", "source version unavailable", "source schema lookup failed", "source auth lookup failed", "target database connection failed", "target version unavailable", "source snapshot failed", "source snapshot export failed", "source tables unavailable", "source table hashing failed", "source key lookup failed", "independent source count failed", "target image unavailable", "snapshot staging unavailable", "pg_dump snapshot extraction failed", "bundle creation failed", "dump member write failed", "auth snapshot extraction failed", "snapshot commit failed", "bundle manifest write failed", "source partition lookup failed", "bundle directory required", "bundle staging failed", "bundle changed while it was staged", "target config required for snapshot extraction", "source has triggers that call Supabase-owned functions; drop or replace them in the source first", "source has users whose email or phone repeats (case-insensitive); fix them in the source first", "storage object does not match the storage catalog", "source is not a Supabase database (auth.users with encrypted_password is missing)", "source storage lookup failed", "source policy lookup failed", "storage api url invalid", "storage api request failed", "storage listing differs from the storage catalog", "storage bucket name not valid for the target object store", "storage object name not valid for the bundle", "source has storage objects; --api-url (or --project-ref) and --service-key-env are required"}

var (
	versionRe   = regexp.MustCompile(`POSTGRES_VERSION=([0-9]{1,3})`)
	oldSourceRe = regexp.MustCompile(`major ([0-9]{1,3}) is older than 12`)
)

func publicError(err error, code, message string) error {
	// Preserve only known source failure codes, with fresh credential-free text.
	var safe *SafeError
	if errors.As(err, &safe) && passesText(safe.Code) && plainText(safe.What) {
		return errs.New(safe.Code, safe.What)
	}
	var cli *errs.CLIError
	if errors.As(err, &cli) {
		// A portable bundle error names a member path or manifest value; keep it unless it could garble a terminal.
		if cli.Code == "E515" || cli.Code == "E516" {
			if plainText(cli.What) {
				return errs.New(cli.Code, cli.What)
			}
			return errs.New(cli.Code, "portable bundle failed its check")
		}
		for _, c := range []string{"E517", "E518", "E520", "E521"} {
			if cli.Code == c {
				if isSafeMessage(cli.What) {
					return errs.New(c, cli.What)
				}
				if c == "E520" && strings.HasPrefix(cli.What, PartitionedPrefix) && safeName(strings.TrimPrefix(cli.What, PartitionedPrefix)) {
					return errs.New(c, cli.What)
				}
				if c == "E518" {
					if match := oldSourceRe.FindStringSubmatch(cli.What); len(match) == 2 {
						return errs.New(c, "source Postgres major "+match[1]+" is older than 12; upgrade the source to Postgres 12 or newer")
					}
					if match := versionRe.FindStringSubmatch(cli.What); len(match) == 2 {
						return errs.New(c, "source requires POSTGRES_VERSION="+match[1]+" (Postgres 12 or later)")
					}
					return errs.New(c, "source requires POSTGRES_VERSION matching its major (Postgres 12 or later)")
				}
				return errs.New(c, message)
			}
		}
	}
	return errs.New(code, message)
}

// plainText reports whether s is short, valid UTF-8 and free of control and format (bidi) characters.
func plainText(s string) bool {
	return len(s) <= 300 && utf8.ValidString(s) && strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) < 0
}
