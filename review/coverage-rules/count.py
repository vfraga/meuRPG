# Counts the rows of review/coverage-rules.md by state and impact. Run: python3 -I count.py ../coverage-rules.md
import re, sys, collections
rows = []
for l in open(sys.argv[1]).read().split('\n'):
    if not l.startswith('| ') or l.startswith(('| Mechanic', '| Condition:', '| ---')):
        continue
    m = re.match(r'\| (.+?) \| (built|partial|reminder|absent)[^|]* \| (.*) \| (high|medium|low) \|(.*)\|$', l)
    if m:
        rows.append((m.group(2), m.group(4)))
print(len(rows), collections.Counter(r[0] for r in rows))
for st in ('built', 'partial', 'reminder', 'absent'):
    print(st, {i: sum(1 for r in rows if r == (st, i)) for i in ('high', 'medium', 'low')})
