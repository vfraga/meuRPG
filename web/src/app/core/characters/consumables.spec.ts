import { type MessageInitShape, create } from '@bufbuild/protobuf';

import {
  ConsumableTargetSchema,
  ConsumableTargetState,
  UseConsumableResponseSchema,
} from '../../../gen/meurpg/play/v1/charges_pb';
import { ConsumableUseOutcome } from '../../../gen/meurpg/play/v1/combat_pb';
import type { AttunementEntryVm } from './attunement';
import type { ConsumableVm } from './consumable-vm';
import {
  canGive,
  combatItemRows,
  effectIcon,
  effectLine,
  goneNotice,
  itemWords,
  kindWords,
  leftWords,
  magicRows,
  subtitleOf,
  targetRows,
  textOf,
  turnBlockOf,
  useCost,
  useNotice,
} from './consumables';

const POTION: ConsumableVm = {
  kind: 'potion',
  costsAction: true,
  heal: { count: 2, sides: 4, modifier: 2, text: '2d4 + 2' },
};
const RESISTANCE: ConsumableVm = { kind: 'potion', costsAction: true, heal: null };
const OIL: ConsumableVm = { kind: 'oil', costsAction: false, heal: null };
const SCROLL: ConsumableVm = { kind: 'scroll', costsAction: false, heal: null };
const BEAD: ConsumableVm = { kind: 'other', costsAction: false, heal: null };

const entry = (over: Partial<AttunementEntryVm> = {}): AttunementEntryVm => ({
  id: 'm1',
  key: 'item:potion-of-healing',
  namePt: 'Poção de cura',
  kind: 'magic',
  attuned: false,
  asks: false,
  restrictionPt: '',
  restriction: 'none',
  sameItemAttuned: false,
  effectPt: 'Cura 2d4 + 2',
  consumable: POTION,
  ...over,
});

describe('magicRows', () => {
  it('makes one row of the entries that share a key, at the first one, and sends the id of one of them', () => {
    const rows = magicRows([
      entry({ id: 'p1' }),
      entry({
        id: 'r1',
        key: 'item:ring-of-protection',
        namePt: 'Anel',
        effectPt: '',
        consumable: undefined,
      }),
      entry({ id: 'p2' }),
      entry({
        id: 'f1',
        key: 'item:potion-of-fire-resistance',
        namePt: 'Poção de resistência',
        effectPt: 'Resistência a fogo por 1 hora',
        consumable: RESISTANCE,
      }),
    ]);
    expect(rows.map((r) => r.id)).toEqual([
      'consumable:item:potion-of-healing',
      'r1',
      'consumable:item:potion-of-fire-resistance',
    ]);
    const heal = rows[0];
    expect(heal.type === 'consumable' && heal.row.count).toBe(2);
    expect(heal.type === 'consumable' && heal.row.first.id).toBe('p1');
    expect(heal.type === 'consumable' && heal.row.icon).toBe('science');
    expect(rows[1].type).toBe('item');
  });

  it('reads what is a consumable from the field the server filled, and nothing from the key or a catalog', () => {
    const rows = magicRows([
      entry({ id: 'a', key: 'item:potion-of-healing', consumable: undefined }),
      entry({ id: 'b', key: 'item:unheard-of-dust', consumable: BEAD }),
    ]);
    expect(rows.map((r) => r.type)).toEqual(['item', 'consumable']);
  });

  it('draws a flask for a potion, a drop for an oil, a page for a scroll and a box for any other item', () => {
    const icon = (consumable: ConsumableVm) => {
      const [row] = magicRows([entry({ consumable })]);
      return row.type === 'consumable' ? [row.row.kind, row.row.icon] : [];
    };
    expect(icon(POTION)).toEqual(['potion', 'science']);
    expect(icon(OIL)).toEqual(['oil', 'water_drop']);
    expect(icon(SCROLL)).toEqual(['scroll', 'description']);
    expect(icon(BEAD)).toEqual(['other', 'inventory_2']);
  });

  it('keeps an item that is no consumable as the row it was, even with the same key twice', () => {
    const rows = magicRows([
      entry({ id: 'a', key: 'item:ring-of-protection', effectPt: '', consumable: undefined }),
      entry({ id: 'b', key: 'item:ring-of-protection', effectPt: '', consumable: undefined }),
    ]);
    expect(rows.map((r) => r.type)).toEqual(['item', 'item']);
  });
});

