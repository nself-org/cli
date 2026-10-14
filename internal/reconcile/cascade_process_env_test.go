package reconcile_test

import "testing"

// cascadeProcessEnvProd covers an operator selecting prod over a dev base file.
func cascadeProcessEnvProd(t *testing.T, bin string, mode cascadeMode) {
	p := newCascadeProject(t, bin, "dev-minimal", mode.v15)
	p.processEnv = "prod"
	p.write(".env.prod", "BASE_DOMAIN=fromprod.example\n")
	before := p.snapshot()
	pl := p.planJSON()
	if pl.Env != "prod" || pl.EnvClass != "prod" || !pl.RequiresConfirmation {
		t.Fatalf("process ENV=prod plan: env=%q class=%q confirmation=%v", pl.Env, pl.EnvClass, pl.RequiresConfirmation)
	}
	p.assertUnchanged(before, "process ENV=prod plan")
	if mode.v15 {
		r := p.run("build", "--json")
		if r.code != 4 || cascadeErrCode(r.stdout) != "E403" {
			t.Fatalf("unconfirmed process ENV=prod build: exit %d\n%s", r.code, r.all())
		}
		p.assertUnchanged(before, "process ENV=prod refusal")
		p.mustRun("build", "--yes")
	} else {
		r := p.mustRun("build")
		cascadeContains(t, "v1.4 process ENV=prod notice", r.stderr, "v1.5 will require --yes")
	}
	cascadeContains(t, "confirmed prod cascade", p.read("docker-compose.yml"), "fromprod.example")
}
