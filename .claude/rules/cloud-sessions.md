# Cloud session rules

These rules hold for every MeuRPG cloud session, on every item. They live on the fork's `main` only.

## Models and effort

- **The main agent** (the coordinator that plans, dispatches and reviews) runs on Claude Opus 5.5 at effort `medium`. The orchestrator sets it from outside; a session never changes its own model or effort.
- **A subagent that writes or edits code** (an implementer, a fixer) runs on Claude Sonnet 5.5 at effort `medium`: pass `model: "sonnet"` and `effort: "medium"` on the Agent call.
- **A subagent that reviews code or investigates a failure** runs on Claude Sonnet 5.5 at effort `high`: pass `model: "sonnet"` and `effort: "high"`.
- Every Agent call names both `model` and `effort`. A subagent with no `model` inherits the main agent's model, which breaks the rule.
- Never spawn a subagent on Fable or Opus, and don't use a fork for work a Sonnet subagent can do: a fork runs on the main agent's model.
- **A subagent that only looks things up or drives a browser** may run on Claude Haiku 5.5. Haiku never writes or edits code.
- Never pass `low`, `xhigh` or `max` to a subagent.
- Spend with care: one implementer and one reviewer per task, no second reviewer on the same diff, no agent for a lookup you can do with one command, and no re-run of a suite that already passed on the same head.
- When a task proves too hard for Sonnet (a second failed fix round on the same finding, for example), say so in a `question:` and name the task. Do not raise a subagent's model or effort by yourself.
- If your Agent tool has no `effort` parameter, or you find the main agent on another model, say so in your next `progress:` line.
- A hook refuses an Agent call that breaks these rules (`.claude/hooks/agent-effort.py`) and says why: repeat the call with the values it names.

## Commits

- Commit messages and PR texts carry no attribution lines: no `Co-Authored-By`, no session link. The settings turn them off; don't add them by hand.

## Nothing stays only in the container

- A container can be reset between turns, and a reset loses every commit the fork does not have.
- Push after every commit: to the work branch before its first `ready:`, and to `wip/<branch>` once the branch has a preflight or when it is stacked on another branch's `ready:` (`git push origin HEAD:refs/heads/wip/<branch>`).
- Work in a clone under the home directory, never under `/tmp`.
- After a reset, recover from `wip/<branch>` first, and post `blocked:` only when the fork lacks the work.
