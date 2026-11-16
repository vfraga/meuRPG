#!/usr/bin/env python3
"""PreToolUse hook of the cloud sessions for the Agent tool: the effort floor of .claude/rules/cloud-sessions.md.

A subagent names its model, Sonnet or Haiku: one that leaves `model` out inherits the main agent's model. A
subagent on Sonnet names its effort, medium (it writes code) or high (it reviews or investigates), and no
subagent runs at low, xhigh or max. The built-in search and planning agents and a fork pass untouched.
Exit code 2 refuses the call and shows the reason to the main agent, which repeats the call with the right
values. Anything this hook cannot read passes: it never stops work it does not understand.
"""
import json
import sys

PASS_TYPES = {"explore", "plan", "fork", "claude-code-guide", "statusline-setup", "web-fetch"}


def verdict(tool_input):
    kind = str(tool_input.get("subagent_type") or "").lower()
    if kind in PASS_TYPES:
        return None
    model = str(tool_input.get("model") or "").lower()
    effort = str(tool_input.get("effort") or "").lower()
    if effort in ("max", "xhigh", "low"):
        return (f'effort "{effort}" is never used for a subagent. Repeat the call with effort "medium" '
                '(it writes code) or "high" (it reviews or investigates).')
    if not model:
        return ('this Agent call names no model, so it would run on the main agent\'s model. '
                'Repeat the call with model "sonnet" and effort "medium" or "high" (or model "haiku" for a lookup).')
    if "sonnet" not in model and "haiku" not in model:
        return (f'a subagent runs on Sonnet or Haiku, not on "{model}". '
                'Repeat the call with model "sonnet" and effort "medium" or "high".')
    if "sonnet" in model and effort not in ("medium", "high"):
        said = f'effort "{effort}"' if effort else "no effort"
        return (f'this Agent call runs on Sonnet with {said}. A Sonnet subagent names its effort: "medium" when it '
                'writes code, "high" when it reviews or investigates. Repeat the call with one of them.')
    return None


def main():
    try:
        data = json.load(sys.stdin)
        tool_input = data.get("tool_input")
        if not isinstance(tool_input, dict):
            return 0
        reason = verdict(tool_input)
    except Exception:  # an input this hook cannot read never stops the session
        return 0
    if reason:
        print("agent-effort: " + reason + " (.claude/rules/cloud-sessions.md)", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
