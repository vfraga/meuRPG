import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  CombatantKind,
  GetMoveOptionsResponseSchema,
  ReachableSquareSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  MountChoiceSchema,
  MountOptionsSchema,
  MountRefusal,
  MountSquareSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { MovePage } from './move-page';

/** What a person reads of a node: the glyphs of the icon font are not words. */
const plain = (node: Element | null | undefined): string => {
  const clone = node?.cloneNode(true) as Element | undefined;
  clone?.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return (clone?.textContent ?? '')
    .replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
};

const tor = (over: Partial<Parameters<typeof combatant>[0]> = {}) =>
  combatant({
    id: 'tor',
    label: 'Tor',
    kind: CombatantKind.PLAYER,
    mine: true,
    col: 5,
    row: 5,
    speedDft: 300,
    movementLeftDft: 300,
    ...over,
  });
const horse = (over: Partial<Parameters<typeof combatant>[0]> = {}) =>
  combatant({
    id: 'cav',
    label: 'Cavalo de guerra',
    col: 5,
    row: 5,
    speedDft: 600,
    movementLeftDft: 600,
    ...over,
  });
const mastiff = combatant({ id: 'mas', label: 'Mastim', col: 6, row: 5 });

const walk = create(GetMoveOptionsResponseSchema, {
  movementLeftDft: 600,
  reachable: [create(ReachableSquareSchema, { col: 8, row: 5, costDft: 150, leftAfterDft: 450 })],
});

function setup(
  over: {
    combatants: ReturnType<typeof combatant>[];
    mounts?: ReturnType<typeof create<typeof MountOptionsSchema>>;
    blocked?: boolean;
    busy?: boolean;
    readOnly?: boolean;
    master?: boolean;
  } = { combatants: [tor(), mastiff] },
) {
  const fixture = TestBed.createComponent(MovePage);
  const ref = fixture.componentRef;
  ref.setInput(
    'encounter',
    encounter({ combatants: over.combatants, currentCombatantId: 'tor', turnGroupIds: ['tor'] }),
  );
  ref.setInput('image', { url: '/images/x', width: 2000, height: 1400 });
  ref.setInput('sessionNumber', 6);
  ref.setInput('options', walk);
  ref.setInput('mounts', over.mounts);
  ref.setInput('dismountBlocked', over.blocked ?? false);
  ref.setInput('busy', over.busy ?? false);
  ref.setInput('readOnly', over.readOnly ?? false);
  if (over.master) {
    ref.setInput('isMaster', true);
    ref.setInput('moverId', 'tor');
  }
  const mounted: string[] = [];
  const landed: unknown[] = [];
  const moved: unknown[] = [];
  fixture.componentInstance.mountOn.subscribe((id) => mounted.push(id));
  fixture.componentInstance.dismountTo.subscribe((sq) => landed.push(sq));
  fixture.componentInstance.confirm.subscribe((sq) => moved.push(sq));
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const press = (name: string) => {
    Array.from(el.querySelectorAll('button'))
      .find((b) => plain(b).includes(name))!
      .click();
    fixture.detectChanges();
  };
  const choose = (col: number, row: number) => {
    const surface = el.querySelector<HTMLElement>('.cm__surface')!;
    surface.getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        width: 200,
        height: 140,
        right: 200,
        bottom: 140,
        x: 0,
        y: 0,
      }) as DOMRect;
    surface.dispatchEvent(
      new MouseEvent('click', { clientX: col * 10 + 5, clientY: row * 10 + 5, bubbles: true }),
    );
    fixture.detectChanges();
  };
  const pick = (label: string) => {
    const input = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')).find(
      (i) => plain(i.closest('label')).includes(label),
    )!;
    input.click();
    fixture.detectChanges();
  };
  return { fixture, el, press, choose, pick, mounted, landed, moved };
}

const choices = create(MountOptionsSchema, {
  mounts: [
    create(MountChoiceSchema, { combatantId: 'mas', costDft: 125 }),
    create(MountChoiceSchema, { combatantId: 'cav', costDft: 125, refusal: MountRefusal.TAKEN }),
  ],
});

describe('MovePage, on foot', () => {
  it('says nothing about a mount on a plain page, for a character and for a creature', () => {
    expect(setup({ combatants: [tor(), mastiff] }).el.querySelector('.move__why')).toBeNull();
    const creature = combatant({
      id: 'cr',
      label: 'Lobo',
      kind: CombatantKind.CREATURE,
      controlledByMe: true,
      col: 5,
      row: 5,
    });
    const page = setup({ combatants: [tor(), creature] });
    page.fixture.componentRef.setInput('moverId', 'cr');
    page.fixture.detectChanges();
    expect(page.el.querySelector('.move__why')).toBeNull();
    expect(
      page.el.querySelector('[role="radiogroup"][aria-label="Como se mover montado"]'),
    ).toBeNull();
  });
});

