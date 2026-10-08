#!/usr/bin/env python3
"""Assembles ../coverage-content.md from report-src/*.md, filling {{PLACEHOLDERS}} from the JSON the other scripts write.
Run the other scripts first (see the README section at the end of the report)."""
import json, glob, os, re, collections
root = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..')
J = lambda n: json.load(open(os.path.join(root, n)))
fill = {}

# ---- spells
if os.path.exists(os.path.join(root, 'spells-states.json')):
    S = J('spells-states.json')
    cnt = collections.Counter(x['state'] for x in S)
    lv = collections.defaultdict(collections.Counter)
    for x in S: lv[x['level']][x['state']] += 1
    t = ['| State | Spells | What it means here |', '| --- | --- | --- |',
         f"| built | {cnt['built']} | the cast computes what the text asks for in a fight |",
         f"| partial | {cnt['partial']} | the cast rolls an attack, a save, damage, a heal or a hit point effect, and a named part is left to the table |",
         f"| reminder | {cnt['reminder']} | the cast spends the slot and the action, sets concentration if any, and logs; nothing is computed |",
         f"| absent | {cnt['absent']} | the app refuses to cast it: casting time of a minute or more (57) or a reaction other than Shield (3) |",
         f"| **total** | **{len(S)}** | |", '', '| spell level | built | partial | reminder | absent |', '| --- | --- | --- | --- | --- |']
    for l in sorted(lv): t.append(f"| {l} | {lv[l]['built']} | {lv[l]['partial']} | {lv[l]['reminder']} | {lv[l]['absent']} |")
    imp = collections.Counter((x['state'], x['impact']) for x in S if x['state'] != 'built')
    t += ['', 'Table impact of the 284 spells that are not built: ' + ', '.join(f"{i} {sum(v for (s,k),v in imp.items() if k==i)}" for i in ('high', 'medium', 'low')) + '.']
    fill['SPELL_COUNTS'] = '\n'.join(t)
    g = open(os.path.join(root, 'spells-groups.md')).read()
    fill['SPELL_GROUPS'] = re.sub(r'^## ', '#### ', g.split('## spells that are partial or reminder, by what is missing')[1].split('\n', 1)[1], flags=re.M)
    EV = {'attack_roll': 'combat_spells.go:598', 'save_roll': 'combat_spells.go:636', 'darts': 'combat_spells.go:670', 'heal': 'combat_spells.go:574',
          'summon': 'creature_cast.go:170', 'concentration_flag': 'combat_spells.go:418'}
    rows = ['| Spell | L | State | Impact | Engine does | Missing / why | Evidence | Deliberate? |', '| --- | --- | --- | --- | --- | --- | --- | --- |']
    for x in S:
        does = [] if x['state'] == 'absent' else [d.replace('concentration_flag', 'conc').replace('area_targets_picked_by_hand', 'area: targets picked by hand') for d in x['engine_does']]
        ev = sorted({EV[d] for d in x['engine_does'] if d in EV} | ({'combat_spells_hp.go:91'} if any(d.startswith('hp_effect') for d in x['engine_does']) else set()))
        if x['state'] == 'reminder' and not ev: ev = ['cast-result.ts:63 (plain)']
        elif x['state'] == 'reminder': ev.append('cast-result.ts:63')
        if x['state'] == 'absent': ev = ['turn.go:482' if 'reaction' in x['casting_time'] else 'turn.go:478']
        if x['key'] == 'spell:shield': ev = ['combat_reactions.go:169']
        delib = 'no doc'
        if x['state'] == 'absent': delib = 'architecture.md:821' if 'reaction' in x['casting_time'] else 'architecture.md:823'
        elif x['state'] in ('partial', 'reminder') and x['missing']: delib = 'rules.md:329'
        elif x['state'] == 'built': delib = '-'
        miss = ', '.join(x['missing']) if x['missing'] else ''
        why = x['why'] or x['note']
        text = (miss + ('; ' if miss and why else '') + why).replace('|', '/')
        rows.append(f"| {x['name_pt'] or x['key'][6:]} (`{x['key'][6:]}`) | {x['level']} | {x['state']} | {x['impact']} | {', '.join(does) or '-'} | {text} | {'; '.join('`'+e+'`' for e in ev)} | {delib} |")
    fill['SPELL_TABLE'] = '\n'.join(rows)

parts = sorted(glob.glob(os.path.join(root, 'report-src/*.md')))
out = []
for p in parts:
    s = open(p).read()
    for k, v in fill.items(): s = s.replace('{{' + k + '}}', v)
    left = re.findall(r'\{\{[A-Z_]+\}\}', s)
    if left: print('WARNING unfilled', p, left)
    out.append(s.rstrip() + '\n')
open(os.path.join(root, '..', 'coverage-content.md'), 'w').write('\n'.join(out))
print('wrote', os.path.abspath(os.path.join(root, '..', 'coverage-content.md')), sum(len(o) for o in out), 'chars')