describe('the effect line', () => {
  it('says what the server says, or that the item only shows its text', () => {
    expect(effectLine('Cura 2d4 + 2')).toBe('Cura 2d4 + 2');
    expect(effectLine('')).toBe('Só mostra o texto');
  });

  it('draws a heart for a healing the server sent dice for, a shield for a resistance and a book for the text alone', () => {
    expect(effectIcon('Cura 2d4 + 2', POTION)).toBe('favorite');
    expect(effectIcon('Resistência a fogo por 1 hora', RESISTANCE)).toBe('shield');
    expect(effectIcon('', OIL)).toBe('menu_book');
  });
});

describe('kindWords', () => {
  it('says the oil is "o óleo", used and not drunk, and the potion is drunk', () => {
    expect(kindWords('oil')).toMatchObject({
      the: 'o óleo',
      The: 'O óleo',
      use: 'Usar',
      goneLead: 'O óleo acabou.',
    });
    expect(kindWords('potion')).toMatchObject({ the: 'a poção', use: 'Beber' });
    expect(kindWords('scroll').the).toBe('o pergaminho');
    expect(kindWords('other').the).toBe('o item');
  });

  it('gives a potion and an oil to someone, and nothing else', () => {
    expect(canGive('potion')).toBe(true);
    expect(canGive('oil')).toBe(true);
    expect(canGive('scroll')).toBe(false);
    expect(canGive('other')).toBe(false);
  });
});

describe('combatItemRows', () => {
  const units = [
    entry({ id: 'p1' }),
    entry({
      id: 'r1',
      key: 'item:ring-of-protection',
      namePt: 'Anel',
      effectPt: '',
      consumable: undefined,
    }),
    entry({ id: 'p2' }),
    entry({
      id: 'f1',
      key: 'item:potion-of-fire-resistance',
      namePt: 'Poção de resistência',
      effectPt: 'Resistência a fogo por 1 hora',
      consumable: RESISTANCE,
    }),
    entry({
      id: 'o1',
      key: 'item:oil-of-sharpness',
      namePt: 'Óleo de afiar',
      effectPt: '',
      consumable: OIL,
    }),
    entry({
      id: 's1',
      key: 'item:spell-scroll',
      namePt: 'Pergaminho de magia',
      effectPt: '',
      consumable: SCROLL,
    }),
  ];

  it('lists the consumables only, one row for each key, with the id of one unit', () => {
    const rows = combatItemRows(units);
    expect(rows.map((r) => [r.key, r.count, r.first.id])).toEqual([
      ['item:potion-of-healing', 2, 'p1'],
      ['item:potion-of-fire-resistance', 1, 'f1'],
      ['item:oil-of-sharpness', 1, 'o1'],
      ['item:spell-scroll', 1, 's1'],
    ]);
  });

  it('says what the item does in the line under the name, and leaves how many the group holds to its count', () => {
    const rows = combatItemRows(units);
    expect(rows.map((r) => r.detail)).toEqual([
      'cura 2d4 + 2',
      'resistência a fogo por 1 hora',
      'só mostra o texto',
      'só mostra o texto',
    ]);
  });

  it('marks the Ação where the server says the use costs it, and nothing on an oil or a scroll', () => {
    expect(combatItemRows(units).map((r) => r.economy)).toEqual(['action', 'action', null, null]);
  });

  it('marks the Ação from the field and not from the key: a potion the server says is free is free', () => {
    const free = entry({
      key: 'item:potion-of-healing',
      consumable: { ...POTION, costsAction: false },
    });
    expect(combatItemRows([free])[0].economy).toBeNull();
  });

  it('draws the icon of the kind', () => {
    expect(combatItemRows(units).map((r) => r.icon)).toEqual([
      'science',
      'science',
      'water_drop',
      'description',
    ]);
  });

  it('is empty with no consumable, and so is the tab', () => {
    expect(combatItemRows([units[1]])).toEqual([]);
  });
});