describe('MovePage, the "Montaria" panel', () => {
  it('lists each mount with its cost or why it cannot be mounted, and mounts the one chosen', () => {
    const { el, mounted } = setup({
      combatants: [
        tor(),
        mastiff,
        horse({ col: 7, row: 5 }),
        combatant({ id: 'x', label: 'Wight', mountedOnId: 'cav', col: 7, row: 5 }),
      ],
      mounts: choices,
    });
    const rows = Array.from(el.querySelectorAll('.mp__row'), (r) => plain(r));
    expect(rows[0]).toContain('Mastim');
    expect(rows[0]).toContain('Ao lado · custa 3,8 m');
    expect(rows[1]).toContain('Já leva Wight.');
    const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>('.mp__go'));
    expect(buttons[1].getAttribute('aria-disabled')).toBe('true');
    expect(buttons[1].classList.contains('mr-button--off')).toBe(true);
    buttons[0].click();
    expect(mounted).toEqual(['mas']);
  });

  it('draws each row with the app token and reads the name and the reason with the button', () => {
    const { el } = setup({
      combatants: [tor(), mastiff, horse({ col: 7, row: 5 })],
      mounts: choices,
    });
    expect(el.querySelectorAll('.mp__row app-combatant-token').length).toBe(2);
    const refused = el.querySelectorAll<HTMLButtonElement>('.mp__go')[1];
    const ids = refused.getAttribute('aria-describedby')!.split(' ');
    expect(ids.map((id) => plain(el.querySelector('#' + id)))).toEqual([
      'Cavalo de guerra',
      'Já leva alguém.',
    ]);
    expect(refused.getAttribute('aria-label')).toBeNull();
  });

  it('presses nothing on a refused row', () => {
    const { el, fixture, mounted } = setup({
      combatants: [tor(), mastiff, horse({ col: 7, row: 5 })],
      mounts: choices,
    });
    el.querySelectorAll<HTMLButtonElement>('.mp__go')[1].click();
    fixture.detectChanges();
    expect(mounted).toEqual([]);
  });

  it('says "Montando…" on the row that was pressed only', () => {
    const { el, fixture } = setup({
      combatants: [tor(), mastiff, horse({ col: 7, row: 5 })],
      mounts: create(MountOptionsSchema, {
        mounts: [
          create(MountChoiceSchema, { combatantId: 'mas', costDft: 125 }),
          create(MountChoiceSchema, { combatantId: 'cav', costDft: 125 }),
        ],
      }),
    });
    const buttons = () => Array.from(el.querySelectorAll<HTMLButtonElement>('.mp__go'));
    buttons()[1].click();
    fixture.componentRef.setInput('busy', true);
    fixture.detectChanges();
    expect(buttons().map((b) => plain(b))).toEqual(['Montar', 'Montando…']);
    fixture.componentRef.setInput('busy', false);
    fixture.detectChanges();
    expect(buttons().map((b) => plain(b))).toEqual(['Montar', 'Montar']);
  });

  it('says what to do when a mount is there to get on, and nothing when none can be', () => {
    const open = setup({ combatants: [tor(), mastiff], mounts: choices });
    expect(plain(open.el.querySelector('[data-testid="mount-hint"]'))).toBe(
      'Escolha um quadrado no mapa, ou monte.',
    );
    const shut = setup({
      combatants: [tor(), horse({ col: 7, row: 5 })],
      mounts: create(MountOptionsSchema, {
        mounts: [create(MountChoiceSchema, { combatantId: 'cav', refusal: MountRefusal.TAKEN })],
      }),
    });
    expect(shut.el.querySelector('[data-testid="mount-hint"]')).toBeNull();
  });

  it('shows no panel on the read-only map', () => {
    const { el } = setup({ combatants: [tor(), mastiff], mounts: choices, readOnly: true });
    expect(el.querySelector('app-mount-panel')).toBeNull();
  });

  it('shows no panel when the server lists no mount', () => {
    const { el } = setup({ combatants: [tor(), mastiff], mounts: create(MountOptionsSchema, {}) });
    expect(el.querySelector('app-mount-panel')).toBeNull();
  });
});

