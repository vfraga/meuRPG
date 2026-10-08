#!/usr/bin/env python3
"""Splits data/spells.json into 8 batches of <=40 spells for the tagging sweep."""
import json, os
root = os.path.join(os.path.dirname(__file__), '..')
spells = json.load(open('/home/user/meuRPG/backend/internal/rules/srd51/data/spells.json'))
spells.sort(key=lambda s: s['key'])
os.makedirs(os.path.join(root, 'raw/in'), exist_ok=True)
for i in range(0, len(spells), 40):
    batch = []
    for s in spells[i:i+40]:
        batch.append({'key': s['key'], 'name': s['name'], 'level': s.get('level', 0), 'casting_time': s['casting_time'],
                      'duration': s['duration'], 'concentration': s.get('concentration', False),
                      'desc': ' '.join(s['desc']), 'higher_level': ' '.join(s.get('higher_level', []))})
    json.dump(batch, open(os.path.join(root, f'raw/in/spells-b{i//40+1}.json'), 'w'), indent=1)
print(len(spells), 'spells in', (len(spells)+39)//40, 'batches')
