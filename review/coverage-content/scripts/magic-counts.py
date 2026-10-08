#!/usr/bin/env python3
"""Counts the 362 magic items (data/magic-items.json) by category, attunement, charges, bonus and what they do.
Writes magic-counts.json and magic-items-by-effect.json (raw list: key, category, what the text asks for)."""
import json, re, collections, os
root = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..')
I = json.load(open('/home/user/meuRPG/backend/internal/rules/srd51/data/magic-items.json'))
cons = json.load(open('/home/user/meuRPG/backend/internal/rules/srd51/effects/consumables.json'))
consumable_cats = set(cons['categories']); consumable_items = set(cons['items'])
rows = []
for it in I:
    txt = ' '.join(it.get('desc') or [])
    low = txt.lower()
    r = {'key': it['key'], 'category': it['category'], 'rarity': it['rarity'], 'attunement': bool(it.get('attunement')),
         'attunement_by': it.get('attunement_by', ''), 'family': bool(it.get('variants')), 'variant_of': it.get('variant_of', ''),
         'consumable': it['category'] in consumable_cats or it['key'] in consumable_items,
         'charges': bool(re.search(r'\bcharges?\b', low)),
         'dawn_recharge': 'regains' in low and 'dawn' in low,
         'bonus_plus': bool(re.search(r'\+\d\b', it['name'])) or bool(re.search(r'bonus to (attack|ac|armor class)', low)),
         'ac_bonus': bool(re.search(r'(\+\d bonus to ac|bonus to ac|armor class)', low)) and it['category'] in ('armor', 'ring', 'wondrous-item'),
         'saves_bonus': 'saving throws' in low and 'bonus' in low,
         'casts_spell': bool(re.search(r'cast the .{0,40} spell|can cast|spell save dc|expend', low)),
         'damage_resistance': 'resistance to' in low or 'resistance to ' in low,
         'ability_score': bool(re.search(r'(strength|dexterity|constitution|intelligence|wisdom|charisma) score (is|becomes|increases)|score (increases|changes|is 19|becomes)', low)),
         'speed': bool(re.search(r'(walking|flying|swim\w*) speed', low)),
         }
    rows.append(r)
n = lambda f: sum(1 for r in rows if f(r))
C = collections.OrderedDict()
C['items'] = len(rows)
C['by_category'] = dict(collections.Counter(r['category'] for r in rows))
C['by_rarity'] = dict(collections.Counter(r['rarity'] for r in rows))
C['families'] = n(lambda r: r['family']); C['variants'] = n(lambda r: r['variant_of'])
C['attunement_required'] = n(lambda r: r['attunement'])
C['attunement_by_class_or_kind'] = n(lambda r: r['attunement_by'])
C['consumable'] = n(lambda r: r['consumable'])
C['mentions_charges'] = n(lambda r: r['charges'])
C['dawn_recharge'] = n(lambda r: r['dawn_recharge'])
C['name_or_text_plus_bonus'] = n(lambda r: r['bonus_plus'])
C['grants_damage_resistance'] = n(lambda r: r['damage_resistance'])
C['casts_spells'] = n(lambda r: r['casts_spell'])
C['sets_ability_score'] = n(lambda r: r['ability_score'])
C['changes_speed'] = n(lambda r: r['speed'])
json.dump(rows, open(os.path.join(root, 'magic-items-by-effect.json'), 'w'), indent=1)
json.dump(C, open(os.path.join(root, 'magic-counts.json'), 'w'), indent=1)
print(json.dumps(C, indent=1))
for key in ('potion-of-healing', 'ring-of-protection', 'cloak-of-protection', 'bag-of-holding', 'armor-1', 'weapon-1', 'wand-of-magic-missiles', 'spell-scroll'):
    print(key, [r for r in rows if r['key'] == 'item:' + key][:1])
