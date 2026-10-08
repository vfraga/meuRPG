#!/usr/bin/env python3
"""Monster feature counts (data/monsters.json) and what a combat NPC made from the stat block keeps.
Writes monsters-features.json (per monster) and monsters-counts.json (totals)."""
import json, re, collections, os
root = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..')
M = json.load(open('/home/user/meuRPG/backend/internal/rules/srd51/data/monsters.json'))
DT = {'acid','bludgeoning','cold','fire','force','lightning','necrotic','piercing','poison','psychic','radiant','slashing','thunder'}
def dice(s):
    m = re.fullmatch(r'(\d+)d(\d+)(?:\s*([+-])\s*(\d+))?', s.replace(' ', ''))
    return (int(m[1]), int(m[2])) if m else None
def on_sheet(a):
    """characters.basicAttackOf: weapon/spell attack, damage type in the enum, 1..20 dice of d4/6/8/10/12."""
    if not a.get('has_attack') or not a.get('damage'): return False
    d = a['damage'][0]; t = d['damage_type'].split(':')[1]
    x = dice(d['dice'])
    return t in DT and x is not None and 1 <= x[0] <= 20 and x[1] in (4, 6, 8, 10, 12)
trait_names = collections.Counter(); rows = []
FLAGS = {
 'pack_tactics': lambda m: any(a['name'] == 'Pack Tactics' for a in m.get('special_abilities') or []),
 'magic_resistance': lambda m: any(a['name'] == 'Magic Resistance' for a in m.get('special_abilities') or []),
 'legendary_resistance': lambda m: any(a['name'].startswith('Legendary Resistance') for a in m.get('special_abilities') or []),
 'regeneration': lambda m: any(a['name'] == 'Regeneration' for a in m.get('special_abilities') or []),
 'undead_fortitude': lambda m: any(a['name'] == 'Undead Fortitude' for a in m.get('special_abilities') or []),
 'innate_spellcasting': lambda m: any(a['name'] == 'Innate Spellcasting' for a in m.get('special_abilities') or []),
 'spellcasting': lambda m: any(a['name'] == 'Spellcasting' for a in m.get('special_abilities') or []),
 'magic_weapons': lambda m: any(a['name'] == 'Magic Weapons' for a in m.get('special_abilities') or []),
 'sunlight_sensitivity': lambda m: any('Sunlight' in a['name'] for a in m.get('special_abilities') or []),
 'swarm': lambda m: any(a['name'] == 'Swarm' for a in m.get('special_abilities') or []),
 'shapechanger': lambda m: any(a['name'] == 'Shapechanger' for a in m.get('special_abilities') or []),
 'amphibious_or_water_breathing': lambda m: any(a['name'] in ('Amphibious', 'Water Breathing', 'Hold Breath') for a in m.get('special_abilities') or []),
 'keen_senses': lambda m: any(a['name'].startswith('Keen ') for a in m.get('special_abilities') or []),
 'charge_pounce_trample': lambda m: any(a['name'] in ('Charge', 'Pounce', 'Trampling Charge', 'Reckless', 'Flyby') for a in m.get('special_abilities') or []),
}
for m in M:
    sa = m.get('special_abilities') or []; ac = m.get('actions') or []
    for a in sa: trait_names[a['name']] += 1
    attacks = [a for a in ac if a.get('has_attack')]
    sheet = [a for a in attacks if on_sheet(a)][:3]
    dropped = [a['name'] for a in attacks if a not in sheet]
    extra_dmg = [a['name'] for a in sheet if len(a['damage']) > 1]
    saves = [a for a in ac if a.get('save')]
    usage = [(a['name'], a['usage']) for a in ac if a.get('usage')]
    recharge = [u for u in usage if 'Recharge' in u[1]]
    per_day = [u for u in usage if re.search(r'\d/Day', u[1], re.I)]
    desc_all = ' '.join(a['desc'] for a in ac)
    riders = sorted(set(re.findall(r'(grappled|restrained|poisoned|paralyzed|knocked prone|prone|frightened|stunned|blinded|charmed|petrified|incapacitated|unconscious|diseased|curse|exhaustion|level of exhaustion)', desc_all)))
    mult = [a for a in ac if a['name'] == 'Multiattack']
    r = {'key': m['key'], 'cr': m['challenge_rating'],
         'attack_actions': len(attacks), 'on_npc_sheet': len(sheet), 'dropped_from_sheet': dropped, 'second_damage_part_lost': extra_dmg,
         'save_actions': [a['name'] for a in saves], 'recharge': recharge, 'per_day': per_day,
         'multiattack': bool(mult), 'legendary_actions': [a['name'] for a in m.get('legendary_actions') or []],
         'reactions': [a['name'] for a in m.get('reactions') or []], 'condition_riders_in_actions': riders,
         'condition_immunities': m.get('condition_immunities') or [],
         'resist_notes': [d['note'] for k in ('resistances', 'immunities', 'vulnerabilities') for d in m.get(k) or [] if d.get('note')],
         'senses': {k: m.get(k) for k in ('darkvision', 'blindsight', 'tremorsense', 'truesight') if m.get(k)}}
    for k, f in FLAGS.items(): r[k] = f(m)
    rows.append(r)
json.dump(rows, open(os.path.join(root, 'monsters-features.json'), 'w'), indent=1)
C = collections.OrderedDict()
n = lambda f: sum(1 for r in rows if f(r))
C['monsters'] = len(rows)
C['with_attack_action'] = n(lambda r: r['attack_actions'] > 0)
C['with_multiattack'] = n(lambda r: r['multiattack'])
C['more_than_3_attack_actions'] = n(lambda r: r['attack_actions'] > 3)
C['attack_actions_total'] = sum(r['attack_actions'] for r in rows)
C['attack_actions_not_on_npc_sheet'] = sum(len(r['dropped_from_sheet']) for r in rows)
C['monsters_losing_an_attack'] = n(lambda r: r['dropped_from_sheet'])
C['attacks_with_second_damage_part_lost'] = sum(len(r['second_damage_part_lost']) for r in rows)
C['monsters_with_save_action'] = n(lambda r: r['save_actions'])
C['save_actions_total'] = sum(len(r['save_actions']) for r in rows)
C['monsters_with_recharge'] = n(lambda r: r['recharge'])
C['recharge_actions_total'] = sum(len(r['recharge']) for r in rows)
C['monsters_with_n_per_day_action'] = n(lambda r: r['per_day'])
C['monsters_with_legendary_actions'] = n(lambda r: r['legendary_actions'])
C['legendary_actions_total'] = sum(len(r['legendary_actions']) for r in rows)
C['monsters_with_reactions'] = n(lambda r: r['reactions'])
C['monsters_with_condition_immunities'] = n(lambda r: r['condition_immunities'])
C['monsters_with_conditional_damage_notes'] = n(lambda r: r['resist_notes'])
C['monsters_with_condition_rider_in_actions'] = n(lambda r: r['condition_riders_in_actions'])
for k in FLAGS: C['trait_' + k] = n(lambda r, k=k: r[k])
for s in ('darkvision', 'blindsight', 'tremorsense', 'truesight'): C['sense_' + s] = n(lambda r, s=s: s in r['senses'])
C['monsters_with_any_trait'] = sum(1 for m in M if m.get('special_abilities'))
C['distinct_trait_names'] = len(trait_names)
C['reaction_names'] = sorted({x for r in rows for x in r['reactions']})
C['top_traits'] = trait_names.most_common(25)
json.dump(C, open(os.path.join(root, 'monsters-counts.json'), 'w'), indent=1)
print(json.dumps(C, indent=1))