describe('turnBlockOf', () => {
  it('says it is not the turn before it says anything of the Action', () => {
    expect(turnBlockOf({ myTurn: false, actionUsed: true })).toBe('turn');
    expect(turnBlockOf({ myTurn: false, actionUsed: false })).toBe('turn');
  });

  it('says the Action is spent on the turn of the character', () => {
    expect(turnBlockOf({ myTurn: true, actionUsed: true })).toBe('action');
  });

  it('blocks nothing while the turn and the Action are there', () => {
    expect(turnBlockOf({ myTurn: true, actionUsed: false })).toBeNull();
  });
});

describe('the words under the title', () => {
  it('says how many are left', () => {
    expect(leftWords(1)).toBe('resta 1');
    expect(leftWords(2)).toBe('restam 2');
  });

  it('opens the owner subtitle with the count and adds what a combat costs', () => {
    expect(
      subtitleOf({
        itemName: 'Poção de cura',
        left: 2,
        actingFor: null,
        inCombat: true,
        costsAction: true,
      }),
    ).toBe('Restam 2 · em combate, gasta a ação');
  });

  it('names the item first for the master, whose title names the character', () => {
    expect(
      subtitleOf({
        itemName: 'Poção de resistência',
        left: 1,
        actingFor: 'Tor',
        inCombat: false,
        costsAction: true,
      }),
    ).toBe('Poção de resistência · resta 1');
  });

  it('keeps the cost out of the line when the page knows there is no combat', () => {
    expect(
      subtitleOf({
        itemName: 'Poção de cura',
        left: 1,
        actingFor: null,
        inCombat: false,
        costsAction: true,
      }),
    ).toBe('Resta 1');
  });

  it('says no cost for an item that costs no action, in a combat or not', () => {
    expect(
      subtitleOf({
        itemName: 'Poção de escalada',
        left: 1,
        actingFor: null,
        inCombat: true,
        costsAction: false,
      }),
    ).toBe('Resta 1');
  });

  it('says nothing of the cost while the sheet does not know whether there is a combat', () => {
    expect(
      subtitleOf({
        itemName: 'Poção de cura',
        left: 3,
        actingFor: null,
        inCombat: null,
        costsAction: true,
      }),
    ).toBe('Restam 3');
  });
});

describe('useCost', () => {
  const base = {
    kind: 'potion' as const,
    costsAction: true,
    inCombat: false as boolean | null,
    blocked: false,
    phase: 'choose' as const,
    receiverName: '',
    canGive: true,
  };

  it('says one case: out of a combat, the turn is not spent, and nothing of the action', () => {
    const cost = useCost(base);
    expect(cost).toEqual({
      mark: false,
      lead: 'Fora de combate: não gasta a vez.',
      rest: ' “Dar a…” lista o grupo todo.',
    });
    expect(`${cost?.lead}${cost?.rest}`).not.toMatch(/gasta a (sua )?ação/);
  });

  it('says the action once, in the subtitle: in a combat the first choice has no cost line, and the list says only what the potion does', () => {
    expect(useCost({ ...base, inCombat: true })).toBeNull();
    expect(useCost({ ...base, inCombat: true, phase: 'give', receiverName: 'Brisa' })).toEqual({
      mark: false,
      lead: '',
      rest: 'A poção age em Brisa.',
    });
    expect(useCost({ ...base, inCombat: true, phase: 'give', receiverName: '' })).toBeNull();
  });

  it('says nothing of the cost while it is not known whether there is a combat', () => {
    expect(useCost({ ...base, inCombat: null })).toBeNull();
  });

  it('says in the words of the oil that it acts on who receives it', () => {
    expect(
      useCost({
        ...base,
        kind: 'oil',
        costsAction: false,
        phase: 'give',
        receiverName: 'Brisa',
      }),
    ).toEqual({
      mark: false,
      lead: 'Não gasta a ação: o mestre decide o tempo que isso leva.',
      rest: ' O óleo age em Brisa.',
    });
  });

  it('says an item that costs no action costs none, in a combat or not', () => {
    for (const inCombat of [true, false, null]) {
      expect(useCost({ ...base, kind: 'scroll', costsAction: false, inCombat })?.lead).toBe(
        'Não gasta a ação: o mestre decide o tempo que isso leva.',
      );
    }
  });

  it('says nothing under a turn that is blocked', () => {
    expect(useCost({ ...base, blocked: true, inCombat: true })).toBeNull();
  });

  it('says the receiver out of a combat does not spend the turn', () => {
    expect(useCost({ ...base, phase: 'give', receiverName: 'Pensantus' })).toEqual({
      mark: false,
      lead: 'Fora de combate, não gasta a vez.',
      rest: ' A poção age em Pensantus.',
    });
  });
});

