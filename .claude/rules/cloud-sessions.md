# Cloud session rules

These rules hold for every MeuRPG cloud session, on every item. They live on the fork's `main` only.

## Models and effort

- **The main agent** (the coordinator that plans, dispatches and reviews) runs on Claude Fable 5.2 at effort `medium`. The orchestrator sets it from outside; a session never changes its own model or effort.
- **Every subagent that writes, reviews or investigates code** runs on Claude Sonnet 5.5 at effort `high`: pass `model: "sonnet"` and `effort: "high"` on every Agent call. A subagent with no `model` inherits the main agent's model, and one with no `effort` runs at Sonnet's default, `medium`: both break the rule.
- Never spawn a subagent on Fable or Opus, and don't use a fork for work a Sonnet subagent can do: a fork runs on the main agent's model.
- **A subagent that only looks things up or drives a browser** may run on Claude Haiku 5.5, also at effort `high`. Haiku never writes or edits code.
- Never pass an effort below `high` to a subagent, and never use `max` anywhere.
- When a task proves too hard for Sonnet at `high` (a second failed fix round on the same finding, for example), say so in a `question:` and name the task. Do not raise a subagent to `xhigh` by yourself.
- If your Agent tool has no `effort` parameter, or you find the main agent on another model, say so in your next `progress:` line.
- A hook refuses an Agent call that breaks these rules (`.claude/hooks/agent-effort.py`) and says why: repeat the call with the values it names.

## Commits

- Commit messages and PR texts carry no attribution lines: no `Co-Authored-By`, no session link. The settings turn them off; don't add them by hand.

## Nothing stays only in the container

- A container can be reset between turns, and a reset loses every commit the fork does not have.
- Push after every commit: to the work branch before its first `ready:`, and to `wip/<branch>` once the branch has a preflight or when it is stacked on another branch's `ready:` (`git push origin HEAD:refs/heads/wip/<branch>`).
- Work in a clone under the home directory, never under `/tmp`.
- After a reset, recover from `wip/<branch>` first, and post `blocked:` only when the fork lacks the work.
