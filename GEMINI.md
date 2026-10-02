# Gemini Guidelines

All Gemini agents and assistants operating in this repository must adhere to the instructions defined in [AGENTS.md](AGENTS.md).

## Summary of Rules
- **Worktree Isolation**: All agents must work in their own dedicated Git worktree under `.worktrees/`. Never make modifications directly in the main working tree or on shared branches.
- **Repository Guidelines**: Follow the conventions, presubmit checks, and conventional commits policy documented in [AGENTS.md](AGENTS.md).