describe('targetRows', () => {
  const target = (name: string, distanceFt: number, state: ConsumableTargetState) =>
    create(ConsumableTargetSchema, {
      characterId: `id-${name}`,
      name,
      distanceFt,
      state,
      mayReceive: true,
    });

  it('says the distance and how the character stands, as the board does', () => {
    const rows = targetRows([
      target('Brisa', 5, ConsumableTargetState.DOWN),
      target('Jabuti', 5, ConsumableTargetState.OK),
    ]);
    expect(rows.map((r) => [r.id, r.name, r.line, r.down])).toEqual([
      ['id-Brisa', 'Brisa', 'A 1,5\u00a0m · Caída', true],
      ['id-Jabuti', 'Jabuti', 'A 1,5\u00a0m · De pé', false],
    ]);
  });

  it('says "Caído" for a name of a man and "Caída" for a name of a woman, as the combat does', () => {
    const rows = targetRows([
      target('Toren', 5, ConsumableTargetState.DOWN),
      target('Brisa', 5, ConsumableTargetState.DOWN),
    ]);
    expect(rows.map((r) => r.line)).toEqual(['A 1,5\u00a0m · Caído', 'A 1,5\u00a0m · Caída']);
  });

  it('keeps what the list needs of a receiver and nothing of how far they are', () => {
    const [row] = targetRows([target('Brisa', 5, ConsumableTargetState.DOWN)]);
    expect(Object.keys(row).sort()).toEqual(['down', 'id', 'line', 'name']);
  });

  it('says nothing of a distance or of how they stand out of a combat, where the server tells neither', () => {
    const rows = targetRows([
      target('Pensantus', 0, ConsumableTargetState.UNSPECIFIED),
      target('Lia', 0, ConsumableTargetState.UNSPECIFIED),
    ]);
    expect(rows.map((r) => [r.line, r.down])).toEqual([
      ['', false],
      ['', false],
    ]);
  });

  it('lists every receiver the server sent: it only sends who may receive', () => {
    const rows = targetRows([
      create(ConsumableTargetSchema, { characterId: 'x', name: 'Zé', mayReceive: false }),
    ]);
    expect(rows.map((r) => r.id)).toEqual(['x']);
  });
});

