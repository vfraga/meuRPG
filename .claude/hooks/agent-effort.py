#!/usr/bin/env python3
"""PreToolUse hook of the cloud sessions for the Agent tool: the rules of .claude/rules/cloud-sessions.md.

Every subagent is one of the named agents of .claude/agents (implementer, reviewer, explorer): their definition
sets the model, the effort and the limits. Any other agent type is refused, except the built-in search and
planning agents and a fork. A named agent is refused when the call overrides its model with one that is not
Sonnet or Haiku, or its effort with low, xhigh or max. An implementer is refused until its prompt names the
task's reading list, a file ending in "reading.md" that exists: an explorer writes it first.

A session that works under the earlier rules (a control run) has the file ~/.meurpg-earlier-agent-rules: there a
general-purpose agent passes when the call names Sonnet or Haiku and, on Sonnet, an effort of medium or high.

Exit code 2 refuses the call and shows the reason to the main agent, which repeats the call with the right
values. Anything this hook cannot read passes: it never stops work it does not understand.
"""
import json
import os
import re
import sys

PASS_TYPES = {"explore", "plan", "fork", "claude-code-guide", "statusline-setup", "web-fetch"}
NAMED_TYPES = {"implementer", "reviewer", "explorer", "turncap-smoke"}


def reading_list_named(prompt):
    """True when the prompt names an existing file whose name ends in reading.md."""
    for path in re.findall(r"[~/][\w./~-]*reading\.md", prompt or ""):
        if os.path.isfile(os.path.expanduser(path)):
            return True
    return False


def earlier_rules(tool_input):
    model = str(tool_input.get("model") or "").lower()
    effort = str(tool_input.get("effort") or "").lower()
    if not model:
        return ("this Agent call names no model, so it would run on the main agent's model. "
                'Repeat the call with model "sonnet" and effort "medium" or "high" (or model "haiku" for a lookup).')
    if "sonnet" not in model and "haiku" not in model:
        return (f'a subagent runs on Sonnet or Haiku, not on "{model}". '
                'Repeat the call with model "sonnet" and effort "medium" or "high".')
    if "sonnet" in model and effort not in ("medium", "high"):
        said = f'effort "{effort}"' if effort else "no effort"
        return (f'this Agent call runs on Sonnet with {said}. A Sonnet subagent names its effort: "medium" when it '
                'writes code, "high" when it reviews or investigates. Repeat the call with one of them.')
    return None


def verdict(tool_input):
    kind = str(tool_input.get("subagent_type") or "").lower()
    if kind in PASS_TYPES:
        return None
    model = str(tool_input.get("model") or "").lower()
    effort = str(tool_input.get("effort") or "").lower()
    if effort in ("max", "xhigh", "low"):
        return (f'effort "{effort}" is never used for a subagent. Repeat the call without `effort`: '
                "the agent's definition sets it.")
    if kind in NAMED_TYPES:
        if model and "sonnet" not in model and "haiku" not in model:
            return (f'a subagent runs on Sonnet or Haiku, not on "{model}". Repeat the call without `model`: '
                    f'the definition of "{kind}" sets it.')
        if kind == "implementer" and not reading_list_named(str(tool_input.get("prompt") or "")):
            return ("an implementer starts from its task's reading list, and this prompt names no existing file "
                    'ending in "reading.md". Start an `explorer` first: it writes the list. Then repeat this call '
                    "with the list's path and the state file's path in the prompt.")
        return None
    if os.path.exists(os.path.expanduser("~/.meurpg-earlier-agent-rules")):
        return earlier_rules(tool_input)
    said = f'"{kind}"' if kind else "no agent type"
    return (f"this Agent call uses {said}. A subagent is one of the named agents: `implementer` (writes or fixes), "
            "`reviewer` (reviews or investigates) or `explorer` (reading lists and look-ups). Repeat the call "
            "with `subagent_type` set to one of them and without `model` and `effort`.")


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