describe('MovePage, mounted', () => {
  const ridden = [tor({ mountedOnId: 'cav' }), horse({ mountKind: 'controlled' })];

  it('moves the mount: its counter in the lead, the pair in the notice, the rider keeps the rest for getting off', () => {
    const { el, choose, press, moved } = setup({
      combatants: ridden,
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav', dismountCostDft: 150 }),
    });
    expect(plain(el)).toContain('Restam 18,0 m de 18,0 m ao Cavalo de guerra');
    expect(plain(el.querySelector('[data-testid="mounted-notice"]'))).toContain(
      'Montado no Cavalo de guerra. O movimento é do Cavalo de guerra, e Tor vai junto no mesmo quadrado.',
    );
    expect(plain(el.querySelector('.mr-legend'))).toContain('O Cavalo de guerra alcança');
    choose(8, 5);
    expect(plain(el.querySelector('.status__text'))).toContain(
      'Depois restam 13,5 m ao Cavalo de guerra. Os 9,0 m de Tor ficam para desmontar.',
    );
    press('Mover para cá');
    expect(moved).toEqual([{ col: 8, row: 5 }]);
  });

  it('puts the notice in a live region that was on the page before it, and names the group', () => {
    const { el } = setup({
      combatants: ridden,
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav' }),
    });
    const notice = el.querySelector('[data-testid="mounted-notice"]')!;
    expect(notice.getAttribute('role')).toBeNull();
    expect(notice.closest('[role="status"]')?.classList.contains('move__live')).toBe(true);
    expect(el.querySelector('[role="radiogroup"]')!.getAttribute('aria-label')).toBe(
      'Como se mover montado',
    );
  });

  it('keeps the segment row and the notice in one row', () => {
    const { el } = setup({
      combatants: ridden,
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav' }),
    });
    const row = el.querySelector('.move__ride')!;
    expect(row.querySelector('app-segmented')).not.toBeNull();
    expect(row.querySelector('[data-testid="mounted-notice"]')).not.toBeNull();
  });

  it('offers Cavalgar and Desmontar in place of Andar and Saltar', () => {
    const { el } = setup({
      combatants: ridden,
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav' }),
    });
    const labels = Array.from(el.querySelectorAll('.seg__item'), (l) => plain(l));
    expect(labels).toEqual(['Cavalgar', 'Desmontar']);
  });

  it('gets off on a landing square only, with the cost on the card', () => {
    const { el, pick, choose, press, landed } = setup({
      combatants: ridden,
      mounts: create(MountOptionsSchema, {
        mountedOnId: 'cav',
        dismountCostDft: 150,
        landing: [create(MountSquareSchema, { col: 6, row: 6 })],
      }),
    });
    pick('Desmontar');
    expect(plain(el)).toContain(
      'Desmontar custa 4,5 m, metade do deslocamento de Tor. Toque num quadrado ao lado do Cavalo de guerra.',
    );
    choose(9, 9);
    expect(landed).toEqual([]);
    expect(plain(el.querySelector('[data-testid="dismount-ignored"]'))).toBe(
      'Só dá para descer num quadrado destacado, ao lado do Cavalo de guerra.',
    );
    expect(el.querySelector('.move__go')!.getAttribute('aria-disabled')).toBe('true');
    choose(6, 6);
    expect(plain(el.querySelector('.status__title'))).toBe('Desmontar aqui · 4,5 m');
    expect(plain(el.querySelector('.status__text'))).toBe(
      'Depois restam 4,5 m a Tor, a pé. O Cavalo de guerra volta à iniciativa própria.',
    );
    expect(el.querySelector('[data-testid="dismount-ignored"]')).toBeNull();
    press('Desmontar aqui');
    expect(landed).toEqual([{ col: 6, row: 6 }]);
  });

  it('dashes Desmontar with the reason when the server refuses it', () => {
    const { el } = setup({
      combatants: ridden,
      mounts: create(MountOptionsSchema, {
        mountedOnId: 'cav',
        dismountRefusal: MountRefusal.NO_ROOM,
      }),
    });
    const input = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')).find(
      (i) => plain(i.closest('label')).includes('Desmontar'),
    )!;
    // Off but still focusable: it reads the reason, as every dashed control of the app does.
    expect(input.disabled).toBe(false);
    expect(input.getAttribute('aria-disabled')).toBe('true');
    const why = el.querySelector('#' + input.getAttribute('aria-describedby'));
    expect(plain(why)).toBe('Nenhum quadrado livre ao lado do Cavalo de guerra.');
    input.click();
    expect(input.checked).toBe(false);
  });

  it('leaves only Desmontar on a mount with a turn of its own', () => {
    const { el } = setup({
      combatants: [tor({ mountedOnId: 'cav' }), horse({ mountKind: 'independent' })],
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav', dismountCostDft: 150 }),
    });
    expect(plain(el.querySelector('.move__why'))).toBe('Montaria independente: anda na vez dela.');
    expect(plain(el)).toContain('Desmontar custa 4,5 m');
    const ride = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')).find(
      (i) => plain(i.closest('label')).includes('Cavalgar'),
    )!;
    expect(ride.getAttribute('aria-disabled')).toBe('true');
    expect(ride.getAttribute('aria-describedby')).toBe('move-why-ride');
  });

  it('treats a mount with no mark as independent: the rider cannot walk it', () => {
    const { el } = setup({
      combatants: [tor({ mountedOnId: 'cav' }), horse({ mountKind: '' })],
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav', dismountCostDft: 150 }),
    });
    expect(plain(el.querySelector('.move__why'))).toBe('Montaria independente: anda na vez dela.');
  });

  it("rides the mount the master marked, for the rider's player and for the master", () => {
    // The server sends the rider's player the mark of the mount it rides (RN-10, RN-21).
    const player = setup({
      combatants: [tor({ mountedOnId: 'cav' }), horse({ mountKind: 'controlled' })],
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav' }),
    });
    expect(player.el.querySelector('[data-testid="mounted-notice"]')).not.toBeNull();
    const master = setup({
      combatants: [tor({ mine: false, mountedOnId: 'cav' }), horse({ mountKind: 'controlled' })],
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav' }),
      master: true,
    });
    expect(master.el.querySelector('[data-testid="mounted-notice"]')).not.toBeNull();
  });

  it('waits for the options that follow a mount before it speaks of getting off', () => {
    // Right after mounting the mount's options still describe the rider on foot: no cost, no landing.
    const { el } = setup({
      combatants: [tor({ mountedOnId: 'cav' }), horse({ mountKind: 'independent' })],
      mounts: create(MountOptionsSchema, { mountedOnId: '' }),
    });
    expect(plain(el)).not.toContain('Desmontar custa 0,0 m');
  });

  it('forgets the tap that was ignored when the rider gets on or off', () => {
    const { el, choose, fixture } = setup({
      combatants: [tor({ mountedOnId: 'cav' }), horse({ mountKind: 'independent' })],
      mounts: create(MountOptionsSchema, {
        mountedOnId: 'cav',
        landing: [create(MountSquareSchema, { col: 6, row: 6 })],
      }),
    });
    choose(9, 9);
    expect(el.querySelector('[data-testid="dismount-ignored"]')).not.toBeNull();
    const set = (mountedOnId: string) => {
      fixture.componentRef.setInput(
        'encounter',
        encounter({
          combatants: [tor({ mountedOnId }), horse({ mountKind: 'independent' })],
          currentCombatantId: 'tor',
          turnGroupIds: ['tor'],
        }),
      );
      fixture.componentRef.setInput('mounts', create(MountOptionsSchema, { mountedOnId }));
      fixture.detectChanges();
    };
    set('');
    set('cav');
    expect(el.querySelector('[data-testid="dismount-ignored"]')).toBeNull();
  });

  it('shows the warning of a failed dismount once: Cavalgar and back does not bring it again', () => {
    const { el, pick, fixture } = setup({
      combatants: ridden,
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav' }),
    });
    pick('Desmontar');
    fixture.componentRef.setInput('dismountBlocked', true);
    fixture.detectChanges();
    expect(el.querySelector('[data-testid="dismount-blocked"]')).not.toBeNull();
    pick('Cavalgar');
    pick('Desmontar');
    expect(el.querySelector('[data-testid="dismount-blocked"]')).toBeNull();
  });

  it('says something unseen holds the square, never what, in the live region, and drops it for Cavalgar', () => {
    const { el, pick, fixture } = setup({
      combatants: ridden,
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav' }),
    });
    pick('Desmontar');
    fixture.componentRef.setInput('dismountBlocked', true);
    fixture.detectChanges();
    const warning = el.querySelector('[data-testid="dismount-blocked"]')!;
    expect(plain(warning)).toBe(
      'Algo bloqueia o caminho: Tor continua montado e nada foi gasto. Escolha outro quadrado.',
    );
    expect(warning.getAttribute('role')).toBeNull();
    expect(warning.closest('[role="status"]')?.classList.contains('move__live')).toBe(true);
    pick('Cavalgar');
    expect(el.querySelector('[data-testid="dismount-blocked"]')).toBeNull();
  });
});
