import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  CombatantSchema,
  ManeuverDieOptionSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  ActionOptionSchema,
  AttackKind,
  AttackOptionSchema,
  BonusAttackRule,
  DisabledReasonCode,
  SpellOptionSchema,
  TurnOptionsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { ActionGroups } from './action-groups';

/** A spell option as `GetTurnOptions` sends it (name, circle, economy, and why it is off). */
function spell(
  key: string,
  namePt: string,
  level: number,
  over: { enabled?: boolean; reason?: DisabledReasonCode; economy?: ActionEconomy } = {},
) {
  const enabled = over.enabled ?? true;
  return create(SpellOptionSchema, {
    spell: { key, name: namePt, namePt, level, schoolNamePt: 'Encantamento' },
    economy: over.economy ?? ActionEconomy.ACTION,
    enabled,
    reason: enabled
      ? undefined
      : { code: over.reason ?? DisabledReasonCode.NO_SLOT, minLevel: level },
  });
}

describe('ActionGroups: the spells (E8-02)', () => {
  function setup(
    spells: ReturnType<typeof spell>[],
    slots = { usage: [] as { level: number; total: number; used: number }[], pact: null },
  ) {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput('options', create(TurnOptionsSchema, { spells }));
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, { movementLeftFt: 25, speedFt: 25 }),
    );
    fixture.componentRef.setInput('slots', slots);
    const described: { key: string; name: string }[] = [];
    const cast: string[] = [];
    fixture.componentInstance.describe.subscribe((d) => described.push(d));
    fixture.componentInstance.cast.subscribe((k) => cast.push(k));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    return { el, described, cast };
  }

  // The order the server sends: what can be cast now first, then by circle and name.
  const SERVER_ORDER = [
    spell('spell:minor-illusion', 'Ilusão Menor', 0),
    spell('spell:mage-armor', 'Armadura Arcana', 1),
    spell('spell:sleep', 'Sono', 1),
    spell('spell:shield', 'Escudo Arcano', 1, {
      enabled: false,
      reason: DisabledReasonCode.REACTION_ONLY_WHEN_HIT,
      economy: ActionEconomy.REACTION,
    }),
    spell('spell:scorching-ray', 'Raio Ardente', 2, { enabled: false }),
    spell('spell:web', 'Teia', 2, { enabled: false }),
  ];

  // The spell rows: the Movimento row has no name.
  const rows = (el: HTMLElement) =>
    ([...el.querySelectorAll('app-action-row')] as HTMLElement[]).filter((r) =>
      r.querySelector('.row__name'),
    );
  const name = (row: HTMLElement) => row.querySelector('.row__name')!.textContent!.trim();

  it('lists the spells in the order the server sent, every economy in one list', () => {
    const { el } = setup(SERVER_ORDER);
    expect(rows(el).map(name)).toEqual([
      'Ilusão Menor',
      'Armadura Arcana',
      'Sono',
      'Escudo Arcano',
      'Raio Ardente',
      'Teia',
    ]);
  });

  it('gives a spell row no "?": the details are the cast sheet\'s own "?"', () => {
    const { el } = setup(SERVER_ORDER);
    expect(el.querySelector('app-spell-help')).toBeNull();
  });

  it('puts the circle first in the detail line, and the economy beside the name for Escudo', () => {
    const { el } = setup(SERVER_ORDER);
    const detail = (row: HTMLElement) => row.querySelector('.row__detail')!.textContent!.trim();
    expect(detail(rows(el)[0])).toMatch(/^Truque\s·\s./);
    expect(detail(rows(el)[2])).toMatch(/^1º\snível\s·\s./);
    // The economy is the row's own mark and word, not a tag.
    expect(detail(rows(el)[3])).toMatch(/^1º\snível\s·\s./);
    expect(rows(el)[3].querySelector('.row__econ')?.textContent?.trim()).toBe('Reação');
    expect(rows(el)[1].querySelector('.row__econ')?.textContent?.trim()).toBe('Ação');
    expect(el.querySelector('.row__pill')).toBeNull();
    expect(rows(el)[2].querySelector('.row__btn mat-icon')?.textContent?.trim()).toBe(
      'auto_fix_high',
    );
  });

  it('Escudo Arcano on the own turn keeps a dashed, disabled "Conjurar" and says "Só fora da sua vez"', () => {
    const { el, cast } = setup(SERVER_ORDER);
    const shield = rows(el)[3];
    const button = shield.querySelector<HTMLButtonElement>('button.row__btn')!;
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button.classList).toContain('row__btn--off');
    button.click();
    expect(cast).toEqual([]);
    expect(shield.querySelector('.row__why')!.textContent).toContain('Só fora da sua vez');
  });

  it('repeats only "Sem espaço" on a spell without a slot, and the slot rows above are the explanation', () => {
    const { el } = setup(SERVER_ORDER, {
      usage: [
        { level: 1, total: 4, used: 4 },
        { level: 2, total: 2, used: 2 },
      ],
      pact: null,
    });
    const web = rows(el)[5];
    expect(web.querySelector('.row__why')!.textContent!.trim()).toContain('Sem espaço');
    expect(web.querySelector('.row__why')!.textContent).not.toContain('nível');
    const slotRows = [...el.querySelectorAll('.slots__row')].map((r) =>
      r.getAttribute('aria-label'),
    );
    expect(slotRows).toEqual(['1º\u00a0nível: 0 livres de 4', '2º\u00a0nível: 0 livres de 2']);
    expect(el.querySelector('.slots__lead')!.textContent).toBe('Espaços:');
  });

  it('casts a spell that can be cast, and not one that cannot', () => {
    const { el, cast } = setup(SERVER_ORDER);
    rows(el)[2].querySelector<HTMLButtonElement>('button.row__btn')!.click();
    rows(el)[5].querySelector<HTMLButtonElement>('button.row__btn')!.click();
    expect(cast).toEqual(['spell:sleep']);
  });
});

