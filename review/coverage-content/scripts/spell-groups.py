#!/usr/bin/env python3
"""Groups spells-states.json by what is missing. Writes spells-groups.md."""
import json, collections, os
root = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..')
r = json.load(open(os.path.join(root, 'spells-states.json')))
out = []
cnt = collections.Counter(x['state'] for x in r)
out.append('## counts by state\n\n' + '\n'.join(f'- {k}: {v}' for k, v in cnt.most_common()) + '\n')
by_level = collections.defaultdict(collections.Counter)
for x in r: by_level[x['level']][x['state']] += 1
out.append('## state by spell level\n\n| level | built | partial | reminder | absent |\n|---|---|---|---|---|')
for l in sorted(by_level): out.append(f"| {l} | {by_level[l]['built']} | {by_level[l]['partial']} | {by_level[l]['reminder']} | {by_level[l]['absent']} |")
groups = collections.defaultdict(list)
for x in r:
    if x['state'] in ('partial', 'reminder'):
        for g in x['missing']:
            groups[g.split(':')[0]].append(x['key'][6:])
out.append('\n## spells that are partial or reminder, by what is missing (a spell can be in several groups)\n')
for g, ks in sorted(groups.items(), key=lambda kv: -len(kv[1])):
    out.append(f'- **{g}**: {len(ks)} - ' + ', '.join(sorted(ks)))
cond = collections.defaultdict(list)
for x in r:
    if x['state'] in ('partial', 'reminder'):
        for c in x['conditions']: cond[c].append(x['key'][6:])
out.append('\n## conditions a spell should impose and the app does not apply\n')
for c, ks in sorted(cond.items(), key=lambda kv: -len(kv[1])): out.append(f'- {c}: {len(ks)} - ' + ', '.join(sorted(ks)))
conc = [x['key'][6:] for x in r if x['concentration'] and x['state'] != 'absent']
out.append(f"\n## concentration spells castable in combat: {len(conc)} (flag set and DC reminder only; no auto end on duration, incapacitation or failed save)\n")
rit = [x['key'][6:] for x in r if x['ritual']]
out.append(f"\n## rituals: {len(rit)} ({', '.join(rit)}); only find-familiar can be cast as a ritual (CastSummon); {sum(1 for x in r if x['ritual'] and x['state']=='absent')} of them are absent\n")
open(os.path.join(root, 'spells-groups.md'), 'w').write('\n'.join(out))
print('\n'.join(out)[:6000])
