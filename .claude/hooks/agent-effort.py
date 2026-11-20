#!/usr/bin/env python3
"""PreToolUse hook of the cloud sessions for the Agent tool: the effort floor of .claude/rules/cloud-sessions.md.

A subagent names its model, Sonnet or Haiku: one that leaves `model` out inherits the main agent's model. A
subagent on Sonnet runs at effort high, and no subagent runs at max. Sonnet's own default is medium, so a call
that leaves `effort` out is refused too. The built-in search and planning agents and a fork pass untouched.
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
    if effort == "max":
        return 'effort "max" is never used. Repeat the call with effort "high".'
    if not model:
        return ('this Agent call names no model, so it would run on the main agent\'s model. '
                'Repeat the call with model "sonnet" and effort "high" (or model "haiku" for a lookup).')
    if "sonnet" not in model and "haiku" not in model:
        return (f'a subagent runs on Sonnet or Haiku, not on "{model}". '
                'Repeat the call with model "sonnet" and effort "high".')
    if "sonnet" in model and effort not in ("high", "xhigh"):
        said = f'effort "{effort}"' if effort else "no effort"
        return (f'this Agent call runs on Sonnet with {said}. A Sonnet subagent runs at effort "high": '
                'repeat the call with model "sonnet" and effort "high".')
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