describe('ActionGroups: the movement belongs to the row under the map (E9-13)', () => {
  const plain = (t: string | null | undefined) =>
    (t ?? '')
      .replace(/\u00a0/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();

  function setup(own: Parameters<typeof create<typeof CombatantSchema>>[1], locked = '') {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput('options', create(TurnOptionsSchema, {}));
    fixture.componentRef.setInput('own', create(CombatantSchema, own));
    fixture.componentRef.setInput('locked', locked);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('has no "Mover": the movement row under the map has the one button', () => {
    const el = setup({ speedDft: 300, movementLeftDft: 150, movementLeftFt: 15 });
    expect([...el.querySelectorAll('button')].map((b) => plain(b.textContent))).not.toContain(
      'Mover',
    );
    expect(plain(el.textContent)).not.toContain('Restam');
  });

  it('has its live region on the page when no note is shown', () => {
    const el = setup({ speedDft: 300, movementLeftDft: 300, movementLeftFt: 30 });
    const region = el.querySelector('.mr-live[role="status"]');
    expect(region).not.toBeNull();
    expect(region?.querySelector('.groups__note')).toBeNull();
  });

  it('says Desengajar in a notice, once', () => {
    const el = setup({
      speedDft: 300,
      movementLeftDft: 300,
      movementLeftFt: 30,
      disengaged: true,
      actionUsed: true,
    });
    expect(plain(el.textContent)).toContain(
      'Você usou Desengajar. Seus movimentos deste turno não provocam ataque de oportunidade.',
    );
    const note = el.querySelector('.groups__note');
    expect(note?.closest('.mr-live[role="status"]')).not.toBeNull();
    expect(note?.hasAttribute('role')).toBe(false);
  });
});

describe('ActionGroups: "Ver pelos olhos do Nanquim" (MR-036, E9-04)', () => {
  function setup(familiar: string | null, own: Partial<{ actionUsed: boolean }> = {}) {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput('options', create(TurnOptionsSchema, {}));
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, { movementLeftFt: 25, speedFt: 25, ...own }),
    );
    fixture.componentRef.setInput('familiar', familiar);
    const asked: number[] = [];
    fixture.componentInstance.familiarEyes.subscribe(() => asked.push(1));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    // With the Action spent the standard actions (and this row) are one folded line: open it.
    el.querySelectorAll<HTMLButtonElement>('.mr-fold__button').forEach((b) => b.click());
    fixture.detectChanges();
    const row = [...el.querySelectorAll('app-action-row')].find((r) =>
      r.textContent?.includes('Ver pelos olhos'),
    );
    return { el, row: row as HTMLElement | undefined, asked };
  }

  it('is an Ação row with its tag and what it lasts, and asks the question when pressed', () => {
    const { row, asked } = setup('Nanquim');
    expect(row?.querySelector('.row__name')?.textContent?.trim()).toBe(
      'Ver pelos olhos do Nanquim',
    );
    expect(row?.querySelector('.row__econ')?.textContent?.trim()).toBe('Ação');
    expect(row?.querySelector('.row__detail')?.textContent?.trim()).toBe(
      'Dura até o começo da sua próxima vez, ou até você voltar.',
    );
    row!.querySelector<HTMLButtonElement>('button')!.click();
    expect(asked).toEqual([1]);
  });

  it('is off, with the reason, once the action is spent', () => {
    const { row, asked } = setup('Nanquim', { actionUsed: true });
    expect(row?.querySelector('.row__why')?.textContent).toContain(
      'Você já usou a sua ação neste turno.',
    );
    row!.querySelector<HTMLButtonElement>('button')!.click();
    expect(asked).toEqual([]);
  });

  it('is not there without a familiar, or while the player already looks through its eyes', () => {
    expect(setup(null).row).toBeUndefined();
  });
});

