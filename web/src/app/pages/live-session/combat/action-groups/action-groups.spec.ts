import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import { CombatantSchema } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
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

  it('gives each spell the "?" with its name, 44 px, whether or not it can be cast', () => {
    const { el } = setup(SERVER_ORDER);
    for (const row of rows(el)) {
      const help = row.querySelector<HTMLButtonElement>('app-spell-help button')!;
      expect(help.getAttribute('aria-label')).toBe(`Detalhes de ${name(row)}`);
      expect(help.disabled).toBe(false);
    }
  });

  it('opens the details with the spell key and name', () => {
    const { el, described } = setup(SERVER_ORDER);
    rows(el)[2].querySelector<HTMLButtonElement>('app-spell-help button')!.click();
    expect(described).toEqual([{ key: 'spell:sleep', name: 'Sono' }]);
    // A spell that is off has its "?" too.
    rows(el)[5].querySelector<HTMLButtonElement>('app-spell-help button')!.click();
    expect(described[1]).toEqual({ key: 'spell:web', name: 'Teia' });
  });

  it('puts the circle in a tag on its own line, and "Reação" next to it for Escudo', () => {
    const { el } = setup(SERVER_ORDER);
    const tags = (row: HTMLElement) =>
      [...row.querySelectorAll('.row__tags .row__pill')].map((t) => t.textContent!.trim());
    expect(tags(rows(el)[0])).toEqual(['Truque']);
    expect(tags(rows(el)[2])).toEqual(['1º nível']);
    expect(tags(rows(el)[3])).toEqual(['1º nível', 'Reação']);
    expect(el.querySelector('.row__name .row__pill')).toBeNull();
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
    const slotRows = [...el.querySelectorAll('.slots__row')].map((r) => [
      r.querySelector('.slots__title')!.textContent!.trim(),
      r.querySelector('.slots__text')!.textContent!.trim(),
    ]);
    expect(slotRows).toEqual([
      ['1º\u00a0nível', '0 livres de 4'],
      ['2º\u00a0nível', '0 livres de 2'],
    ]);
  });

  it('casts a spell that can be cast, and not one that cannot', () => {
    const { el, cast } = setup(SERVER_ORDER);
    rows(el)[2].querySelector<HTMLButtonElement>('button.row__btn')!.click();
    rows(el)[5].querySelector<HTMLButtonElement>('button.row__btn')!.click();
    expect(cast).toEqual(['spell:sleep']);
  });
});

describe('ActionGroups: a move that waits for an opportunity attack (E9-13)', () => {
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

  it('says the movement in tenths of a foot, metres with one decimal', () => {
    const el = setup({
      speedDft: 300,
      movementLeftDft: 229,
      movementLeftFt: 22,
      movementUsedDft: 71,
      movementUsedFt: 7,
    });
    expect(plain(el.textContent)).toContain('Restam 6,9 m');
    expect(plain(el.textContent)).toContain(
      'Você já andou 2,1 m. Dá para andar mais 6,9 m (4 quadrados).',
    );
  });

  it('turns "Mover" off with the reason instead of failing on click', () => {
    const el = setup(
      { speedDft: 300, movementLeftDft: 150, movementLeftFt: 15 },
      'Esperando a reação do mestre',
    );
    const move = Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find(
      (b) => plain(b.textContent) === 'Mover',
    )!;
    expect(move.getAttribute('aria-disabled') === 'true' || move.disabled).toBe(true);
    expect(plain(el.textContent)).toContain('Esperando a reação do mestre');
  });

  it('reads Desengajar as such: a notice, "Desengajado" and the move that does not provoke', () => {
    const el = setup({
      speedDft: 300,
      movementLeftDft: 300,
      movementLeftFt: 30,
      disengaged: true,
      actionUsed: true,
    });
    const text = plain(el.textContent);
    expect(text).toContain(
      'Você usou Desengajar. Seus movimentos deste turno não provocam ataque de oportunidade.',
    );
    expect(text).toContain('Desengajado');
    expect(text).toContain('Dá para andar até 9,0 m (6 quadrados) sem provocar.');
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
    expect(row?.querySelector('.row__pill')?.textContent?.trim()).toBe('Ação');
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
      create(TurnOptionsSchema, { featureActions: [wildFeature() as never] }),
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
