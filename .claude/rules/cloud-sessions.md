# Cloud session rules

These rules hold for every MeuRPG cloud session, on every item. They live on the fork's `main` only.

## Models, effort and subagents

- **The main agent** (the coordinator that plans, dispatches and reviews) runs on Claude Opus 5.5 at effort `medium`. The orchestrator sets it from outside; a session never changes its own model or effort.
- **Every subagent is one of three named agents**, defined in `.claude/agents/`:
  - `implementer` writes or fixes code, tests and docs: Claude Sonnet 5.5 at `medium`, with a limit of 60 turns;
  - `reviewer` reviews a diff or investigates a failure and never edits code: Sonnet 5.5 at `high`;
  - `explorer` builds reading lists and does look-ups: Claude Haiku 5.5. Haiku never writes or edits code.

  Pass `subagent_type` and leave `model` and `effort` out: the definition sets them. `general-purpose` is not used for this work.
- Never spawn a subagent on Fable or Opus, never pass `low`, `xhigh` or `max`, and don't use a fork for work a named agent can do: a fork runs on the main agent's model.
- **Before an implementer starts a task, an explorer writes the task's reading list.** The task's reviewer gets the same list. The task's message names the reading list and the state file (implementer) or the findings file (reviewer). They live in the session's notes folder under the home directory (`~/<area>-notes/<branch>/`), never under `/tmp`.
- **A run that comes back partial has hit its turn limit.** Start a fresh implementer from the state file. Never resume the old one: a resumed agent carries its whole history.
- Start the `description` of every Agent call with the task's label and a colon (`D3: ...`), the same label your `progress:` lines use.
- Spend with care: one implementer and one reviewer per task, no second reviewer on the same diff, no agent for a lookup you can do with one command, and no re-run of a suite that already passed on the same head.
- When a task proves too hard for the implementer (a second failed fix round on the same finding, for example), say so in a `question:` and name the task. Do not change an agent's model or effort by yourself.
- If your Agent tool does not offer `implementer`, `reviewer` and `explorer`, or you find the main agent on another model, say so in your next `progress:` line.
- A hook refuses an Agent call that breaks these rules (`.claude/hooks/agent-effort.py`) and says why: any agent type but the three named ones and the built-in search and planning agents, and an implementer whose prompt names no existing reading list. Repeat the call with the values it names.

## Commits

- Commit messages and PR texts carry no attribution lines: no `Co-Authored-By`, no session link. The settings turn them off; don't add them by hand.

## Nothing stays only in the container

- A container can be reset between turns, and a reset loses every commit the fork does not have.
- Push after every commit: to the work branch before its first `ready:`, and to `wip/<branch>` once the branch has a preflight or when it is stacked on another branch's `ready:` (`git push origin HEAD:refs/heads/wip/<branch>`).
- Work in a clone under the home directory, never under `/tmp`.
- After a reset, recover from `wip/<branch>` first, and post `blocked:` only when the fork lacks the work.
