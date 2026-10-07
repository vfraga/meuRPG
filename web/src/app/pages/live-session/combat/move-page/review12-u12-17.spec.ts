// Finding U12-17: a long jump that leaves a melee enemy's reach provokes (docs/architecture.md, Opportunity attacks),
// but jumpSummary always returns warning '', so the page neither warns nor offers Desengajar.
import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  CombatantKind,
  GetMoveOptionsResponseSchema,
  MoveRefusal,
  ReachableSquareSchema,
  RefusedSquareSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { JumpLimitsSchema } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import type { MapLayers } from '../../../../core/maps/layers';
import { type JumpRequest, MovePage } from './move-page';

const plain = (t: string | null | undefined) =>
  (t ?? '')
    .replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();

// Toren at (8, 7) with 9,0 m: Goblin 2 stands at (7, 7), the wall is at (8, 5).
const toren = combatant({
  id: 'toren',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  mine: true,
  col: 8,
  row: 7,
  speedFt: 30,
  speedDft: 300,
  movementLeftFt: 30,
  movementLeftDft: 300,
});
const goblin = combatant({ id: 'g2', label: 'Goblin 2', col: 7, row: 7 });

const options = create(GetMoveOptionsResponseSchema, {
  movementLeftDft: 300,
  reachable: [
    create(ReachableSquareSchema, { col: 9, row: 8, costDft: 71 }),
    create(ReachableSquareSchema, { col: 10, row: 7, costDft: 100, provokesReactorIds: ['g2'] }),
    create(ReachableSquareSchema, {
      col: 8,
      row: 9,
      costDft: 100,
      knownTrapName: 'Fosso escondido',
      knownTrapPointId: 'p',
    }),
  ],
  refused: [create(RefusedSquareSchema, { col: 8, row: 5, reason: MoveRefusal.WALL })],
});

const layers: MapLayers = {
  columns: 20,
  rows: 14,
  walls: [{ col: 8, row: 5 }],
  terrain: [{ col: 9, row: 8 }],
  half: [],
  threeQuarters: [],
};

function setup(
  over: {
    options?: typeof options | null;
    jumps?: ReturnType<typeof create<typeof JumpLimitsSchema>>;
    canDisengage?: boolean;
  } = {},
) {
  const fixture = TestBed.createComponent(MovePage);
  const ref = fixture.componentRef;
  ref.setInput(
    'encounter',
    encounter({
      combatants: [toren, goblin],
      currentCombatantId: 'toren',
      turnGroupIds: ['toren'],
    }),
  );
  ref.setInput('image', { url: '/images/x', width: 2000, height: 1400 });
  ref.setInput('mapName', 'A caverna do Vale Seco');
  ref.setInput('sessionNumber', 6);
  ref.setInput('options', over.options === undefined ? options : over.options);
  ref.setInput('layers', layers);
  ref.setInput('jumps', over.jumps);
  ref.setInput('canDisengage', over.canDisengage ?? true);
  const confirmed: unknown[] = [];
  const jumped: JumpRequest[] = [];
  let disengaged = 0;
  fixture.componentInstance.confirm.subscribe((s) => confirmed.push(s));
  fixture.componentInstance.jump.subscribe((j) => jumped.push(j));
  fixture.componentInstance.disengage.subscribe(() => disengaged++);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const choose = (col: number, row: number) => {
    const surface = el.querySelector<HTMLElement>('.cm__surface')!;
    // The map is one focus stop: a click picks the square under it.
    surface.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      width: 200,
      height: 140,
      right: 200,
      bottom: 140,
      x: 0,
      y: 0,
      toJSON: () => '',
    });
    surface.dispatchEvent(
      new MouseEvent('click', { clientX: col * 10 + 5, clientY: row * 10 + 5, bubbles: true }),
    );
    fixture.detectChanges();
  };
  const press = (name: string) => {
    Array.from(el.querySelectorAll('button'))
      .find((b) => plain(b.textContent).includes(name))!
      .click();
    fixture.detectChanges();
  };
  return { fixture, el, choose, press, confirmed, jumped, disengaged: () => disengaged };
}

describe('Review12 U12-17: long jump out of a reach shows no opportunity-attack warning', () => {
  const jumps = create(JumpLimitsSchema, {
    longRunningDft: 160,
    longStandingDft: 80,
    highRunningDft: 60,
    highStandingDft: 30,
    runningStart: true,
  });

  it('warns and offers Desengajar when the landing square leaves Goblin 2 reach', () => {
    const { fixture, el, choose } = setup({ jumps });
    const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
    radios[1].click();
    radios[1].dispatchEvent(new Event('change'));
    fixture.detectChanges();
    choose(10, 7); // reachable, provokesReactorIds = ['g2']
    expect(plain(el.querySelector('.status__title')?.textContent)).toBe('Saltar 3,0 m');
    expect(plain(el.textContent)).toContain(
      'Sair do alcance do Goblin 2 pode provocar um ataque de oportunidade.',
    );
    expect(plain(el.textContent)).toContain('Desengajar (gasta a ação)');
  });
});
