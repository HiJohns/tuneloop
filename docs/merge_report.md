# Core Files Consolidation Report

## Projects Analyzed

| # | Project | AGENTS.md | project.md | verdict |
|---|---------|-----------|------------|---------|
| 1 | tuneloop | 363 lines | 98 lines | Both full |
| 2 | beaconiam | 222 lines | 74 lines | Both full |
| 3 | report-v2 | 180 lines | 498 lines | Both full |
| 4 | utilities | 175 lines | 57 lines | Both full |
| 5 | lamb | 94 lines | empty | project.md empty |
| 6 | fspgroup | 288 lines | empty | project.md empty |

---

## Question 1: Generic content to move from AGENTS.md → instructions.md?

Each AGENTS.md has 3 layers of content:

### ✅ Truly Generic — Move to instructions.md

These appear nearly identically across all 6 projects:

| Category | Present In | Content |
|----------|-----------|---------|
| Naming conventions | All 6 | camelCase/PascalCase/snake_case rules |
| Import organization | 4 Go projects | 3-group import ordering |
| Error handling | All 6 | Never ignore errors, wrap with context |
| Testing patterns | 4 Go projects | Table-driven tests, testify, mocks |
| Git workflow | tuneloop | Feature branches, clean commits |
| Security basics | beaconiam | Never log secrets, hash passwords |

### ❌ Project-Specific — Stay in AGENTS.md

| Category | Projects | Why it stays |
|----------|----------|-------------|
| Build commands (Make targets, npm scripts) | All 6 | Unique per project |
| Directory structure | All 6 | Unique per project |
| Framework patterns (Controller struct, Redux middleware) | tuneloop, fspgroup, utilities | Framework-specific |
| Frontend navigation / menus | tuneloop | TuneLoop-specific UI |
| Business domain models | tuneloop, beaconiam | Domain-specific data |
| Audit lessons learned | tuneloop | Project-specific bug history |
| Dependencies & versions | All 6 | Project-specific |
| Project description & stack | All 6 | Identity |

### Recommendation

Add one section to `instructions.md` §1:

```markdown
### 1.1 Universal Code Style (Enforced via instructions.md)

The following rules apply to ALL projects. Do NOT duplicate in AGENTS.md.

- Naming: Exported=PascalCase, unexported=camelCase, packages=lowercase
- Imports: 3 groups (stdlib → external → internal), blank line between
- Errors: Never use `_`, wrap with `fmt.Errorf("context: %w", err)`
- Testing: Table-driven, testify (require/assert), mock external deps
- Security: Never log secrets, hash passwords (bcrypt), validate all input
```

Then trim AGENTS.md to:
```markdown
> Universal coding rules: see `prompts/instructions.md` §1.1
```

---

## Question 2: Can project.md merge into AGENTS.md?

### Commands That Reference project.md

| Command | Lines | What it does |
|---------|-------|-------------|
| `start.md` | 5, 19-35 | Reads `prompts/project.md` at init; checks `## Core Documents` |
| `analyze.md` | 42-70 | Reads `project.md`'s Core Documents list for conflict detection |

No other commands reference project.md directly.

### What's in project.md vs AGENTS.md (overlap analysis)

| Content | Already in AGENTS.md? | Example project |
|---------|----------------------|----------------|
| Core Documents list | tuneloop: YES | Both list `docs/api_design.md` etc. |
| Business logic models | tuneloop: NO | Properties flow, Instrument Levels |
| Build/deploy commands | tuneloop: PARTIAL | Build commands in AGENTS, env config in project |
| Architecture decisions | beaconiam: YES | Namespace→Client mapping in both |
| TODO/DONE task lists | report-v2: NO | Belongs in GitHub Issues, not docs |
| UI design references | report-v2: YES | Both reference `docs/ui_design.md` |
| Custom conventions | utilities: PARTIAL | Path conventions, custom commands |

### Verdict: YES, merge project.md → AGENTS.md

**Benefits:**
- One file to maintain per project instead of two
- `analyze.md` and `start.md` reference one file for Core Documents
- No more "which file has what?" confusion

**Merge mapping per project:**

| Project | Merge action |
|---------|-------------|
| tuneloop | project.md §1-4 (business logic, levels, documents) → AGENTS.md; project.md → empty symlink |
| beaconiam | project.md §Core Docs + architecture → AGENTS.md; keep project description |
| report-v2 | project.md §Core Docs + conventions → AGENTS.md; move TODOs to Issues |
| utilities | project.md fully → AGENTS.md |
| lamb | Already empty project.md — no action |
| fspgroup | Already empty project.md — no action |

### Commands to Update (if merging)

Only 2 files need changes:

**`start.md`** — 2 changes:

1. Line 5: `prompts/project.md` → `AGENTS.md`
   ```
   - 请立即读取并内化项目根目录下的 `prompts/instructions.md`... 以及 `AGENTS.md`
   ```

2. Lines 19-35: `prompts/project.md` → `AGENTS.md`
   ```
   - 检查 `AGENTS.md` 中是否定义了 `## Core Documents` 章节
   ```

**`analyze.md`** — 1 change:

1. Lines 42-43:
   ```
   #    - 读取 project.md 中的 "## Core Documents" 章节
   ```
   → Change `project.md` to `AGENTS.md`

---

## Recommended Action Plan

### Phase 1: Update `instructions.md` (done in prior session ✗✗✗ ADD this)
Add §1.1 Universal Code Style with naming, imports, errors, testing, security rules that apply to all projects.

### Phase 2: Update `start.md` and `analyze.md`
Change all `project.md` references to `AGENTS.md`.

### Phase 3: Merge per-project
For each project, move project.md content into AGENTS.md, then replace project.md with a symlink or note pointing to AGENTS.md.

### Phase 4: Trim AGENTS.md
Remove generic content now covered by `instructions.md` §1.1. Add reference line:
```
> Universal coding rules: see `prompts/instructions.md` §1.1
```

### Phase 5: Verify
Run `/start` in each project to confirm initialization works with new structure.

---
*Model: deepseek/deepseek-v4-pro*
