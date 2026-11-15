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
      .find((b) => plain(b).includes(name) || b.getAttribute('aria-label') === name)!
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

describe('MovePage, the "Montaria" panel', () => {
  it('lists each mount with its cost or why it cannot be mounted, and mounts the one chosen', () => {
    const { el, press, mounted } = setup({
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
    press('Montar Mastim');
    expect(mounted).toEqual(['mas']);
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
      'Montado no Cavalo de guerra. O movimento é dele, e Tor vai junto no mesmo quadrado.',
    );
    expect(plain(el.querySelector('.mr-legend'))).toContain('O Cavalo de guerra alcança');
    choose(8, 5);
    expect(plain(el.querySelector('.status__text'))).toContain(
      'Depois restam 13,5 m ao Cavalo de guerra. Os 9,0 m de Tor ficam para desmontar.',
    );
    press('Mover para cá');
    expect(moved).toEqual([{ col: 8, row: 5 }]);
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
    expect(plain(el)).toContain('Desmontar custa 4,5 m, metade do deslocamento de Tor.');
    choose(9, 9);
    expect(landed).toEqual([]);
    expect(el.querySelector('.move__go')!.getAttribute('aria-disabled')).toBe('true');
    choose(6, 6);
    expect(plain(el.querySelector('.status__title'))).toBe('Desmontar aqui · 4,5 m');
    expect(plain(el.querySelector('.status__text'))).toBe(
      'Depois restam 4,5 m a Tor, a pé. O cavalo volta à iniciativa dele.',
    );
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
    expect(input.disabled).toBe(true);
    expect(plain(el.querySelector('.move__why'))).toBe('Nenhum quadrado livre ao lado do cavalo.');
  });

  it('leaves only Desmontar on a mount with a turn of its own', () => {
    const { el } = setup({
      combatants: [tor({ mountedOnId: 'cav' }), horse({ mountKind: 'independent' })],
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav', dismountCostDft: 150 }),
    });
    expect(plain(el.querySelector('.move__why'))).toBe('Montaria independente: anda na vez dela.');
    expect(plain(el)).toContain('Desmontar custa 4,5 m');
  });

  it('says something unseen holds the square, never what', () => {
    const { el } = setup({
      combatants: ridden,
      mounts: create(MountOptionsSchema, { mountedOnId: 'cav' }),
      blocked: true,
    });
    expect(plain(el.querySelector('[data-testid="dismount-blocked"]'))).toBe(
      'Algo bloqueia o caminho: Tor continua montado e nada foi gasto. Escolha outro quadrado.',
    );
  });
});