describe('ActionGroups: Wild Shape (MR-037, E9-11)', () => {
  const wildFeature = () => ({
    action: {
      key: 'feature:wild-shape',
      namePt: 'Forma Selvagem',
      economy: ActionEconomy.ACTION,
      resourceKey: 'wild_shape',
    },
    enabled: true,
    usesLeft: 2,
  });

  function setup(
    over: { wild?: { key: string; detail: string; reason: string } | null; beast?: string } = {},
  ) {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput(
      'options',
      create(TurnOptionsSchema, {
        featureActions: [wildFeature() as never],
        spells: [spell('spell:sleep', 'Sono', 1)],
      }),
    );
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, { movementLeftFt: 25, speedFt: 25 }),
    );
    fixture.componentRef.setInput(
      'wild',
      over.wild === undefined
        ? { key: 'feature:wild-shape', detail: 'Vire uma fera · restam 2 de 2 usos', reason: '' }
        : over.wild,
    );
    fixture.componentRef.setInput('beast', over.beast ?? '');
    const events: string[] = [];
    fixture.componentInstance.transform.subscribe(() => events.push('transform'));
    fixture.componentInstance.leaveForm.subscribe(() => events.push('leave'));
    fixture.detectChanges();
    return { el: fixture.nativeElement as HTMLElement, events };
  }
  const text = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();

  it('is a line of the Ação with "Transformar" and what it costs', () => {
    const { el, events } = setup();
    const row = Array.from(el.querySelectorAll('app-action-row')).find((r) =>
      text(r)?.includes('Forma Selvagem'),
    )!;
    expect(text(row)).toContain('Característica');
    expect(text(row)).toContain('Vire uma fera · restam 2 de 2 usos');
    row.querySelector('button')!.click();
    expect(events).toEqual(['transform']);
    expect(row.querySelector('button')?.getAttribute('aria-label')).toBe(
      'Transformar: Forma Selvagem',
    );
  });

  it('with no uses the button is off and the line says why', () => {
    const { el } = setup({
      wild: { key: 'feature:wild-shape', detail: 'Vire uma fera', reason: 'Sem usos' },
    });
    const row = Array.from(el.querySelectorAll('app-action-row')).find((r) =>
      text(r)?.includes('Forma Selvagem'),
    )!;
    expect(row.querySelector('button')?.getAttribute('aria-disabled')).toBe('true');
    expect(text(row)).toContain('Sem usos');
  });

  it('as a beast: no spells, no Transformar, and "Voltar à forma normal" is a bonus action', () => {
    const { el, events } = setup({ wild: null, beast: 'Lobo' });
    expect(text(el)).toContain('Sem magias na forma de fera. Volte à forma normal para conjurar.');
    expect(text(el)).not.toContain('Transformar');
    const back = Array.from(el.querySelectorAll('app-action-row')).find((r) =>
      text(r)?.includes('Voltar à forma normal'),
    )!;
    back.querySelector('button')!.click();
    expect(events).toEqual(['leave']);
  });
});

