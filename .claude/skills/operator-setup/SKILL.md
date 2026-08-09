---
name: operator-setup
description: Create or update a personal operator profile under ~/.agents/projects/ — who the operator is, skill calibration, and collaboration preferences for this project. Only invoke when the user explicitly runs /operator-setup. Do not auto-trigger based on context.
---

# Operator Setup

Interview the operator and write a **personal calibration profile** so models adjust explanation depth, autonomy, and collaboration style for this human on this project.

**In scope:** role, autonomy, verbosity, working mode, skill calibration (what to skip / surface).  
**Out of scope:** project policy, team process, quality gates, shared repo rules. Those belong in committed docs (`AGENTS.md`, CONTRIBUTING, CI). If the operator volunteers team rules, redirect: ask what that means for how the model should work *with them*, and point shared norms at repo instructions.

## Canonical storage

```text
~/.agents/
  AGENTS.md                    # general user-global instructions (wide responsibility)
  projects/
    index.yaml                 # canonical project root → profile id
    <id>/
      OPERATOR.md              # calibration payload
```

| Path | Role |
|------|------|
| `~/.agents/AGENTS.md` | Multi-purpose global notes. This skill only ensures a **short** every-session operator blurb (below). |
| `~/.agents/projects/index.yaml` | Map of canonical roots → `id` (+ optional `name`). |
| `~/.agents/projects/<id>/OPERATOR.md` | Profile content. |

Expand `~` to the user's home directory on the current OS. Create directories as needed.

