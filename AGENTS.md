# AGENTS.md

## Language
- Thinking: English for internal reasoning.
- Response: ALWAYS respond in Vietnamese.
- Keep code, identifiers, commit messages, file paths, CLI commands, and technical proper nouns in English.
- Use Vietnamese for narrative headings, bullets, summaries, and reports.

## Role
Orchestrator. Route work by complexity:
- Simple tasks touching 1-2 files: do directly.
- Medium tasks touching 3-10 files: short plan, implement, test.
- Complex tasks touching 10+ files: use relevant Codex skills and subagents.

## Before Planning Or Implementing
- Read `README.md` first when it exists.
- Check `docs/` when relevant, especially project overview, code standards, architecture, deployment, design, and roadmap docs.
- Claude-specific rules remain in `.claude/rules/`.
- Codex repo skills live in `.agents/skills/`; Codex project config, agents, and hooks live in `.codex/`.
- How Codex loads skills/agents/hooks (and how to wire/verify them): see `docs/codex-setup.md`.

## Git
- Use conventional commits: `feat`, `fix`, `refactor`, `test`, `style`, `perf`, `ci`, `build`.
- Do not use `chore` or `docs` for files under `.claude/`, `.agents/`, or `.codex/`.
- Keep messages clean and omit AI attribution.

## Skill Scripts
- Claude skill scripts use `.claude/skills/`.
- Codex skill scripts use `.agents/skills/`.
- If a skill has a Python venv, use the venv under that skill tree when present.
- Script fails: investigate and fix directly instead of stopping at the first error.

## Code Style
- Files over 200 LOC should be considered for modularization, except markdown, shell, config, env, and generated files.
- Prefer kebab-case and descriptive filenames for grep-friendly agent work.
- Check for existing modules before creating new ones.

## Surgical Changes
- Touch only what the user request requires.
- Do not refactor unrelated code.
- Match existing style even when another style looks cleaner.
- Remove dead code introduced by your own change. Leave unrelated pre-existing dead code alone and mention it if relevant.

## API Response Contract
If the project has internal API responses shaped as `{success, data?, message?}`, follow the API response contract in `.claude/rules/api-response-contract.md` or the equivalent Codex skill/rule copy when present.
- Do not manually read `response.data.data`.
- Do not use defensive `?? response.data` patterns.
- Do not call raw axios in feature files when wrappers exist.
- Use unwrap wrappers such as `apiGet` / `apiPost`, or create a wrapper if missing.

## Privacy Block Hook
If a tool call is blocked with `@@PRIVACY_PROMPT@@`, see `.claude/rules/privacy-block-hook.md` for Claude or the corresponding Codex hook output in `.codex/hooks/privacy-block.cjs`.

## Reports
- Keep reports concise.
- Prefer clear technical signal over perfect grammar.
- List unresolved questions at the end when any remain.