describe('ActionGroups: the movement without a map (RN-25, E10-04 state 7)', () => {
  function setup(left: number, theatre: boolean) {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput('options', create(TurnOptionsSchema, {}));
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, {
        movementLeftFt: left,
        movementLeftDft: left * 10,
        speedFt: 30,
        speedDft: 300,
      }),
    );
    fixture.componentRef.setInput('theatre', theatre);
    const moves: number[] = [];
    fixture.componentInstance.move.subscribe(() => moves.push(1));
    fixture.detectChanges();
    return { el: fixture.nativeElement as HTMLElement, moves };
  }
  const text = (el: HTMLElement) => (el.textContent ?? '').replace(/\s+/g, ' ');

  it('has one phrase and "Gastar movimento" instead of "Mover"', () => {
    const { el, moves } = setup(30, true);
    expect(text(el)).toContain(
      'Sem mapa, você diz quanto andou. O mestre decide se o caminho está livre.',
    );
    const buttons = [...el.querySelectorAll('button')].map((b) => (b.textContent ?? '').trim());
    expect(buttons.some((b) => b.includes('Gastar movimento'))).toBe(true);
    expect(buttons).not.toContain('Mover');
    (el.querySelector('.spend') as HTMLButtonElement).click();
    expect(moves.length).toBe(1);
  });

  it('is dashed with its reason once the movement is spent, and presses nothing', () => {
    const { el, moves } = setup(0, true);
    const spend = el.querySelector('.spend') as HTMLButtonElement;
    expect(spend.getAttribute('aria-disabled')).toBe('true');
    expect(text(el)).toContain('Você já gastou todo o movimento deste turno.');
    spend.click();
    expect(moves.length).toBe(0);
  });

  it('keeps "Mover" on a map', () => {
    const { el } = setup(30, false);
    expect(el.querySelector('.spend')).toBeNull();
    expect(text(el)).not.toContain('Gastar movimento');
  });
});

interface AttackOver {
  enabled?: boolean;
  reason?: { code: DisabledReasonCode };
  bonusRule?: BonusAttackRule;
  bonusAttacksLeft?: number;
  bonusDropsModifier?: boolean;
  beamsLeft?: number;
}

