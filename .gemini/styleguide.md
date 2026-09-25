# Agent Guidelines & Worktrees
- All AI agents and Gemini assistants operating on this repository must follow the instructions in [AGENTS.md](../AGENTS.md).
- In particular, agents must work in their own dedicated Git worktree under `.worktrees/`. Do not edit files directly in the main working tree.

# Review Formatting
- When proposing a code change, always use the GitHub "suggestion" Markdown block.
- Ensure the suggestion block captures the correct lines to allow for direct committing in the UI.
- Example:

  ```suggestion
  updated_code_here()
  ```
