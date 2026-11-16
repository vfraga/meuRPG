"""Builds monster-kinds.json and monsters.json from the judged kind rows.

Usage: python3 build_monsters.py <monsters.json> <kind rows dir> <out dir>
The kinds of each monster come from inventory.py; a kind row is "party" when a
beast of challenge 1 or lower (a Wild Shape form) or a familiar form has it.
"""
import collections, json, os, subprocess, sys
from fractions import Fraction

monsters_path, rows_dir, out_dir = sys.argv[1:4]
here = os.path.dirname(os.path.abspath(__file__))
inv, per = os.path.join(out_dir, 'p3', 'inventory.json'), os.path.join(out_dir, 'p3', 'kinds-by-monster.json')
subprocess.run([sys.executable, os.path.join(here, 'inventory.py'), monsters_path, inv, per], check=True)
per = json.load(open(per))

# Find Familiar's forms and Pact of the Chain's extra ones (effects/spells.json).
FAMILIARS = {'monster:bat', 'monster:cat', 'monster:crab', 'monster:frog', 'monster:hawk', 'monster:lizard',
             'monster:octopus', 'monster:owl', 'monster:poisonous-snake', 'monster:quipper', 'monster:rat',
             'monster:raven', 'monster:sea-horse', 'monster:spider', 'monster:weasel',
             'monster:imp', 'monster:pseudodragon', 'monster:quasit', 'monster:sprite'}
monsters = json.load(open(monsters_path))
party = {m['key'] for m in monsters if (m['type'] == 'beast' and float(Fraction(str(m['challenge_rating']))) <= 1) or m['key'] in FAMILIARS}

rows = []
for b in ('p3a', 'p3b', 'p3c', 'p3d'):
    rows += json.load(open(os.path.join(rows_dir, b + '.json')))
verdict = {r['key']: r['verdict'] for r in rows}
for r in rows:
    r['party'] = any(r['key'] in per.get(mk, []) for mk in party)
missing = {k for ks in per.values() for k in ks} - set(verdict)
if missing:
    sys.exit(f'kinds with no row: {sorted(missing)}')
json.dump(rows, open(os.path.join(out_dir, 'monster-kinds.json'), 'w'), indent=1, ensure_ascii=False)

out = []
for m in monsters:
    kinds = per.get(m['key'], [])
    out.append({'key': m['key'], 'challenge_rating': m['challenge_rating'], 'party': m['key'] in party,
                'not_runs': [k for k in kinds if verdict[k] != 'runs']})
json.dump(out, open(os.path.join(out_dir, 'monsters.json'), 'w'), indent=1, ensure_ascii=False)
print(len(rows), 'kind rows,', sum(r['party'] for r in rows), 'party;', len(out), 'monsters,', len(party), 'party forms')
print(collections.Counter(r['verdict'] for r in rows))