describe('ActionGroups: the bonus action attacks', () => {
  const attack = (key: string, namePt: string, over: AttackOver = {}) =>
    create(AttackOptionSchema, {
      attack: {
        key,
        name: namePt,
        namePt,
        attackBonus: 5,
        damage: '1d4',
        damageTypePt: 'perfurante',
        rangeFt: 5,
      },
      enabled: true,
      ...over,
    });

  function setup(attacks: ReturnType<typeof attack>[], attacksLeft = 0, attacksPerAction = 1) {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput('options', create(TurnOptionsSchema, { attacks }));
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, { movementLeftFt: 25, speedFt: 25, actionUsed: true }),
    );
    fixture.componentRef.setInput('attacksLeft', attacksLeft);
    fixture.componentRef.setInput('attacksPerAction', attacksPerAction);
    const attacked: string[] = [];
    fixture.componentInstance.attack.subscribe((k) => attacked.push(k));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    // The Action is spent: its attacks are one folded line; open it to read them.
    el.querySelectorAll<HTMLButtonElement>('.mr-fold__button').forEach((b) => b.click());
    fixture.detectChanges();
    const row = (name: string) =>
      ([...el.querySelectorAll('app-action-row')] as HTMLElement[]).find(
        (r) => r.querySelector('.row__name')?.textContent?.trim() === name,
      )!;
    return { row, attacked };
  }
  const tags = (row: HTMLElement) =>
    [...row.querySelectorAll('.row__pill')].map((t) => t.textContent!.trim());
  const note = (row: HTMLElement) => row.querySelector('.row__note')?.textContent?.trim() ?? '';
  const button = (row: HTMLElement) => row.querySelector<HTMLButtonElement>('button.row__btn')!;

  it('offers the off-hand attack with the action spent: the tag, the line and a working button', () => {
    const { row, attacked } = setup([
      attack('equipment:dagger', 'Adaga', {
        bonusRule: BonusAttackRule.OFF_HAND,
        bonusDropsModifier: true,
      }),
      attack('equipment:longsword', 'Espada longa', {
        enabled: false,
        reason: { code: DisabledReasonCode.ACTION_USED },
      }),
    ]);
    const dagger = row('Adaga');
    expect(tags(dagger)).toEqual([]);
    expect(dagger.querySelector('.row__econ')?.textContent?.trim()).toBe('Bônus');
    expect(note(dagger)).toBe('Ataque com a outra mão, sem o modificador no dano');
    expect(button(dagger).getAttribute('aria-disabled')).not.toBe('true');
    button(dagger).click();
    expect(attacked).toEqual(['equipment:dagger']);
    expect(tags(row('Espada longa'))).toEqual([]);
    expect(button(row('Espada longa')).getAttribute('aria-disabled')).toBe('true');
  });

  it('offers the Martial Arts strike, and Rajada de Golpes counting down', () => {
    const martial = setup([
      attack('attack:unarmed-strike', 'Golpe desarmado', {
        bonusRule: BonusAttackRule.MARTIAL_ARTS,
      }),
    ]);
    expect(note(martial.row('Golpe desarmado'))).toBe('Golpe desarmado das Artes Marciais');
    for (const [left, text] of [
      [2, 'Rajada de Golpes: 2 golpes restantes'],
      [1, 'Rajada de Golpes: 1 golpe restante'],
    ] as const) {
      const flurry = setup([
        attack('attack:unarmed-strike', 'Golpe desarmado', {
          bonusRule: BonusAttackRule.FLURRY_OF_BLOWS,
          bonusAttacksLeft: left,
        }),
      ]);
      expect(tags(flurry.row('Golpe desarmado'))).toEqual([]);
      expect(note(flurry.row('Golpe desarmado'))).toBe(text);
    }
  });

  it('keeps the attacks of the Attack action first: no tag while Extra Attack has an attack left', () => {
    const { row } = setup([attack('equipment:dagger', 'Adaga')], 1, 2);
    expect(tags(row('Adaga'))).toEqual([]);
    expect(note(row('Adaga'))).toBe('');
  });

  it('turns the bonus action attack off with the bonus action reason once it is spent', () => {
    const { row, attacked } = setup([
      attack('equipment:dagger', 'Adaga', {
        bonusRule: BonusAttackRule.OFF_HAND,
        enabled: false,
        reason: { code: DisabledReasonCode.BONUS_ACTION_USED },
      }),
    ]);
    const dagger = row('Adaga');
    expect(tags(dagger)).toEqual([]);
    expect(dagger.querySelector('.row__econ')?.textContent?.trim()).toBe('Bônus');
    expect(note(dagger)).toBe('');
    expect(dagger.querySelector('.row__why')!.textContent).toContain('Ação bônus já usada');
    button(dagger).click();
    expect(attacked).toEqual([]);
  });

  it('offers the next beam of Eldritch Blast with the action spent, counting down', () => {
    for (const [left, text] of [
      [2, 'Rajada Mística: 2 raios restantes'],
      [1, 'Rajada Mística: 1 raio restante'],
    ] as const) {
      const { row, attacked } = setup([
        attack('spell:eldritch-blast', 'Rajada Mística', { beamsLeft: left }),
        attack('equipment:dagger', 'Adaga', {
          enabled: false,
          reason: { code: DisabledReasonCode.ACTION_USED },
        }),
      ]);
      const blast = row('Rajada Mística');
      expect(note(blast)).toBe(text);
      expect(button(blast).getAttribute('aria-disabled')).not.toBe('true');
      button(blast).click();
      expect(attacked).toEqual(['spell:eldritch-blast']);
      expect(button(row('Adaga')).getAttribute('aria-disabled')).toBe('true');
    }
  });

  it('shows Rajada de Golpes off with its reason before the Attack action', () => {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput(
      'options',
      create(TurnOptionsSchema, {
        featureActions: [
          create(ActionOptionSchema, {
            action: {
              key: 'feature:flurry-of-blows',
              namePt: 'Rajada de Golpes',
              economy: ActionEconomy.BONUS_ACTION,
            },
            enabled: false,
            reason: { code: DisabledReasonCode.ATTACK_ACTION_FIRST },
          }),
        ],
      }),
    );
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, { movementLeftFt: 25, speedFt: 25 }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const flurry = ([...el.querySelectorAll('app-action-row')] as HTMLElement[]).find(
      (r) => r.querySelector('.row__name')?.textContent?.trim() === 'Rajada de Golpes',
    )!;
    expect(flurry.querySelector('.row__why')!.textContent).toContain(
      'Só depois de atacar com a ação',
    );
    expect(button(flurry).getAttribute('aria-disabled')).toBe('true');
  });
});