describe('useNotice', () => {
  const healed = (over: MessageInitShape<typeof UseConsumableResponseSchema> = {}) =>
    create(UseConsumableResponseSchema, {
      outcome: ConsumableUseOutcome.HEALED,
      diceText: '2d4+2',
      faces: [3, 2],
      rollTotal: 7,
      healed: 7,
      ...over,
    });
  const base = {
    itemName: 'Poção de cura',
    kind: 'potion' as const,
    effectPt: 'Cura 2d4 + 2',
    heal: POTION.heal,
    userName: 'Tor',
    receiverName: 'Brisa',
    gift: true,
    woke: true,
    left: 1,
  };

  it('says who got the hit points and wakes, with the dice written out and what is left', () => {
    expect(useNotice(healed(), base)).toEqual({
      tone: 'good',
      icon: 'favorite',
      headline: 'Brisa recebeu 7 PV e acorda.',
      detail: '2d4 + 2: 3 + 2 + 2 = 7. Resta 1 Poção de cura.',
      text: [],
    });
  });

  it('does not wake who was on their feet', () => {
    expect(
      useNotice(healed(), { ...base, woke: false, receiverName: 'Tor', gift: false }).headline,
    ).toBe('Tor recebeu 7 PV.');
  });

  it('says what is left when none is, and when there are several', () => {
    expect(useNotice(healed(), { ...base, left: 0 }).detail).toContain('Era a última unidade.');
    expect(useNotice(healed(), { ...base, left: 3 }).detail).toContain(
      'Restam 3 unidades de Poção de cura.',
    );
  });

  it('says nothing is known of what is left when the read of the sheet is not on screen', () => {
    const n = useNotice(healed(), { ...base, left: null });
    expect(n.detail).toBe('2d4 + 2: 3 + 2 + 2 = 7.');
  });

  it('is a neutral notice with its own lead for who had all their hit points: no success, no dice', () => {
    const n = useNotice(healed({ healed: 0, faces: [1, 1], rollTotal: 4 }), {
      ...base,
      woke: false,
    });
    expect(n).toEqual({
      tone: 'neutral',
      icon: 'info',
      headline: 'Brisa já estava com os PV cheios.',
      detail: 'Nada foi curado, mas a unidade foi gasta. Resta 1 Poção de cura.',
      text: [],
    });
    expect(n.detail).not.toContain('=');
  });

  it('writes a number taken off the roll with its sign', () => {
    const n = useNotice(healed({ faces: [2], rollTotal: 1, healed: 1, diceText: '1d4-1' }), {
      ...base,
      effectPt: 'Cura 1d4 − 1',
      heal: { count: 1, sides: 4, modifier: -1, text: '1d4 − 1' },
      woke: false,
    });
    expect(n.detail).toContain('1d4 − 1: 2 − 1 = 1.');
  });

  it('lands a resistance in "Seus efeitos" and says so in a line', () => {
    const n = useNotice(
      create(UseConsumableResponseSchema, {
        outcome: ConsumableUseOutcome.RESISTANCE,
        damageTypeKey: 'damage:fire',
      }),
      {
        ...base,
        itemName: 'Poção de resistência',
        effectPt: 'Resistência a fogo por 1 hora',
        receiverName: 'Tor',
        gift: false,
        woke: false,
      },
    );
    expect(n).toEqual({
      tone: 'good',
      icon: 'check_circle',
      headline: 'Tor bebeu a Poção de resistência: resistência a fogo por 1 hora.',
      detail: '',
      text: [],
    });
  });

  it('says a resistance given to someone was received', () => {
    const n = useNotice(
      create(UseConsumableResponseSchema, { outcome: ConsumableUseOutcome.RESISTANCE }),
      { ...base, itemName: 'Poção de resistência', effectPt: 'Resistência a fogo por 1 hora' },
    );
    expect(n.headline).toBe('Brisa recebeu a Poção de resistência: resistência a fogo por 1 hora.');
  });

  it('is neutral for a potion with no effect of its own and brings the text of the item', () => {
    const n = useNotice(
      create(UseConsumableResponseSchema, {
        outcome: ConsumableUseOutcome.TEXT_ONLY,
        text: ['Por 1 hora, você escala com o mesmo deslocamento.'],
      }),
      { ...base, itemName: 'Poção de escalada', effectPt: '', gift: false, receiverName: 'Tor' },
    );
    expect(n).toEqual({
      tone: 'neutral',
      icon: 'menu_book',
      headline: 'Tor bebeu a Poção de escalada.',
      detail: 'O app não aplica este efeito: o mestre e você seguem o texto.',
      text: ['Por 1 hora, você escala com o mesmo deslocamento.'],
    });
  });

  it('heads the text of an item with its first line when more follow: the rarity is a caption, not a box', () => {
    const n = useNotice(
      create(UseConsumableResponseSchema, {
        outcome: ConsumableUseOutcome.TEXT_ONLY,
        text: ['Poção, muito rara', 'Este óleo gelatinoso.', 'Aplicar o óleo leva 1 minuto.'],
      }),
      { ...base, itemName: 'Óleo de precisão', kind: 'oil', effectPt: '', gift: false },
    );
    expect(n.caption).toBe('Poção, muito rara');
    expect(n.text).toEqual(['Este óleo gelatinoso.', 'Aplicar o óleo leva 1 minuto.']);
  });

  it('says an oil is used and not drunk, with the article of its name', () => {
    const text = create(UseConsumableResponseSchema, { outcome: ConsumableUseOutcome.TEXT_ONLY });
    const oil = { ...base, itemName: 'Óleo de precisão', kind: 'oil' as const, effectPt: '' };
    expect(useNotice(text, { ...oil, gift: false, receiverName: 'Tor' }).headline).toBe(
      'Tor usou o Óleo de precisão.',
    );
    expect(useNotice(text, oil).headline).toBe('Tor deu o Óleo de precisão a Brisa.');
  });

  it('says who the potion was given to when it has no effect of its own', () => {
    const n = useNotice(
      create(UseConsumableResponseSchema, { outcome: ConsumableUseOutcome.TEXT_ONLY }),
      {
        ...base,
        itemName: 'Poção de escalada',
        effectPt: '',
      },
    );
    expect(n.headline).toBe('Tor deu a Poção de escalada a Brisa.');
  });
});

