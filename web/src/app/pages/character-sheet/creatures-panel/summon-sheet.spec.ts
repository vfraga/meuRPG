import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { ReplacedCreatureSchema } from '../../../../gen/meurpg/characters/v1/characters_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import {
  FakeCreaturesClient,
  beastSpell,
  familiarSpell,
  flat,
  isOff,
  raven,
  summary,
  summonAnswer,
  undeadSpell,
} from '../../../core/creatures/creatures-testing';
import { SummonSheet, type SummonSheetData } from './summon-sheet';

describe('SummonSheet: casting a summon outside combat (E9-10, MR-037)', () => {
  let api: FakeCreaturesClient;
  let close: ReturnType<typeof vi.fn>;

  async function setup(
    spellKey: string,
    spells = [familiarSpell(), undeadSpell(), beastSpell()],
    slots: [number, number, number, boolean?][] = [
      [1, 4, 4],
      [3, 2, 1],
      [4, 1, 0],
    ],
  ) {
    api = new FakeCreaturesClient();
    api.options = summonAnswer(spells, slots);
    for (const [k, n, hp, ft, fly] of [
      ['bat', 'Morcego', 1, 5, 30],
      ['cat', 'Gato', 2, 40, 0],
      ['raven', 'Corvo', 1, 10, 50],
    ] as const) {
      api.blocks.set(
        `monster:${k}`,
        raven({
          summary: summary(`monster:${k}`, n),
          hitPoints: hp,
          speedWalkFt: ft,
          speedFlyFt: fly,
        }),
      );
    }
    api.blocks.set(
      'monster:skeleton',
      raven({ summary: summary('monster:skeleton', 'Esqueleto', { typePt: 'morto-vivo' }) }),
    );
    api.blocks.set('monster:zombie', raven({ summary: summary('monster:zombie', 'Zumbi') }));
    api.catalog = [
      summary('monster:wolf', 'Lobo', { challengeRating: '1/4', sizePt: 'Médio' }),
      summary('monster:bear', 'Urso negro', { challengeRating: '1/2' }),
    ];
    close = vi.fn();
    const data: SummonSheetData = { campaignId: 'camp-1', characterId: 'char-1', spellKey };
    TestBed.configureTestingModule({
      providers: [
        { provide: CreaturesClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(SummonSheet);
    const settle = async () => {
      for (let i = 0; i < 5; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await new Promise((r) => setTimeout(r));
      }
      fixture.detectChanges();
    };
    fixture.detectChanges();
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        flat(b)?.includes(name),
      )!;
    const radio = (name: string) =>
      Array.from(el.querySelectorAll<HTMLLabelElement>('label'))
        .find((l) => flat(l)?.includes(name))!
        .querySelector<HTMLInputElement>('input[type=radio]')!;
    const pick = async (name: string) => {
      const r = radio(name);
      r.checked = true;
      r.dispatchEvent(new Event('change'));
      await settle();
    };
    const type = async (value: string) => {
      const field = el.querySelector<HTMLInputElement>('input[name=name]')!;
      field.value = value;
      field.dispatchEvent(new Event('input'));
      await settle();
    };
    const plus = async (title: string) => {
      // A row not chosen yet has only "Escolher"; a chosen one has "Menos" and "Mais".
      (el.querySelector<HTMLButtonElement>(`button[aria-label="Mais ${title}"]`) ??
        el.querySelector<HTMLButtonElement>(`button[aria-label="Escolher ${title}"]`))!.click();
      await settle();
    };
    return { fixture, el, button, pick, type, plus, settle };
  }

  it('Convocar Familiar as a ritual: the title, "ritual · 1 hora", the name field, the forms the server lists and no slot picker', async () => {
    const { el } = await setup('spell:find-familiar');
    expect(flat(el.querySelector('.frame__title'))).toBe('Convocar Familiar');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Magia de 1º nível · ritual · 1 hora');
    expect(el.querySelector('app-slot-picker')).toBeNull();
    expect(flat(el.querySelector('.forms .cap'))).toBe('Forma · 3 do livro');
    expect(el.querySelectorAll('app-creature-choice-list input[type=radio]')).toHaveLength(3);
    expect(flat(el.querySelector('mat-hint'))).toBe('0 de 40');
  });

  it('names everything that is missing in one sentence, and the filled button stays dashed until it is ready', async () => {
    const { el, button, type, pick } = await setup('spell:find-familiar');
    expect(flat(el.querySelector('.line'))).toBe('Escolha a forma e dê um nome ao familiar.');
    expect(isOff(button('Convocar o familiar'))).toBe(true);
    expect(button('Convocar o familiar').classList).toContain('mr-button--off');
    await type('Nanquim');
    expect(flat(el.querySelector('.line'))).toBe('Escolha a forma.');
    await pick('Corvo');
    expect(flat(el.querySelector('.line'))).toBe(
      'Conjurar como ritual · 1 hora · sem gastar espaço',
    );
    expect(isOff(button('Convocar o familiar'))).toBe(false);
    expect(button('Convocar o familiar').classList).not.toContain('mr-button--off');
  });

  it('lists the forms as "Miúdo · 1 PV · 3 m, voo 15 m" and the search narrows them by name, ignoring accents', async () => {
    const { el, settle } = await setup('spell:find-familiar', [
      familiarSpell(
        Array.from(
          { length: 8 },
          (_, i) => [`monster:f${i}`, `Forma ${i}`] as [string, string],
        ).concat([['monster:raven', 'Corvo']]),
      ),
    ]);
    expect(
      flat(Array.from(el.querySelectorAll('.row')).find((r) => flat(r)?.includes('Corvo'))),
    ).toBe('Corvo Miúdo · 1 PV · 3 m, voo 15 m');
    const search = el.querySelector<HTMLInputElement>('input[type=search]')!;
    search.value = 'cor';
    search.dispatchEvent(new Event('input'));
    await settle();
    expect(Array.from(el.querySelectorAll('.row__title')).map((t) => flat(t))).toEqual(['Corvo']);
  });

  it('casts as a ritual with no slot, with the name and the form once, and closes with what to announce', async () => {
    const { button, pick, type, settle } = await setup('spell:find-familiar');
    await type('Nanquim');
    await pick('Corvo');
    button('Convocar o familiar').click();
    await settle();
    expect(api.casts).toHaveLength(1);
    expect(api.casts[0]).toMatchObject({
      campaignId: 'camp-1',
      characterId: 'char-1',
      spellKey: 'spell:find-familiar',
      ritual: true,
      slot: undefined,
      summon: { option: 0, creatureKeys: ['monster:raven'], names: ['Nanquim'] },
    });
    expect(close).toHaveBeenCalledWith({
      spellName: 'Convocar Familiar',
      ritual: true,
      castingTime: '1 hora',
      names: ['Nanquim'],
      count: 1,
      dismissed: 0,
    });
  });

  it('a refusal stays in the sheet, in words by its typed reason, and a retry keeps the key until a choice changes', async () => {
    const { el, button, pick, type, settle } = await setup('spell:find-familiar');
    await type('Nanquim');
    await pick('Corvo');
    api.failWith = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: EncounterBlockedSchema,
        value: create(EncounterBlockedSchema, { reason: EncounterBlockedReason.SUMMON_IN_COMBAT }),
      },
    ]);
    button('Convocar o familiar').click();
    await settle();
    expect(close).not.toHaveBeenCalled();
    expect(flat(el.querySelector('[role=alert]'))).toBe(
      'Há um combate em andamento. Conjure pela sua vez, na tela do combate.',
    );
    const first = api.casts[0].idempotencyKey;
    button('Convocar o familiar').click();
    await settle();
    expect(api.casts[1].idempotencyKey).toBe(first);
    await pick('Gato');
    button('Convocar o familiar').click();
    await settle();
    expect(api.casts[2].idempotencyKey).not.toBe(first);
  });

  it('a druid in a beast form is told why: in Wild Shape there is no casting', async () => {
    const { el, button, pick, type, settle } = await setup('spell:find-familiar');
    await type('Nanquim');
    await pick('Corvo');
    api.failWith = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: EncounterBlockedSchema,
        value: create(EncounterBlockedSchema, {
          reason: EncounterBlockedReason.WILD_SHAPE_NO_SPELLS,
        }),
      },
    ]);
    button('Convocar o familiar').click();
    await settle();
    expect(flat(el.querySelector('[role=alert]'))).toBe(
      'Na Forma Selvagem não dá para conjurar. Volte à forma normal e tente de novo.',
    );
  });

  it("warns before the cast that a new familiar takes the old one's place, and starts with its name", async () => {
    const old = create(ReplacedCreatureSchema, {
      id: 'cr-1',
      name: 'Nanquim',
      monsterKey: 'monster:raven',
      monsterNamePt: 'Corvo',
    });
    const { el } = await setup('spell:find-familiar', [
      familiarSpell(undefined, { replaces: [old] }),
    ]);
    expect(flat(el.querySelector('.mr-notice--warning'))).toBe(
      'Nanquim sai da ficha: um novo familiar toma o lugar.',
    );
    expect(el.querySelector<HTMLInputElement>('input[name=name]')!.value).toBe('Nanquim');
  });

  it('Animar Mortos: the slot picker lists the circles with slots (a pact slot too), the count follows the slot, kinds mix', async () => {
    const { el, button, plus, pick, settle } = await setup(
      'spell:animate-dead',
      [undeadSpell()],
      [
        [3, 2, 1],
        [5, 1, 1, true],
      ],
    );
    const rows = Array.from(el.querySelectorAll('app-slot-picker .row')).map((r) => flat(r));
    expect(rows).toHaveLength(2);
    expect(rows[0]).toContain('3º nível');
    expect(rows[0]).toContain('1 livre de 2');
    expect(rows[1]).toContain('5º nível (pacto)');
    // The first free circle is chosen already: the 3rd, with one undead, so a radio list.
    expect(flat(el.querySelector('.forms .cap'))).toBe('Criatura');
    await pick('Esqueleto');
    expect(flat(el.querySelector('.line'))).toBe('1 minuto · gasta um espaço de 3º nível');
    // A creature is chosen: the slot and the quantity fold into one line, with "Mudar" to open them again.
    expect(el.querySelector('app-slot-picker')).toBeNull();
    expect(flat(el.querySelector('.setup'))).toContain('3º nível');
    button('Mudar').click();
    await settle();
    // The pact slot: five undead, any mix of the two kinds.
    await pick('5º nível (pacto)');
    expect(flat(el.querySelector('.forms .cap'))).toBe('Criaturas · 0 de 5');
    expect(flat(el.querySelector('.line'))).toBe('Escolha mais 5 criaturas.');
    await plus('Esqueleto');
    await plus('Esqueleto');
    await plus('Zumbi');
    expect(flat(el.querySelector('.line'))).toBe('Escolha mais 2 criaturas.');
    await plus('Zumbi');
    await plus('Zumbi');
    expect(flat(el.querySelector('.line'))).toBe(
      '5 criaturas · 1 minuto · gasta um espaço de 5º nível (pacto)',
    );
    expect(isOff(button('Animar os mortos'))).toBe(false);
    // No more than the count: the "+" is off.
    expect(el.querySelector<HTMLButtonElement>('button[aria-label="Mais Zumbi"]')!.disabled).toBe(
      true,
    );
  });

  it('Animar Mortos casts the mix the person made, with the slot and its pact flag, one key per creature', async () => {
    const { button, plus, pick, settle } = await setup(
      'spell:animate-dead',
      [undeadSpell()],
      [
        [3, 2, 1],
        [5, 1, 1, true],
      ],
    );
    await pick('5º nível (pacto)');
    for (const k of ['Esqueleto', 'Esqueleto', 'Zumbi', 'Zumbi', 'Zumbi']) {
      await plus(k);
    }
    button('Animar os mortos').click();
    await settle();
    expect(api.casts[0]).toMatchObject({
      spellKey: 'spell:animate-dead',
      ritual: false,
      slot: { level: 5, pact: true },
    });
    expect(api.casts[0].summon.creatureKeys).toEqual([
      'monster:skeleton',
      'monster:skeleton',
      'monster:zombie',
      'monster:zombie',
      'monster:zombie',
    ]);
    expect(api.casts[0].summon.names).toEqual([]);
  });

  it("Conjurar Animais: the options come from the server, the beasts of the option's ND, warns what the concentration ends, any mix", async () => {
    const wolves = [
      create(ReplacedCreatureSchema, { id: 'w1', name: 'Lobo 1', monsterNamePt: 'Lobo' }),
      create(ReplacedCreatureSchema, { id: 'w2', name: 'Lobo 2', monsterNamePt: 'Lobo' }),
    ];
    const { el, button, pick, plus, settle } = await setup(
      'spell:conjure-animals',
      [beastSpell({ replaces: wolves })],
      [[3, 2, 2]],
    );
    expect(flat(el.querySelector('.mr-notice--warning'))).toBe(
      'Isso encerra Conjurar Animais e dispensa 2 criaturas: Lobo 1 e Lobo 2.',
    );
    const options = Array.from(el.querySelectorAll('.opt')).map((o) => flat(o));
    expect(options).toEqual([
      '1 fera de ND 2 ou menos',
      '2 feras de ND 1 ou menos',
      '4 feras de ND 1/2 ou menos',
      '8 feras de ND 1/4 ou menos',
    ]);
    expect(api.searches.at(-1)).toMatchObject({ type: 'beast', maxCr: '2' });
    await pick('4 feras');
    expect(api.searches.at(-1)).toMatchObject({ type: 'beast', maxCr: '1/2' });
    await plus('Urso negro');
    await plus('Urso negro');
    await plus('Lobo');
    await plus('Lobo');
    button('Conjurar Animais').click();
    await settle();
    expect(api.casts[0].summon.option).toBe(2);
    expect(api.casts[0].summon.creatureKeys).toEqual([
      'monster:wolf',
      'monster:wolf',
      'monster:bear',
      'monster:bear',
    ]);
  });

  it('the beasts of an option picked after another one never get replaced by a late answer', async () => {
    const { el, pick, settle } = await setup('spell:conjure-animals', [beastSpell()], [[3, 2, 2]]);
    let release!: () => void;
    const slow = new Promise<void>((r) => (release = r));
    api.search.mockImplementationOnce(async () => {
      await slow;
      return { creatures: [summary('monster:old', 'Antigo')], total: 1 };
    });
    await pick('2 feras');
    await pick('4 feras');
    release();
    await settle();
    expect(Array.from(el.querySelectorAll('.row__title')).map((t) => flat(t))).not.toContain(
      'Antigo',
    );
  });

  describe('the numbers of the forms of an option picked after another one', () => {
    const spell = () =>
      undeadSpell({
        circles: [
          {
            circle: 3,
            options: [
              {
                count: 1,
                forms: [{ monsterKey: 'monster:skeleton', namePt: 'Esqueleto', attack: 3 }],
              },
            ],
          },
          {
            circle: 5,
            options: [
              { count: 1, forms: [{ monsterKey: 'monster:zombie', namePt: 'Zumbi', attack: 3 }] },
            ],
          },
        ] as never,
      });
    const slots: [number, number, number, boolean?][] = [
      [3, 2, 1],
      [5, 1, 1],
    ];

    it('never get replaced by a late answer for the option left', async () => {
      const { el, pick, settle } = await setup('spell:animate-dead', [spell()], slots);
      const zombie = api.blocks.get('monster:zombie')!;
      let release!: () => void;
      const slow = new Promise<void>((r) => (release = r));
      api.statBlock.mockImplementationOnce(async () => {
        await slow;
        return zombie;
      });
      await pick('5º nível');
      await pick('3º nível');
      release();
      await settle();
      expect(flat(el.querySelector('.forms'))).toContain('Esqueleto');
      expect(flat(el.querySelector('.forms'))).toContain('PV');
    });

    it('shows the numbers of the option that is picked (positive control)', async () => {
      const { el, pick } = await setup('spell:animate-dead', [spell()], slots);
      await pick('5º nível');
      expect(flat(el.querySelector('.forms'))).toContain('Zumbi');
      expect(flat(el.querySelector('.forms'))).toContain('PV');
    });
  });

  it('a spell the sheet no longer has is said in words, with nothing to cast', async () => {
    const { el } = await setup('spell:conjure-animals', [familiarSpell()]);
    expect(flat(el.querySelector('[role=alert]'))).toBe(
      'A ficha não conjura mais essa magia. Feche esta folha e olhe a ficha.',
    );
  });

  it('Cancelar and the X close without casting', async () => {
    const { button, el } = await setup('spell:find-familiar');
    button('Cancelar').click();
    expect(close).toHaveBeenCalledWith(undefined);
    expect(api.casts).toHaveLength(0);
    expect(el.querySelector('.frame__close')?.getAttribute('aria-label')).toBe('Fechar');
  });
});