describe('ActionGroups: Fúria of a Berserker asks about the frenzy', () => {
  function setup(offersFrenzy: boolean) {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput(
      'options',
      create(TurnOptionsSchema, {
        featureActions: [
          create(ActionOptionSchema, {
            action: { key: 'feature:rage', namePt: 'Fúria', economy: ActionEconomy.BONUS_ACTION },
            enabled: true,
            offersFrenzy,
          }),
        ],
      }),
    );
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, { movementLeftFt: 25, speedFt: 25 }),
    );
    const plain: string[] = [];
    const rages: { key: string; frenzy: boolean }[] = [];
    fixture.componentInstance.feature.subscribe((k) => plain.push(k));
    fixture.componentInstance.rage.subscribe((r) => rages.push(r));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const tap = () => {
      el.querySelector<HTMLButtonElement>('button[aria-label="Usar Fúria"]')!.click();
      fixture.detectChanges();
    };
    const question = () => el.querySelector('.frenzy');
    const answer = (label: string) => {
      const b = [...el.querySelectorAll<HTMLButtonElement>('.frenzy button')].find(
        (x) => x.textContent?.trim() === label,
      )!;
      b.click();
      fixture.detectChanges();
    };
    return { el, plain, rages, tap, question, answer };
  }

  it('asks in place, with the two answers, before the rage starts', () => {
    const { plain, rages, tap, question } = setup(true);
    expect(question()).toBeNull();
    tap();
    expect(question()?.textContent).toContain('Entrar em frenesi?');
    expect(question()?.textContent).toContain('1 nível de exaustão');
    expect([...question()!.querySelectorAll('button')].map((b) => b.textContent?.trim())).toEqual([
      'Fúria com frenesi',
      'Só fúria',
    ]);
    expect(plain).toEqual([]);
    expect(rages).toEqual([]);
  });

  it('"Fúria com frenesi" rages in a frenzy and closes the question', () => {
    const { plain, rages, tap, question, answer } = setup(true);
    tap();
    answer('Fúria com frenesi');
    expect(rages).toEqual([{ key: 'feature:rage', frenzy: true }]);
    expect(plain).toEqual([]);
    expect(question()).toBeNull();
  });

  it('"Só fúria" rages as before, without the frenzy', () => {
    const { rages, tap, question, answer } = setup(true);
    tap();
    answer('Só fúria');
    expect(rages).toEqual([{ key: 'feature:rage', frenzy: false }]);
    expect(question()).toBeNull();
  });

  it('asks nothing of a barbarian without the frenzy: the tap rages at once', () => {
    const { plain, rages, tap, question } = setup(false);
    tap();
    expect(question()).toBeNull();
    expect(plain).toEqual(['feature:rage']);
    expect(rages).toEqual([]);
  });
});