describe('goneNotice', () => {
  it('is a neutral notice with the lead of the kind: "A poção acabou.", "O óleo acabou."', () => {
    expect(goneNotice('potion')).toEqual({
      tone: 'neutral',
      icon: 'info',
      headline: 'A poção acabou.',
      detail: 'Essa poção já foi usada. A ficha foi atualizada.',
      text: [],
    });
    expect(goneNotice('oil').headline).toBe('O óleo acabou.');
  });
});

describe('itemWords', () => {
  it('says a potion is "uma", and drunk when the server says it is a potion', () => {
    expect(itemWords('Poção de cura', 'potion')).toEqual({
      definite: 'a Poção de cura',
      indefinite: 'uma Poção de cura',
      verb: 'bebeu',
    });
  });

  it('writes no rule of its own: a name that sounds like a potion is used, not drunk, when the kind is not said', () => {
    expect(itemWords('Poção de cura').verb).toBe('usou');
  });

  it('says a potion the sheet names after its kind is drunk, and an oil or a scroll is used, whatever the name', () => {
    expect(itemWords('Elixir', 'potion').verb).toBe('bebeu');
    expect(itemWords('Poção estranha', 'oil').verb).toBe('usou');
    expect(itemWords('Poção estranha', 'other').verb).toBe('usou');
  });

  it('says anything else is used, with the article of its name', () => {
    expect(itemWords('Óleo de precisão')).toMatchObject({
      definite: 'o Óleo de precisão',
      verb: 'usou',
    });
    expect(itemWords('Pergaminho de magia').indefinite).toBe('um Pergaminho de magia');
    expect(itemWords('Conta de força').indefinite).toBe('uma Conta de força');
  });
});

describe('textOf', () => {
  it('keeps a single line as the text, with no caption', () => {
    expect(textOf(['Por 1 hora.'])).toEqual({ text: ['Por 1 hora.'] });
    expect(textOf([])).toEqual({ text: [] });
  });

  it('takes the first of several lines as the caption', () => {
    expect(textOf(['Poção, rara', 'a', 'b'])).toEqual({ caption: 'Poção, rara', text: ['a', 'b'] });
  });
});
