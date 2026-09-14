# everos-tools

Official EverOS developer tools for AI coding assistants.

## Available Plugins

### everos-sdk-upgrade

Migrate an EverOS Cloud integration between API/SDK versions.

- **Python SDK** (`everos-cloud` / `evermemos`) — full rule coverage
- **Raw HTTP callers in any language** — endpoint, payload and response rules
- Detects the current version and chains rules to the target
- **Flags capabilities that have no equivalent in the target version** instead of
  silently dropping or approximating them
- `--scan` mode produces an impact report without editing anything

## Installation (Claude Code)

```bash
# 1. Add marketplace (one-time)
/plugin marketplace add EverMind-AI/everos-tools

# 2. Install the plugin
/plugin install everos-sdk-upgrade@everos-tools

# 3. See what a migration would involve, without changing anything
/everos-sdk-upgrade --scan

# 4. Run it
/everos-sdk-upgrade

# 5. Update to the latest rules
/plugin marketplace update
```

## Other AI Tools (Cursor, GitHub Copilot, Codex, Gemini CLI, Cline, Amp, Warp, Goose, Junie, and 45+ supported)

This skill follows the [Agent Skills](https://agentskills.io) open standard. Install with one command:

```bash
npx skills add https://github.com/EverMind-AI/everos-tools
```

The CLI auto-detects your installed tools and copies the skill to the correct directories.

## What it does to your repository

- **`--scan` writes nothing.** It reads your code and prints a report. Use it first.
- **It checks before it edits.** A pre-flight gate runs before the first change: your
  Python version against the target's floor, whether your EverOS calls are on an async
  path, how many capabilities have no v2 equivalent, and whether your working tree already
  has uncommitted work in the files it is about to touch. Any of those can stop the run.
- **It takes a snapshot first**, and recommends a branch, so the whole migration is one
  reviewable diff and one command to undo.
- **It never reports success it has not verified.** The report leads with how many call
  sites will still raise at runtime. A flagged call site is still a call site.
- **It does not read your secrets.** It needs to know which files reference credential
  variables, never their values, and it will not quote a line that looks like a key from
  any file.
- **It flags rather than guesses.** Anything with no equivalent in the target version is
  marked in place with a comment explaining the options. It is never silently deleted,
  rewritten, or approximated.
- **The report is safe to share.** Counts and file locations, no source and no secrets.

The tool runs inside an AI coding assistant, which means your source is read by whichever
model that assistant uses. If that is not acceptable for your codebase, every change it
makes is documented in the migration rules under `migration/`, and can be applied by hand.

## Supported migrations

| Hop | Caller | Rule file | Reference examples |
|---|---|---|---|
| v0 -> v1 (`evermemos` -> `everos-cloud` 0.x) | Python SDK | `migration/python/v0-to-v1.md` | `examples/python/v0.py`, `v1.py` |
| v1 -> v2 (API v1 -> v2) | Any HTTP caller | `migration/http/v1-to-v2.md` | `examples/typescript/`, `examples/go/` |
| v1 -> v2 (`everos-cloud` 0.4.x -> 1.x) | Python SDK | `migration/python/v1-to-v2.md` | `examples/python/v1.py`, `v2.py` |

### Before you start (Python)

`everos-cloud` 1.x requires **Python 3.12 or newer**; 0.4.x required 3.9. The tool checks
this first and refuses to migrate a project targeting anything older, because rewriting the
code and then failing to install the package leaves you running on neither version.

### A note on version names

Three version numbers move independently, which is a common source of confusion:

| | Old | New |
|---|---|---|
| pip package | `everos-cloud` 0.4.x | `everos-cloud` 1.x |
| Memory API | v1 (`/api/v1/memories/*`) | v2 (`/api/v2/memory/*`) |
| Rule files here | `v1` | `v2` |

Rule files are named after the **API** version. When describing the upgrade to users,
say **"everos-cloud 1.x (the v2 Memory API)"** rather than a bare "v2".

## Repository Structure

```
everos-tools/
├── .claude-plugin/
│   └── marketplace.json
├── plugins/
│   └── everos-sdk-upgrade/
│       ├── .claude-plugin/
│       │   └── plugin.json
│       └── skills/
│           └── everos-sdk-upgrade/
│               ├── SKILL.md
│               ├── migration/
│               │   ├── http/
│               │   │   └── v1-to-v2.md      # transport rules — source of truth
│               │   └── python/
│               │       ├── v0-to-v1.md
│               │       └── v1-to-v2.md
│               └── examples/
│                   ├── python/       # v0.py, v1.py, v2.py
│                   ├── typescript/   # v1.ts, v2.ts
│                   └── go/           # v1.go, v2.go
├── .github/
│   └── workflows/
│       └── validate-plugins.yml
├── LICENSE
└── README.md
```

`SKILL.md`, `.claude-plugin/marketplace.json` and `.claude-plugin/plugin.json` are
fixed names required by the Agent Skills standard and the Claude Code plugin spec —
they are not free to rename.

## Adding Migration Rules

When a new API or SDK version ships:

1. Add `skills/everos-sdk-upgrade/migration/http/vN-to-vN+1.md` — the transport-level
   rules. This is the source of truth and covers every caller in every language.
2. Add `skills/everos-sdk-upgrade/migration/{lang}/vN-to-vN+1.md` for each SDK, mapping
   its method signatures onto the transport rules.
3. Add `skills/everos-sdk-upgrade/examples/{lang}/vN+1.{ext}` for major versions.
4. Update the `version` field in `plugin.json`.
5. Update the version-detection table in `SKILL.md` if the new SDK changed how a
   version can be recognised from call sites.
6. Push to this repository.

Users run `/plugin marketplace update` to get the latest rules.

### Rule-writing conventions

- Every rule gets a stable id (`API-0NN` for transport, `SDK-0NN` for Python) so the
  other files and the generated report can cite it.
- Mark each rule's **Change Type**: `BREAKING`, `BEHAVIOURAL`, `NEW`, `OPERATIONAL`
  or `NONE - Informational`.
- Give **Before/After** code, **Search Patterns**, and **Steps**.
- A capability removed with no replacement gets an explicit "FLAG, do not rewrite"
  instruction and suggested comment wording.
- Note where a claim was verified (published wheel, OpenAPI contract, or live API) so
  the next person can re-check it.

## License

Apache-2.0
