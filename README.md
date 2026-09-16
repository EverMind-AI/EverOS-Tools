# EverOS Tools

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

## Install and run

The tool needs a git repository: it refuses to edit a tree it cannot restore. Start with
`--scan` in every case; it reads your code and prints a report without changing anything.

### Claude Code

Inside a Claude Code session:

```bash
/plugin marketplace add EverMind-AI/everos-tools      # one-time
/plugin install everos-sdk-upgrade@everos-tools

/everos-sdk-upgrade --scan     # report only, nothing edited
/everos-sdk-upgrade            # migrate; stops to ask if anything has no v2 equivalent
/everos-sdk-upgrade --yes      # migrate and proceed with those call sites flagged (CI, or a second run)

/plugin marketplace update     # pick up new rules later
```

The same two install steps from a terminal, if you prefer:

```bash
claude plugin marketplace add EverMind-AI/everos-tools
claude plugin install everos-sdk-upgrade@everos-tools
```

### Codex, Cursor, and other tools that follow the Agent Skills standard

This skill follows the [Agent Skills](https://agentskills.io) open standard. From your
project directory:

```bash
npx skills add https://github.com/EverMind-AI/everos-tools
```

This places the skill at `.agents/skills/everos-sdk-upgrade/` and writes `skills-lock.json`;
Codex and Cursor read that directory directly, and a symlink is added for any other detected
tool. Commit both, or add them to `.gitignore`, as you prefer.

Then ask your assistant, in its own chat:

```text
Run the everos-sdk-upgrade skill in --scan mode on this repository.
```

and later, to migrate, the same sentence without `--scan` (add "proceed with every blocker
flagged" for the `--yes` behaviour).

### What has been verified where

| Tool | Install | `--scan` | Full migration |
|---|---|---|---|
| Claude Code | marketplace and `npx skills add` | verified | verified, including a live run of the migrated code against production |
| Codex CLI | `npx skills add` | verified (`codex exec`, read-only sandbox) | not run |
| Cursor | `npx skills add` | verified (`cursor-agent -p`) | not run |
| Other Agent Skills tools | `npx skills add` | same standard, not run against a fixture | not run |

The tool names in `SKILL.md` (`Grep`, `Glob`, `Read`) are Claude Code's; Codex and Cursor
mapped them to their own tools without help.

## What the report looks like

The first lines of a real `--scan` on a small v1 script:

```text
VERDICT
  Can this tree migrate?      yes, with 1 call site left on v1
  The tool does               2 mechanical rewrites across 2 files
  You decide                  0 blocker categories, 1 open decision

STATUS
  1 call site will still raise at runtime after this migration.
      -> flush() has no session_id in scope at the call site; it stays flagged
```

followed by PRE-FLIGHT, the seven BLOCKERS rows (with who resolves each), NEEDS A DECISION,
MECHANICAL and BEFORE YOU SHIP, every item with a file and line. The report contains no
source and no secrets, so it can be sent to EverOS as it is.

## What it does to your repository

- **`--scan` writes nothing.** It reads your code and prints a report. Use it first.
- **It checks before it edits.** A pre-flight gate runs before the first change: your
  Python version against the target's floor, whether your EverOS calls are on an async
  path, how many capabilities have no v2 equivalent, and whether your working tree already
  has uncommitted work in the files it is about to touch. Any of those can stop the run.
- **It takes a snapshot first** (`git stash create`, which leaves your working tree exactly
  as it is), then works on its own branch, so the whole migration is one reviewable diff.
  The report ends with the exact `git restore` command that undoes it, file by file.
- **It stops to ask before flagging anything it cannot migrate.** Answer the question, or
  pass `--yes` to proceed with those call sites flagged in place. Without `--yes`, a run
  where nobody can answer produces the report and edits nothing.
- **It never reports success it has not verified.** The report leads with how many call
  sites will still raise at runtime. A flagged call site is still a call site.
- **It never prints your secrets.** It needs to know which files reference credential
  variables, never their values. It does not open `.env` files, and it will not quote a
  line that looks like a key from any file it does read.
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
