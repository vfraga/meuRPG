import type { EffectDraft } from './effect-draft.types';

/** How a name is said: a weapon property by the menu's name, a resource as the entry calls it. */
export interface PowerAttackNames {
  property(key: string): string;
  resource(key: string): string;
}

const KIND_WORDS: Readonly<Record<string, string>> = {
  melee: ' corpo a corpo',
  ranged: ' à distância',
};

/** "Pesada", "Pesada e Duas mãos", "Pesada, Leve e Duas mãos". */
function listed(items: readonly string[]): string {
  return items.length < 2
    ? (items[0] ?? '')
    : `${items.slice(0, -1).join(', ')} e ${items[items.length - 1]}`;
}

/** The filter of a power attack read back in words, after "Vale para:": the weapon kind, the properties it must all have, whether
 * the character must be proficient with the weapon and, when the trade spends a resource, what a use costs. With no property the
 * sentence is "qualquer arma ..."; with some it is "armas ..., com a propriedade ...". */
export function powerAttackReading(
  d: Pick<EffectDraft, 'weapon' | 'weaponProperties' | 'onlyProficient' | 'resource'>,
  names: PowerAttackNames,
): string {
  const kind = KIND_WORDS[d.weapon] ?? '';
  const props = d.weaponProperties.map((k) => names.property(k));
  const who = d.onlyProficient;
  const base =
    props.length === 0
      ? `qualquer arma${kind}${who ? ' em que o personagem tem proficiência' : ', mesmo uma em que o personagem não tem proficiência'}`
      : `armas${kind}, com ${props.length === 1 ? 'a propriedade' : 'as propriedades'} ${listed(props)}${who ? ', em que o personagem tem proficiência' : ', mesmo as em que o personagem não tem proficiência'}`;
  const resource = d.resource.trim();
  return resource === ''
    ? `${base}.`
    : `${base}. Cada troca gasta 1 uso de ${names.resource(resource)}.`;
}