**Do not** use workspace `.agents/OPERATOR.md` as the system of record. **Do not** inject operator-resolve instructions into the repository `AGENTS.md`. **Do not** add workspace profile paths to `.gitignore` for this design. After a successful home profile write, **clean up legacy workspace artifacts** (see [Legacy cleanup](#legacy-cleanup)).

### Short note for `~/.agents/AGENTS.md`

If missing, append this block (use HTML comment markers so re-runs are idempotent). Do not replace unrelated content in the file. Do not grow this into a catalog or resolve essay:

```markdown
<!-- operator-setup:load -->
At the start of every session, load the operator profile for this workspace
via `~/.agents/projects/` (see `projects/index.yaml` and that project's
`OPERATOR.md`) for role, skill levels, and collaboration style. Do this
quietly — do not narrate bootstrap. If none matches, end your first response with:

> No operator profile found. Run `/operator-setup` to create one.
<!-- /operator-setup:load -->
```

### Index (`projects/index.yaml`)

Path keys are **canonical project roots**. Folder location drives the index—not git remotes.

```yaml
# Canonical project root → profile id
# Git: store main worktree path only. Linked worktrees are not separate keys.
paths:
  "x:/src/example/repo":
    id: repo
    name: repo
```

| Field | Required | Meaning |
|-------|----------|---------|
| map key | yes | Normalized absolute canonical root |
| `id` | yes | Directory name under `projects/` |
| `name` | no | Display label |

### Profile path

`~/.agents/projects/<id>/OPERATOR.md` by convention only (no absolute profile path required in the index).

**`id`:** sanitized basename of the canonical root (lowercase, `[a-z0-9._-]+`, max ~40 chars). On collision with a different path, use `<basename>-<6 hex chars of path hash>`.

### Path normalization

Use one scheme for both setup writes and resolve:

- Absolute path
- No trailing slash
- On Windows, lowercase the drive letter
- Prefer forward slashes in `index.yaml` keys for stability across tools
- Resolve symlinks when practical so the same folder is not double-registered

## Resolve (session load — quiet)

**One intent, not a checklist to narrate:**

> Load the operator profile for this workspace via `~/.agents/projects/` (index + `OPERATOR.md`). Use it to calibrate; do not treat it as project policy. Do this quietly — do not narrate bootstrap, index lookups, or path canonicalization unless the user asks or setup requires confirmation.

**Canonicalize cwd → lookup key:**

1. If git is available and cwd is inside a work tree: resolve the **main worktree** absolute path (not a linked worktree path). Use that as the key.
2. Else: normalized absolute path of the project folder (the directory this profile is for — typically the workspace root used for setup).

**Lookup:** read `~/.agents/projects/index.yaml`, exact-match the canonical path under `paths`, load `~/.agents/projects/<id>/OPERATOR.md`. No match → no profile (missing-profile nudge only).

**Worktrees:** creating a linked worktree must **not** require an index update. Setup run inside a linked worktree still **registers the main worktree path**. Second clones at a new path are a different key until the operator sets up (or later explicitly binds) that path.

Git is optional: non-git folders use the folder path only.

## Before starting (`/operator-setup`)

1. Canonicalize the current workspace (main worktree path if git).
2. Read `~/.agents/projects/index.yaml` if it exists; resolve any existing profile for this canonical path.
3. **If a profile exists:** do **not** default to full re-interview or “start fresh.” Summarize what is on file briefly, use the conversation to **derive which sections to change**, and apply a **minimal patch**. Full refresh only if the user clearly asks or the file is broken/empty.
4. **If no profile:** full create interview below.
5. If legacy workspace `.agents/OPERATOR.md` exists and the home profile does not yet: **migrate** useful content into the new location (or use it to seed the interview), then proceed. Always run [Legacy cleanup](#legacy-cleanup) after the home profile is written.

### Sticky vs fluid

| Section | Cadence |
|---------|---------|
| Role, autonomy, verbosity, working mode | Rare |
| Skill matrix | Living (new areas, too easy / too hard) |
| Full rewrite | Escape hatch only |

## Tech stack detection (for skill questions)

Read repo `AGENTS.md` / `CLAUDE.md` if present and scan the project root for signals (examples):

- `*.csproj` / `*.sln` → C# / .NET  
- `package.json` → JavaScript / TypeScript  
- `*.py` / `pyproject.toml` → Python  
- `*.go` → Go  
- `Dockerfile` / `docker-compose.yml` → Docker / containers  
- `*.tf` → Terraform  
- Plugin / skill layout (`plugin.json`, `SKILL.md`) → agent skills / plugins when relevant  

Use the **2–4** most relevant areas. Do not quiz every library.

## Create interview (one question at a time)

Ask one at a time. Prefer a structured question tool when available. Do not ask the next until the previous is answered.

**Q1 — Role**  
Primary role on this project (open question). Examples only: Solo owner, Lead developer, Solution Architect, Backend/Frontend/Full-stack, Test Analyst, DevOps. Infer decision authority carefully; do not invent stakeholders.

**Q2 — Autonomy**

- Fully autonomous — act on reasonable assumptions, minimal check-ins  
- Checkpoint on approach — agree on plan before implementing, then execute  
- Frequent check-ins — confirm at key decision points  
- Depends on risk — autonomous for low-risk; check in for architectural or destructive changes  

**Q3 — Verbosity**

- Minimal — code and diffs, very little prose  
- Brief rationale — one sentence on why, then the change  
- Explain trade-offs — surface alternatives and reasoning before committing  
- Teach me — explain what is happening, especially in unfamiliar areas  

**Q4 — Working mode**

- I direct, you implement — clear picture; model executes  
- Collaborative problem-solving — think through together before committing  
- Review-driven — they write or sketch; model reviews and improves  
- Exploration-first — understand options before deciding  

**Q5 — Skill calibration** (per detected area, 2–4 areas)

Do **not** use Expert / Proficient / Functional / Beginner as the primary signal. Offer a **menu of calibration stances** — instructions to the model, not grades of the human:

| Stance | Model behavior |
|--------|----------------|
| Fluent | Skip idioms and basics; treat as fluent |
| Day-to-day | Comfortable daily use; flag advanced or unusual cases |
| Explain non-obvious | Can follow along; explain non-obvious choices |
| Teach as you go | Do not assume stack fluency; explain as you work |
| Mixed / Other | Short freeform → put detail in Notes |

Write the **Notes** cell from the choice (concrete skip / surface guidance). Do not leave Notes as a copy of a vague level label.

After the per-area menus, optionally ask once (not per tech): anything to always skip or always explain?

**Policy redirect:** if they answer with project rules (tests required, never force-push, etc.), reframe as collaboration preference or point them at shared repo docs.

## Generate `OPERATOR.md`

Write `~/.agents/projects/<id>/OPERATOR.md`:

```markdown
# OPERATOR.md

Personal calibration for models working with this operator on this project.
Not project policy.

## Role

[Role and decision authority in 1–3 behavioral sentences.]

## Autonomy

[Short bullets: when to act vs check in — derived from their choice.]

## Communication Style

- **Verbosity:** [choice] — [one line on what to do]
- **Working mode:** [choice] — [one line on what to do]

## Skill Levels

| Area | Calibration | Notes |
|------|-------------|--------|
| … | Fluent / Day-to-day / … | Concrete skip / surface notes |
```

Prefer crisp bullets over soft paragraphs so models can apply the profile.

Then:

1. Upsert `~/.agents/projects/index.yaml` for this canonical path → `id` / `name`.  
2. Ensure the short load note in `~/.agents/AGENTS.md`.  
3. Self-wire **this harness only** (next section).  
4. Run [Legacy cleanup](#legacy-cleanup).

## Harness wiring (this product only)

After writing profile + index + short global note:

- Ensure **this** harness will load `~/.agents/AGENTS.md` at session start (if the product does not already).  
- Use **this product’s normal place** for user-global agent instructions. Do **not** ship or follow a table of paths for other tools (`~/.grok`, `~/.codex`, `~/.claude`, etc.).  
- Prefer a minimal pointer: *read `~/.agents/AGENTS.md` at session start* when not already injected. If this harness never reads that file, install the **same short operator load note** (marked block) in this harness’s global instructions instead—still not the index or resolve essay.  
- Marked, idempotent; do not overwrite unrelated content.  
- Re-read the file you touched and confirm the block is present. Report the path to the user.  
- **Only wire yourself.** Do not configure other coding agents.  
- **Do not** inject operator-resolve into the repository `AGENTS.md`.

If you cannot find this harness’s global instructions location, say so and stop; do not guess other products’ directories.

## Legacy cleanup

Previous versions of this skill used a **workspace-local** profile and injected a load blurb into the **repo**. Those must not remain after a successful home-profile setup — they compete with `~/.agents/` and confuse session bootstrap.

After the home profile, index entry, and harness wire are in place, scan and clean:

| Artifact | Action |
|----------|--------|
| Workspace `.agents/OPERATOR.md` | If content was already migrated (or superseded by the new profile), **delete** the file. If the home profile was created without migrating and the files differ, show a short diff summary and get confirmation before delete. |
| Empty workspace `.agents/` left with nothing else useful | Remove only if empty (or only contained `OPERATOR.md`); do not delete shared `.agents/skills` or other project assets. |
| Repo root `AGENTS.md` / `CLAUDE.md` / `Agents.md` | Remove the **old** operator-load block that tells models to read `.agents/OPERATOR.md` (and the “No operator profile found / `/operator-setup`” nudge tied to that path). Leave all other repo instructions untouched. |
| `.gitignore` entry for `.agents/OPERATOR.md` | Remove that line (and a now-empty “User-specific files” section only if it has no other entries). Optional but preferred so the ignore list matches reality. |

**Do not** remove the new short note from `~/.agents/AGENTS.md`.  
**Do not** strip unrelated repo policy from `AGENTS.md`.  
If the old blurb is mixed into a larger paragraph, delete only the operator-profile sentences.

Report what was removed. If nothing legacy was found, say so briefly.

On **update-only** runs (home profile already existed): still run this cleanup when legacy artifacts remain — do not leave dual sources of truth.

## Confirm

Tell the user:

- Canonical project path registered  
- Profile path (`~/.agents/projects/<id>/OPERATOR.md`)  
- That future sessions of **this** harness should load it quietly via `~/.agents/AGENTS.md`  
- Worktrees of this repo should resolve without further index updates (when git main worktree was registered)  
- What legacy workspace/repo artifacts were cleaned up (or that none remained)

## Mid-session maintenance (any session — not only this skill)

Any session may **propose** updates to the resolved `OPERATOR.md` when signal is clear:

| Do | Don't |
|----|--------|
| Propose a minimal patch | Silently rewrite the file |
| Prefer skill-row add or Notes/stance recalibrate | Full refresh because one area shifted |
| Confirm, then write | Treat every stack file sighting as a new row |
| Leave role/autonomy/style alone unless clearly requested | Re-run the full questionnaire |

**Signals worth acting on:** sustained work in a significant area missing from the matrix; explicit “stop explaining / I don’t follow”; clear request to change collaboration prefs.

**Do nothing** for autopilot, autogenerated, empty, or non-deliberate acknowledgements. If intent is unclear, skip the proposal.

**Index:** write only for a **new** canonical root (first-time setup for that folder), not for skill/pref text patches. Linked worktrees never need a new index row.

When several sections are ambiguous, suggest `/operator-setup` and let it derive targets from conversation.
