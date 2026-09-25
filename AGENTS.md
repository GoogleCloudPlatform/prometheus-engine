# Agent Guidelines

This document outlines essential instructions and operating procedures for AI agents (Antigravity, Gemini, and others) working on the `prometheus-engine` repository.

---

## 1. Mandatory Git Worktree Isolation

**All agents MUST perform their work in their own dedicated Git worktree.**

- **Do NOT modify files directly in the main working tree** or on shared branches.
- **Location**: Worktrees must be created under the `.worktrees/` directory (e.g., `.worktrees/<branch-or-task-name>`), which is ignored by Git.
- **Rationale**: Isolates changes, prevents clobbering active checkouts or uncommitted edits from users or other agents, keeps builds independent, and ensures clean commit histories.

### Worktree Workflow

1. **Create and switch to a new worktree**:
   Always branch from the latest `origin/main` (or the intended target branch):
   ```bash
   git worktree add -b <branch-name> .worktrees/<worktree-name> origin/main
   ```

2. **Execute all actions within the worktree**:
   Set your working directory to `.worktrees/<worktree-name>`. All file edits, commands, tests, and builds must be performed inside this path.

3. **Verify Git status**:
   Ensure `.worktrees/` remains untracked and is ignored. Never commit worktree administrative metadata.

4. **Clean up when finished**:
   After work is merged or no longer needed:
   ```bash
   git worktree remove -f .worktrees/<worktree-name> && git branch -D <branch-name>
   ```

---

## 2. Repository Conventions & Standards

### Commit Messages & Conformity
All commits must comply with the Conventional Commits policy enforced by `make conform` (configured in `.conform.yaml`):
- **Header format**: `<type>(<scope>): <subject>`
  - Maximum header length: 89 characters.
  - Lowercase imperative sentence for `<subject>`, no trailing period.
- **Allowed Types**: `build`, `chore`, `ci`, `docs`, `perf`, `refactor`, `revert`, `style`, `test`.
- **Allowed Scopes**: `agents`, `configs`, `deps`, `e2e`, `export`, `main`, `operator`, `prometheus`, `frontend`, `datasource-syncer`, `config-reloader`, `rule-evaluator`, `ops`.
- Example: `docs(agents): add worktree guidelines to AGENTS.md`

### Presubmit Checks & Formatting
Before committing or submitting a PR:
- **Format codebase**:
  ```bash
  ./hack/presubmit.sh format
  ```
  Formats Go files (`go fmt`), documentation (`mdox fmt`), and shell scripts (`shfmt`).
- **Run Linters**:
  ```bash
  make lint
  ```
- **Verify Commit Conformity**:
  ```bash
  make conform
  ```

### Testing
- **Unit Tests**: Run tests for relevant packages (e.g., `go test ./pkg/...`).
- **E2E Tests**: See documentation in `.gemini/skills/e2e-tests.md`.

---

## 3. Skills Reference

Additional workflows and tasks are documented in `.gemini/skills/`:
- `build-deploy.md`: Building container images and deployment manifests.
- `code-generation.md`: Regenerating CRDs and Go deepcopy methods.
- `e2e-tests.md`: Running local and cloud end-to-end tests.
- `lint-and-format.md`: Code style, linting, and conformity guidelines.
- `unit-tests.md`: Running unit tests across components.
