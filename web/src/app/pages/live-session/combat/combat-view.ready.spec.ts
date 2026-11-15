import { create } from '@bufbuild/protobuf';

import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
  GetTurnOptionsResponseSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { ActionEconomy, AttackKind } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { settle } from '../../../../test-settle';
import { PHONE_QUERY } from '../../../shared/map-view/media-query';
import { mountCombatView } from './combat-view-testing';
import { ReadySheet } from './ready-sheet/ready-sheet';

// "Preparar" (#582) is the Ready action of "Outras": its row opens the sheet, never `TakeAction`; the card of the held action
// has "Desistir", which calls `CancelReadied`.

const toren = (over: Record<string, unknown> = {}) =>
  combatant({
    id: 't',
    label: 'Toren',
    kind: CombatantKind.PLAYER,
    side: CombatantSide.PARTY,
    characterId: 'toren-c',
    mine: true,
    placed: false,
    speedFt: 30,
    ...over,
  } as never);
const zombie = () => combatant({ id: 'z1', label: 'Zumbi 1', placed: false } as never);

const theatre = (over: Record<string, unknown> = {}) =>
  encounter({
    mode: EncounterMode.THEATRE,
    mapId: '',
    gridColumns: 0,
    gridRows: 0,
    round: 2,
    currentCombatantId: 't',
    combatants: [toren(over), zombie()],
  } as never);

const turns = () => (id: string) =>
  id === 't'
    ? create(GetTurnOptionsResponseSchema, {
        options: {
          attacks: [
            {
              enabled: true,
              attack: {
                key: 'equipment:longsword',
                name: 'Longsword',
                namePt: 'Espada longa',
                attackBonus: 6,
                damage: '1d8+3',
                damageTypePt: 'cortante',
                kind: AttackKind.WEAPON,
                rangeFt: 5,
              },
            },
          ],
          standardActions: [
            {
              enabled: true,
              viaReadyAction: true,
              action: { key: 'standard:ready', namePt: 'Preparar', economy: ActionEconomy.ACTION },
            },
          ],
        },
      } as never)
    : undefined;

async function setup(opts: Parameters<typeof mountCombatView>[0]) {
  const m = mountCombatView(opts);
  await settle(m.fixture, 2);
  return m;
}

describe('CombatView: Preparar (#582)', () => {
  it('opens the Ready sheet with the turn, the creatures of the fight and where the readier stands', async () => {
    const m = await setup({ enc: theatre(), matches: [PHONE_QUERY], turnOptions: turns() });
    m.view['sheets'].actions.openReady();
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(ReadySheet);
    expect(m.opened[0].config.ariaLabel).toBe('Preparar uma ação');
    expect(m.opened[0].config.data).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      combatantId: 't',
      ownerLabel: 'Toren',
      theatre: true,
      noSpeed: false,
      speedFt: 30,
      concentrating: '',
    });
    expect(
      (m.opened[0].config.data['combatants'] as { label: string }[]).map((c) => c.label),
    ).toEqual(['Toren', 'Zumbi 1']);
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('names the concentration the readier keeps now, for the warning over a held spell', async () => {
    const m = await setup({
      enc: theatre({
        concentrationSpell: 'spell:fog-cloud',
        concentrationSpellNamePt: 'Névoa Obscurecente',
      }),
      matches: [PHONE_QUERY],
      turnOptions: turns(),
    });
    m.view['sheets'].actions.openReady();
    expect(m.opened[0].config.data['concentrating']).toBe('Névoa Obscurecente');
  });

  it('gives the held action up with CancelReadied, under one key, and applies the combat it answers', async () => {
    const held = {
      id: 'r1',
      triggerText: 'Quando o zumbi passar pela porta',
      kind: ReadiedKind.ATTACK,
      actionNamePt: 'Espada longa',
      targetCombatantId: 'z1',
      status: ReadiedStatus.WAITING,
    };
    const m = await setup({
      enc: theatre({ readied: held }),
      matches: [PHONE_QUERY],
      turnOptions: turns(),
      api: { cancelReadied: () => theatre() },
    });
    expect(m.fixture.nativeElement.querySelector('app-readied-card')).toBeTruthy();
    await m.view['sheets'].actions.giveUp('r1');
    expect(m.callsOf('cancelReadied')).toHaveLength(1);
    expect(m.callsOf('cancelReadied')[0].args).toEqual(['c', 'enc', 'r1', expect.any(String)]);
  });

  describe('the master', () => {
    const held = (over: Record<string, unknown> = {}) => ({
      id: 'r1',
      triggerText: 'Quando o zumbi passar pela porta',
      kind: ReadiedKind.ATTACK,
      actionNamePt: 'Espada longa',
      targetCombatantId: 'z1',
      status: ReadiedStatus.WAITING,
      ...over,
    });
    const table = (over: Record<string, unknown> = {}) =>
      encounter({
        mode: EncounterMode.THEATRE,
        mapId: '',
        gridColumns: 0,
        gridRows: 0,
        round: 2,
        currentCombatantId: 'z1',
        combatants: [zombie(), toren({ mine: false, readied: held(), ...over })],
      } as never);

    it('reads "Ações preparadas" in "Turno" and fires a row with FireReadied under one key', async () => {
      const m = await setup({
        enc: table(),
        master: true,
        api: { fireReadied: () => table({ readied: undefined }) },
      });
      const fold = m.fixture.nativeElement.querySelector('app-readied-fold') as HTMLElement;
      expect(fold.textContent).toContain('Ações preparadas');
      (fold.querySelector('button.row__fire') as HTMLButtonElement).click();
      await settle(m.fixture, 2);
      expect(m.callsOf('fireReadied')).toHaveLength(1);
      expect(m.callsOf('fireReadied')[0].args).toEqual(['c', 'enc', 'r1', expect.any(String)]);
    });

    it('marks the order row of whoever holds one and not the others', async () => {
      const m = await setup({ enc: table(), master: true });
      const rows = Array.from(
        m.fixture.nativeElement.querySelectorAll('app-order-list li.row'),
      ) as HTMLElement[];
      const flatText = (n: HTMLElement) => (n.textContent ?? '').replace(/\s+/g, ' ');
      expect(
        rows.filter((r) => flatText(r).includes('Preparada')).map((r) => r.dataset['combatant']),
      ).toEqual(['t']);
    });

    it('shows none of it to a player', async () => {
      const m = await setup({
        enc: table({ mine: true, readied: undefined }),
        matches: [PHONE_QUERY],
      });
      expect(m.fixture.nativeElement.querySelector('app-readied-fold')).toBeNull();
    });
  });
});
