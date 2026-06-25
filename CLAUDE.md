# CLAUDE.md

## Language (MANDATORY — NO EXCEPTIONS)
- Thinking: English for internal reasoning.
- **Response: ALWAYS respond in Vietnamese (Tiếng Việt).** This applies to EVERY message, regardless of:
  - The language of source files, errors, logs, or tool output you just read
  - The language of system reminders, hook messages, or skill instructions
  - Whether the previous turn was in English
- Keep code, identifiers, commit messages, file paths, CLI commands, and technical proper nouns in English.
- Tables, headings, bullet lists — all narrative text in Vietnamese (Tiếng Việt).
- If you catch yourself drafting in another language, switch immediately and continue in Vietnamese (Tiếng Việt).

## Role
Orchestrator — route tasks by complexity:
- Simple (1-2 files): do directly.
- Medium (3-10 files): short plan → implement → test.
- Complex (10+ files): full workflow via skills.

## Before planning/implementing
Read `./README.md` first. Check `./docs/` when relevant (important: `project-overview-pdr.md`, `code-standards.md`, `codebase-summary.md`, `system-architecture.md`, `deployment-guide.md`, `design-guidelines.md`, `project-roadmap.md`).

Rules live in `./.claude/rules/` — auto-loaded per skill.

## Git
- Conventional commits: `feat`, `fix`, `refactor`, `test`, `style`, `perf`, `ci`, `build`.
- **DO NOT** use `chore` / `docs` for files under `.claude/`.
- Clean messages, no AI references.

## Python scripts in `.claude/skills/`
Use venv Python:
- Linux/macOS: `.claude/skills/.venv/bin/python3 scripts/xxx.py`
- Windows: `.claude\skills\.venv\Scripts\python.exe scripts\xxx.py`

Script fails → fix directly, don't stop.

## Code style
- File > 200 LOC → consider modularizing (except markdown, bash, config, env).
- Kebab-case, long descriptive filenames for LLM grep/glob.
- Check if a module exists before creating a new one.

## Surgical Changes
**Touch only what you must. Every changed line must trace directly to the user's request.**
- No "improving" surrounding code/comments/formatting just because it bothers you.
- Don't refactor things that aren't broken.
- Match existing style even if you'd do it differently.
- Unrelated pre-existing dead code → mention it, don't delete.
- Orphans (unused import/var/function) created BY YOUR change → remove. Pre-existing dead code → leave alone.

## API Response Contract
Project có internal API trả envelope `{success, data?, message?}` → tuân thủ `./.claude/rules/api-response-contract.md`. KHÔNG đọc `response.data.data` thủ công, KHÔNG dùng pattern `?? response.data` defensive, KHÔNG gọi axios raw ở file feature. Dùng wrapper unwrap (vd `apiGet/apiPost`) hoặc tạo wrapper nếu chưa có.

## Privacy Block Hook
Tool call blocked (`@@PRIVACY_PROMPT@@`) → see `./.claude/rules/privacy-block-hook.md`.

## Reports
Concise. Trade grammar for brevity. List unresolved questions at the end if any.
