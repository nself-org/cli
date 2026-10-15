# Command wiki links

`make wiki-check` and `make wiki-commands` validate links in every
`.github/wiki/cmd-*.md` page before generating anything. This includes legacy
plugin pages and preserved PROSE blocks. Links into `.claude` are rejected;
literal configuration examples such as `.claude/settings.json` remain valid.
Links to `nself.org/docs` must resolve to a slug in the docs navigation.
Queries, fragments and trailing slashes do not change page identity.

The check is offline. `tools/wikigen/docs-nav.txt` records the union of the
authored sidebar slugs and generated command navigation from a pinned web
commit, recorded in its header. Refresh it when adding or moving docs pages;
review the new routes against the web navigation before committing. No web
checkout or credentials are needed to run wikigen or its tests.

To refresh from a local web checkout, run this from the CLI repository root,
passing the checkout path and the reviewed commit:

```bash
python3 - /path/to/web REVIEWED_COMMIT <<'PY'
from pathlib import Path
import re, subprocess, sys
web, revision = sys.argv[1:]
revision = subprocess.check_output(['git', '-C', web, 'rev-parse', revision], text=True).strip()
sources = ['org/src/lib/docs-sidebar.ts', 'org/src/generated/docs-nav.json']
slugs = set()
for source in sources:
    body = subprocess.check_output(['git', '-C', web, 'show', revision+':'+source], text=True)
    slugs.update(re.findall(r'''["']?slug["']?\s*:\s*["']([^"']+)["']''', body))
Path('tools/wikigen/docs-nav.txt').write_text(
    '# Public docs navigation snapshot, nself-org/web@'+revision+'\n'
    +'\n'.join('# '+s for s in sources)+'\n'+'\n'.join(sorted(slugs))+'\n')
PY
```