describe('ActionGroups: the table die line above the weapon rows', () => {
  const die = (name: string, usesLeft: number, resourceNamePt = 'Reserva de Golpes') =>
    create(ManeuverDieOptionSchema, {
      die: { key: name, namePt: name, applies: 'attack', sides: 8 },
      resourceNamePt,
      usesLeft,
    });
  const weapon = (kind = AttackKind.WEAPON) =>
    create(AttackOptionSchema, {
      attack: { key: 'attack:sword', name: 'Sword', namePt: 'Espada longa', kind },
      enabled: true,
    });

  function setup(attacks: ReturnType<typeof weapon>[], dice: ReturnType<typeof die>[]) {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput('options', create(TurnOptionsSchema, { attacks }));
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, { movementLeftFt: 25, speedFt: 25 }),
    );
    fixture.componentRef.setInput('maneuverDice', dice);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }
  const note = (el: HTMLElement) =>
    (el.querySelector('[data-testid="maneuver-note"]')?.textContent ?? '')
      .replace(/\s+/g, ' ')
      .trim();

  it('shows one line with the die and the uses left', () => {
    const el = setup([weapon()], [die('Golpe Certeiro', 2)]);
    expect(el.querySelectorAll('[data-testid="maneuver-note"]').length).toBe(1);
    expect(note(el)).toContain(
      'Golpe Certeiro (d8) pode entrar num ataque com arma: você marca ao rolar. Reserva de Golpes: restam 2.',
    );
    expect(
      el.querySelector('[data-testid="maneuver-note"] mat-icon')?.getAttribute('aria-hidden'),
    ).toBe('true');
    expect(el.querySelector('[data-testid="maneuver-note"] button')).toBeNull();
  });

  it('names two dice in the plural', () => {
    const el = setup([weapon()], [die('Golpe Certeiro', 2), die('Ataque Firme', 2)]);
    expect(note(el)).toContain(
      'Golpe Certeiro (d8) e Ataque Firme (d8) podem entrar num ataque com arma',
    );
  });

  it('says "resta 1" in the singular for one use', () => {
    expect(note(setup([weapon()], [die('Golpe Certeiro', 1)]))).toContain(
      'Reserva de Golpes: resta 1.',
    );
  });

  it('leaves out the pool prefix when the pool has no name', () => {
    const text = note(setup([weapon()], [die('Golpe Certeiro', 2, '')]));
    expect(text).toContain('você marca ao rolar. Restam 2.');
    expect(text).not.toContain(': Restam');
  });

  it('says "sem usos" at zero and claims no recharge', () => {
    const el = setup([weapon()], [die('Golpe Certeiro', 0)]);
    expect(note(el)).toContain('Reserva de Golpes: sem usos.');
    expect(note(el)).not.toMatch(/descanso/);
  });

  it('is absent without dice (the master, an NPC)', () => {
    expect(note(setup([weapon()], []))).toBe('');
  });

  it('is absent when no weapon attack row exists (a spell-only list)', () => {
    expect(note(setup([weapon(AttackKind.SPELL)], [die('Golpe Certeiro', 2)]))).toBe('');
    expect(note(setup([], [die('Golpe Certeiro', 2)]))).toBe('');
  });
});

describe('ActionGroups: the marks of a weapon are a chip beside its name (#585)', () => {
  const attack = (over: { silvered?: boolean; magical?: boolean; kind?: AttackKind }) =>
    create(AttackOptionSchema, {
      attack: {
        key: 'equipment:longsword',
        name: 'Longsword',
        namePt: 'Espada longa',
        kind: over.kind ?? AttackKind.WEAPON,
        silvered: over.silvered ?? false,
        magical: over.magical ?? false,
        attackBonus: 6,
        damage: '1d8 + 3',
      },
      enabled: true,
    });
  function row(opt: ReturnType<typeof attack>): HTMLElement {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput('options', create(TurnOptionsSchema, { attacks: [opt] }));
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, { movementLeftFt: 25, speedFt: 25 }),
    );
    fixture.detectChanges();
    return (fixture.nativeElement as HTMLElement).querySelector('app-action-row')!;
  }

  it('puts an icon and a word for each mark beside the name, and keeps them out of the detail line', () => {
    const r = row(attack({ silvered: true, magical: true }));
    const head = r.querySelector('.row__head')!;
    expect(
      Array.from(head.querySelectorAll('.weapon-tags li')).map((li) => li.textContent!.trim()),
    ).toEqual(['dark_modePrateada', 'auto_awesomeMágica']);
    expect(head.querySelector('.row__name')!.textContent).toContain('Espada longa');
    expect(r.querySelector('.row__detail')!.textContent).not.toMatch(/Prateada|Mágica/);
  });

  it('shows no chip on a plain weapon or on a spell', () => {
    expect(row(attack({})).querySelector('.weapon-tags')).toBeNull();
    expect(
      row(attack({ kind: AttackKind.SPELL, magical: true })).querySelector('.weapon-tags'),
    ).toBeNull();
  });
});
