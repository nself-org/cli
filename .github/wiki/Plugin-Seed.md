# Plugin Seed

A plugin can ship an idempotent seed. `nself db seed --plugin <name>` and `nself db seed --all-plugins` run it for you, inside the plugin's own container.

## Declare it

Add a `seed` block to the plugin's manifest v2 (`plugin.json`):

```json
{
  "seed": { "command": ["my-plugin", "seed", "--idempotent"] }
}
```

`command` is an argv list, not a shell string. nself passes each element to `docker exec` as its own argument. Nothing is interpolated, so `;`, `&&` and `$VAR` have no special meaning. A command that must use a shell says so itself: `["sh", "-c", "..."]`.

The seed must be safe to run twice. Running it again must not change the row counts.

## Run it

```bash
nself db seed --plugin my-plugin        # one plugin
nself db seed --all-plugins             # every installed plugin that declares a seed
nself db seed --all-plugins --json      # one envelope
```

The argv runs in the running container of the plugin's first compose service (the first service in its `docker-compose.plugin.yml`). Start the stack first with `nself start`.

`--all-plugins` skips plugins that declare no seed and reports them as `skipped`. It runs every plugin, then fails with E250 naming the ones that failed.

## JSON output

```json
{
  "schema_version": "1",
  "command": "db seed",
  "data": { "plugins": [ { "name": "my-plugin", "result": "seeded" }, { "name": "other", "result": "skipped" } ] }
}
```

## Production safety

Plugin seeds write data. They run only on `dev`, `development`, `local` and `test` environments (or when `ENV` is empty). Any other environment, including `staging` and `prod`, is refused with E403 and exit code 4. Pass `--force` to override.

## Errors

| Code | Exit | Meaning |
|---|---|---|
| E127 | 1 | The plugin declares no `seed.command` (`--plugin` only) |
| E403 | 4 | Prod-class environment without `--force` |
| E100 | 1 | The plugin is not installed |
| E252 | 2 | The plugin's service container is not running |
| E250 | 2 | The seed command exited non-zero |
