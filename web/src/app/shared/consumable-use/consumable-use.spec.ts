import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { Subject, of } from 'rxjs';

import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  ConsumableTargetSchema,
  ConsumableTargetState,
  UseConsumableResponseSchema,
} from '../../../gen/meurpg/play/v1/charges_pb';
import {
  ConsumableUseOutcome,
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { AttunementEntryVm } from '../../core/characters/attunement';
import type { ConsumableVm } from '../../core/characters/consumable-vm';
import type { TurnBlock } from '../../core/characters/consumables';
import type { ChargesClient } from '../../core/play/charges-client';
import { ConsumableUse, type ConsumableUseHost } from './consumable-use';
import type { UseConsumableData } from './use-consumable-sheet';

const POTION: ConsumableVm = {
  kind: 'potion',
  costsAction: true,
  heal: { count: 2, sides: 4, modifier: 2, text: '2d4 + 2' },
};
const OIL: ConsumableVm = { kind: 'oil', costsAction: false, heal: null };

const entry = (id: string, over: Partial<AttunementEntryVm> = {}): AttunementEntryVm => ({
  id,
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

const gone = (reason: EncounterBlockedReason) =>
  new ConnectError('x', Code.FailedPrecondition, undefined, [
    { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
  ]);

const heal = () =>
  create(UseConsumableResponseSchema, {
    outcome: ConsumableUseOutcome.HEALED,
    diceText: '2d4+2',
    faces: [3, 2],
    rollTotal: 7,
    healed: 7,
  });

const apply = { target: null, roll: { inApp: true } } as const;
const brisa = { id: 'c2', name: 'Brisa', down: false, line: '' };

describe('ConsumableUse', () => {
  const client = { useConsumable: vi.fn(), listConsumableTargets: vi.fn() };
  const reload = vi.fn();
  const focusConsumable = vi.fn();
  const dice = vi.fn();
  const openUse = vi.fn();
  let opened: UseConsumableData | undefined;
  let entries: AttunementEntryVm[];
  let isMaster: boolean;
  let consumables: ConsumableUse;
  let inCombat: boolean | null;
  let turnBlock: TurnBlock | null;

  beforeEach(() => {
    opened = undefined;
    isMaster = false;
    inCombat = null;
    turnBlock = null;
    entries = [
      entry('p1'),
      entry('p2'),
      entry('r1', { key: 'item:ring', namePt: 'Anel', consumable: undefined }),
    ];
    Object.values(client).forEach((f) => f.mockReset());
    reload.mockReset().mockResolvedValue(true);
    focusConsumable.mockReset();
    dice.mockReset().mockResolvedValue({
      mode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
    });
    openUse.mockReset().mockImplementation((data: UseConsumableData) => {
      opened = data;
      return of(true);
    });
    const host: ConsumableUseHost = {
      client: () => client as unknown as ChargesClient,
      campaignId: () => 'camp',
      owner: () => ({ id: 'c1', name: 'Tor', byMaster: isMaster, entries }),
      inCombat: () => inCombat,
      turnBlock: () => turnBlock,
      reload,
      dice,
      focusConsumable,
      openUse,
    };
    consumables = new ConsumableUse(host);
  });

  async function open(over: AttunementEntryVm = entries[0]) {
    consumables.openUse(over);
    await vi.waitFor(() => expect(opened).toBeDefined());
    return opened!;
  }

  it('opens the sheet with the item, what the server says it is, how many are left and how the dice come', async () => {
    const data = await open();
    expect(data).toMatchObject({
      itemName: 'Poção de cura',
      kind: 'potion',
      consumable: POTION,
      effectPt: 'Cura 2d4 + 2',
      left: 2,
      actingFor: null,
      inCombat: null,
      blockedBy: null,
      canApp: true,
      canType: true,
      preferApp: true,
    });
  });

  it('opens an oil as an oil: the kind is the server field, whatever the item key', async () => {
    entries = [
      entry('o1', { key: 'item:oil-of-sharpness', namePt: 'Óleo de afiar', consumable: OIL }),
    ];
    const data = await open(entries[0]);
    expect(data.kind).toBe('oil');
  });

  it('opens nothing for an item the server says is no consumable', async () => {
    consumables.openUse(entries[2]);
    await Promise.resolve();
    expect(dice).not.toHaveBeenCalled();
    expect(openUse).not.toHaveBeenCalled();
  });

  it('says there is a combat when the page that owns the use is the combat', async () => {
    inCombat = true;
    const data = await open();
    expect(data.inCombat).toBe(true);
  });

  it('opens for the master as the character it acts for', async () => {
    isMaster = true;
    const data = await open();
    expect(data.actingFor).toBe('Tor');
  });

  it('plays the physical dice of a table that forces them', async () => {
    dice.mockResolvedValue({ mode: DiceMode.PHYSICAL, preference: DicePreference.PHYSICAL });
    const data = await open();
    expect([data.canApp, data.canType, data.preferApp]).toEqual([false, true, false]);
  });

  describe('a double tap on "Usar"', () => {
    it('opens one sheet, and another one once it has closed', async () => {
      const closed = new Subject<boolean | undefined>();
      openUse.mockReturnValue(closed);
      consumables.openUse(entries[0]);
      consumables.openUse(entries[0]);
      await vi.waitFor(() => expect(openUse).toHaveBeenCalled());
      consumables.openUse(entries[0]);
      await Promise.resolve();
      expect(openUse).toHaveBeenCalledTimes(1);
      expect(dice).toHaveBeenCalledTimes(1);
      closed.next(true);
      closed.complete();
      consumables.openUse(entries[0]);
      await vi.waitFor(() => expect(openUse).toHaveBeenCalledTimes(2));
    });

    it('opens again after the table dice could not be read', async () => {
      dice.mockRejectedValueOnce(new Error('off'));
      consumables.openUse(entries[0]);
      await vi.waitFor(() => expect(dice).toHaveBeenCalledTimes(1));
      await Promise.resolve();
      consumables.openUse(entries[0]);
      await vi.waitFor(() => expect(openUse).toHaveBeenCalledTimes(1));
    });
  });

  describe('the receivers', () => {
    it('lists the receivers the server sends, worded for the board, and whether there is a combat', async () => {
      client.listConsumableTargets.mockResolvedValue({
        targets: [
          create(ConsumableTargetSchema, {
            characterId: 'c2',
            name: 'Brisa',
            distanceFt: 5,
            state: ConsumableTargetState.DOWN,
            mayReceive: true,
          }),
        ],
        inCombat: true,
      });
      const data = await open();
      const out = await data.loadTargets();
      expect(client.listConsumableTargets).toHaveBeenCalledWith('camp', 'c1', 'p1');
      expect(out).toEqual({
        targets: [{ id: 'c2', name: 'Brisa', down: true, line: 'A 1,5 m · Caída' }],
        inCombat: true,
        error: null,
      });
    });

    it('says there is no combat when the server says so, whatever the page knew', async () => {
      inCombat = true;
      client.listConsumableTargets.mockResolvedValue({ targets: [], inCombat: false });
      const data = await open();
      expect((await data.loadTargets()).inCombat).toBe(false);
    });

    it('says why the list could not be read, in words, and does not know about a combat', async () => {
      client.listConsumableTargets.mockRejectedValue(gone(EncounterBlockedReason.ITEM_GONE));
      const data = await open();
      const out = await data.loadTargets();
      expect(out).toMatchObject({ targets: [], inCombat: null });
      expect(out.error).toMatchObject({
        lead: 'Essa poção já foi usada.',
        text: 'A ficha foi atualizada.',
      });
      expect(reload).toHaveBeenCalled();
    });

    it('words a list that could not be read as that, never as dice or as a receiver who left', async () => {
      client.listConsumableTargets.mockRejectedValue(new ConnectError('x', Code.InvalidArgument));
      const data = await open();
      expect((await data.loadTargets()).error?.text).toBe(
        'Não deu para ver quem pode receber a poção.',
      );
      client.listConsumableTargets.mockRejectedValue(new ConnectError('x', Code.Unavailable));
      const out = await data.loadTargets();
      expect(out.error).toMatchObject({ retry: true });
    });

    it('rebinds to a remaining unit when the one it was open on is gone, and reads the list again for it', async () => {
      client.listConsumableTargets
        .mockRejectedValueOnce(gone(EncounterBlockedReason.ITEM_GONE))
        .mockResolvedValueOnce({ targets: [], inCombat: false });
      const data = await open();
      entries = [entry('p2')];
      const out = await data.loadTargets();
      expect(client.listConsumableTargets).toHaveBeenLastCalledWith('camp', 'c1', 'p2');
      expect(out).toMatchObject({ rebound: true, left: 1, error: null, inCombat: false });
    });

    it('closes the sheet with the notice when the unit it was open on is gone and none is left', async () => {
      client.listConsumableTargets.mockRejectedValue(gone(EncounterBlockedReason.ITEM_GONE));
      const data = await open();
      entries = [];
      const out = await data.loadTargets();
      expect(out).toMatchObject({ close: true, left: 0 });
      expect(consumables.result()).toMatchObject({
        tone: 'neutral',
        headline: 'A poção acabou.',
      });
    });
  });

  describe('what keeps a use from the Action', () => {
    it('keeps the use a potion costs from a sheet that knows the turn does not allow it', async () => {
      turnBlock = 'action';
      expect((await open()).blockedBy).toBe('action');
    });

    it('says it is not the turn when the page knows', async () => {
      turnBlock = 'turn';
      expect((await open()).blockedBy).toBe('turn');
    });

    it('blocks nothing for an item the server says costs no action, whatever the turn', async () => {
      turnBlock = 'action';
      entries = [
        entry('o1', { key: 'item:oil-of-sharpness', namePt: 'Óleo de afiar', consumable: OIL }),
      ];
      expect((await open(entries[0])).blockedBy).toBeNull();
    });
  });

  describe('a use', () => {
    it('drinks: sends the id of one entry of the group, reads the sheet and says what happened', async () => {
      client.useConsumable.mockResolvedValue(heal());
      entries = [entry('p1')];
      reload.mockImplementation(() => {
        entries = [entry('p2')];
        return Promise.resolve(true);
      });
      const data = await open();
      expect(await data.use(apply, 'k1')).toEqual({ refusal: null });
      expect(client.useConsumable).toHaveBeenCalledWith(
        'camp',
        'c1',
        'p1',
        '',
        { inApp: true },
        'k1',
      );
      expect(consumables.result()).toEqual({
        tone: 'good',
        icon: 'favorite',
        headline: 'Tor recebeu 7 PV.',
        detail: '2d4 + 2: 3 + 2 + 2 = 7. Resta 1 Poção de cura.',
        text: [],
      });
    });

    it('says what is left from what the page holds after it read the character again, never from a guess', async () => {
      client.useConsumable.mockResolvedValue(heal());
      // The sheet opened with two units, but the read after the use shows two: another unit came in meanwhile.
      const data = await open();
      expect(data.left).toBe(2);
      await data.use(apply, 'k1');
      expect(consumables.result()?.detail).toBe(
        '2d4 + 2: 3 + 2 + 2 = 7. Restam 2 unidades de Poção de cura.',
      );
    });

    it('waits for the read of the character, however late it lands, before it says what is left', async () => {
      client.useConsumable.mockResolvedValue(heal());
      let land!: () => void;
      reload.mockImplementation(
        () =>
          new Promise<boolean>((resolve) => {
            land = () => {
              entries = [entry('p2')];
              resolve(true);
            };
          }),
      );
      const data = await open();
      const used = data.use(apply, 'k1');
      await vi.waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
      expect(consumables.result()).toBeNull();
      land();
      await used;
      expect(consumables.result()?.detail).toBe('2d4 + 2: 3 + 2 + 2 = 7. Resta 1 Poção de cura.');
    });

    it('rebinds after a refusal from the sheet the late read brings', async () => {
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.ITEM_GONE));
      let land!: () => void;
      reload.mockImplementation(
        () =>
          new Promise<boolean>((resolve) => {
            land = () => {
              entries = [entry('p2'), entry('p3')];
              resolve(true);
            };
          }),
      );
      const data = await open();
      const used = data.use(apply, 'k1');
      await vi.waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
      land();
      expect(await used).toMatchObject({ left: 2, rebound: true });
    });

    it('says nothing of what is left when the new read of the sheet is not on screen', async () => {
      client.useConsumable.mockResolvedValue(heal());
      reload.mockResolvedValue(false);
      const data = await open();
      await data.use(apply, 'k1');
      expect(consumables.result()?.detail).toBe('2d4 + 2: 3 + 2 + 2 = 7.');
    });

    it('gives it to a character who is down, and says they wake', async () => {
      client.useConsumable.mockResolvedValue(heal());
      const data = await open();
      await data.use(
        {
          target: { id: 'c2', name: 'Brisa', down: true, line: 'A 1,5 m · Caída' },
          roll: { faces: [3, 2] },
        },
        'k2',
      );
      expect(client.useConsumable).toHaveBeenCalledWith(
        'camp',
        'c1',
        'p1',
        'c2',
        { faces: [3, 2] },
        'k2',
      );
      expect(consumables.result()?.headline).toBe('Brisa recebeu 7 PV e acorda.');
    });

    it('says a potion drunk at full hit points is nothing healed, with no success', async () => {
      client.useConsumable.mockResolvedValue(
        create(UseConsumableResponseSchema, {
          outcome: ConsumableUseOutcome.HEALED,
          faces: [1, 1],
          rollTotal: 4,
          healed: 0,
        }),
      );
      const data = await open();
      await data.use(apply, 'k');
      expect(consumables.result()).toMatchObject({
        tone: 'neutral',
        headline: 'Tor já estava com os PV cheios.',
      });
    });

    it('says an oil is used and not drunk', async () => {
      client.useConsumable.mockResolvedValue(
        create(UseConsumableResponseSchema, { outcome: ConsumableUseOutcome.TEXT_ONLY }),
      );
      entries = [
        entry('o1', {
          key: 'item:oil-of-sharpness',
          namePt: 'Óleo de afiar',
          effectPt: '',
          consumable: OIL,
        }),
      ];
      const data = await open(entries[0]);
      await data.use(apply, 'k');
      expect(consumables.result()?.headline).toBe('Tor usou o Óleo de afiar.');
    });

    it('says a potion with no effect of its own was drunk', async () => {
      client.useConsumable.mockResolvedValue(
        create(UseConsumableResponseSchema, { outcome: ConsumableUseOutcome.TEXT_ONLY }),
      );
      entries = [entry('p1', { effectPt: '', consumable: { ...POTION, heal: null } })];
      const data = await open(entries[0]);
      await data.use(apply, 'k');
      expect(consumables.result()?.headline).toBe('Tor bebeu a Poção de cura.');
    });

    it('puts the focus back on the row of the item once the sheet closes', async () => {
      await open();
      expect(focusConsumable).toHaveBeenCalledWith('item:potion-of-healing');
    });

    it('clears the last notice when the sheet opens again', async () => {
      client.useConsumable.mockResolvedValue(
        create(UseConsumableResponseSchema, { outcome: ConsumableUseOutcome.TEXT_ONLY }),
      );
      const data = await open();
      await data.use(apply, 'k');
      expect(consumables.result()).not.toBeNull();
      consumables.openUse(entries[0]);
      expect(consumables.result()).toBeNull();
    });
  });

  describe('a refusal', () => {
    it.each([
      ['the receiver died', EncounterBlockedReason.TARGET_DEAD, 'Brisa morreu.'],
      ['the receiver went away', EncounterBlockedReason.TARGET_NOT_ADJACENT, 'Longe demais.'],
      ['the item is gone', EncounterBlockedReason.ITEM_GONE, 'Essa poção já foi usada.'],
    ])('reads the sheet again and leads in bold when %s', async (_name, reason, lead) => {
      client.useConsumable.mockRejectedValue(gone(reason));
      const data = await open();
      const answer = await data.use({ target: brisa, roll: { inApp: true } }, 'k1');
      expect(answer.refusal?.lead).toBe(lead);
      expect(reload).toHaveBeenCalledTimes(1);
      expect(consumables.result()).toBeNull();
    });

    it('says the receiver is no longer at the side, by name', async () => {
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.TARGET_NOT_ADJACENT));
      const data = await open();
      const answer = await data.use({ target: brisa, roll: { inApp: true } }, 'k1');
      expect(answer.refusal?.text).toBe('Brisa não está mais ao seu lado. Nada foi gasto.');
    });

    it('names the character the master acts for when it is not their turn, as a ban line that blocks the sheet', async () => {
      isMaster = true;
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.NOT_YOUR_TURN));
      const data = await open();
      const answer = await data.use(apply, 'k1');
      expect(answer.refusal).toMatchObject({
        look: 'ban',
        block: 'turn',
        lead: 'Só na vez de Tor.',
      });
    });

    it('names who is dead: the receiver when the server says it is theirs, the character otherwise', async () => {
      const dead = (characterId: string) =>
        new ConnectError('x', Code.FailedPrecondition, undefined, [
          {
            desc: CharacterBlockedSchema,
            value: create(CharacterBlockedSchema, {
              reason: CharacterBlockedReason.CHARACTER_DEAD,
              characterId,
            }),
          },
        ]);
      const data = await open();
      client.useConsumable.mockRejectedValueOnce(dead('c2'));
      expect((await data.use({ target: brisa, roll: { inApp: true } }, 'k')).refusal?.lead).toBe(
        'Brisa morreu.',
      );
      client.useConsumable.mockRejectedValueOnce(dead('c1'));
      expect((await data.use({ target: brisa, roll: { inApp: true } }, 'k')).refusal?.lead).toBe(
        'Tor morreu.',
      );
    });

    it('blames typed dice for an invalid argument only when dice were typed', async () => {
      client.useConsumable.mockRejectedValue(new ConnectError('x', Code.InvalidArgument));
      const data = await open();
      expect((await data.use({ target: null, roll: { faces: [9, 9] } }, 'k')).refusal?.lead).toBe(
        'Dados inválidos.',
      );
      expect((await data.use(apply, 'k2')).refusal?.text).toBe(
        'Não deu para usar a poção. Nada foi gasto.',
      );
    });

    it('rebinds to another unit of the item when the one it was open on is gone, with the count the page now holds', async () => {
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.ITEM_GONE));
      const data = await open();
      entries = [entry('p2'), entry('p3')];
      const answer = await data.use(apply, 'k1');
      expect(answer).toMatchObject({ left: 2, rebound: true });
      client.useConsumable.mockResolvedValue(heal());
      await data.use(apply, 'k2');
      expect(client.useConsumable).toHaveBeenLastCalledWith(
        'camp',
        'c1',
        'p2',
        '',
        { inApp: true },
        'k2',
      );
    });

    it('closes the sheet with the notice when the item is gone and none is left', async () => {
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.ITEM_GONE));
      const data = await open();
      entries = [];
      const answer = await data.use(apply, 'k1');
      expect(answer).toMatchObject({ close: true, left: 0 });
      expect(consumables.result()).toMatchObject({ headline: 'A poção acabou.' });
    });

    it('follows the count the page holds after a refusal that left the unit', async () => {
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.TARGET_DEAD));
      const data = await open();
      expect(await data.use({ target: brisa, roll: { inApp: true } }, 'k')).toMatchObject({
        left: 2,
      });
    });

    it('does not say the sheet was updated when the item is gone and the read after it is not on screen: it says it is unread, and rereads on demand', async () => {
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.ITEM_GONE));
      reload.mockResolvedValue(false);
      const data = await open();
      const answer = await data.use(apply, 'k1');
      expect(answer).toMatchObject({ unread: true });
      expect(answer.close).toBeUndefined();
      expect(consumables.result()).toBeNull();
      // The read lands this time and the unit the sheet was open on is gone: another takes its place.
      reload.mockResolvedValue(true);
      entries = [entry('p2'), entry('p3')];
      expect(await data.reread()).toEqual({ left: 2, rebound: true });
    });

    it('closes the sheet with the notice when the reread finds no unit left', async () => {
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.ITEM_GONE));
      reload.mockResolvedValue(false);
      const data = await open();
      await data.use(apply, 'k1');
      reload.mockResolvedValue(true);
      entries = [];
      expect(await data.reread()).toEqual({ left: 0, close: true });
      expect(consumables.result()).toMatchObject({ headline: 'A poção acabou.' });
    });

    it('keeps saying it is unread while the reread is not on screen either', async () => {
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.ITEM_GONE));
      reload.mockResolvedValue(false);
      const data = await open();
      await data.use(apply, 'k1');
      expect(await data.reread()).toEqual({ unread: true });
    });

    it('is unread too when the receivers read finds the item gone and the page cannot read the sheet', async () => {
      client.listConsumableTargets.mockRejectedValue(gone(EncounterBlockedReason.ITEM_GONE));
      reload.mockResolvedValue(false);
      const data = await open();
      expect(await data.loadTargets()).toMatchObject({ unread: true, targets: [] });
    });

    it('says nothing of the count when the new read is not on screen', async () => {
      client.useConsumable.mockRejectedValue(gone(EncounterBlockedReason.TARGET_DEAD));
      reload.mockResolvedValue(false);
      const data = await open();
      const answer = await data.use({ target: brisa, roll: { inApp: true } }, 'k');
      expect(answer.left).toBeUndefined();
      expect(answer.close).toBeUndefined();
    });
  });

  describe('what follows an answer', () => {
    it('says the unit was used when the read of the sheet after the answer throws: only the call itself may say nothing was spent', async () => {
      client.useConsumable.mockResolvedValue(heal());
      reload.mockRejectedValue(new ConnectError('x', Code.Unavailable));
      const data = await open();
      const answer = await data.use(apply, 'k1');
      expect(answer).toEqual({ refusal: null });
      expect(consumables.result()).toMatchObject({
        tone: 'good',
        headline: 'Tor recebeu 7 PV.',
        detail: '2d4 + 2: 3 + 2 + 2 = 7.',
      });
    });
  });

  describe('a call that failed', () => {
    it('does not read the sheet again for a call that never left: nothing changed, and it can be sent again', async () => {
      client.useConsumable.mockRejectedValue(new ConnectError('x', Code.Unavailable));
      const data = await open();
      const answer = await data.use(apply, 'k1');
      expect(answer.refusal).toMatchObject({
        lead: 'Sem conexão.',
        text: 'Não deu para usar a poção. Nada foi gasto.',
        retry: true,
      });
      expect(reload).not.toHaveBeenCalled();
    });

    it.each([Code.Unknown, Code.Internal, Code.DeadlineExceeded])(
      'reads the sheet again and does not say nothing was spent for %s',
      async (code) => {
        client.useConsumable.mockRejectedValue(new ConnectError('x', code));
        const data = await open();
        const answer = await data.use(apply, 'k1');
        expect(reload).toHaveBeenCalledTimes(1);
        expect(`${answer.refusal?.lead} ${answer.refusal?.text}`).toBe(
          'Não sabemos se a unidade foi usada: confira a ficha.',
        );
      },
    );

    it.each([Code.FailedPrecondition, Code.NotFound, Code.InvalidArgument, Code.PermissionDenied])(
      'reads the sheet again for %s',
      async (code) => {
        client.useConsumable.mockRejectedValue(new ConnectError('x', code));
        const data = await open();
        await data.use(apply, 'k1');
        expect(reload).toHaveBeenCalledTimes(1);
      },
    );

    it('says an unknown outcome in a notice of the page when the unit is gone from the sheet and none is left', async () => {
      client.useConsumable.mockRejectedValue(new ConnectError('x', Code.DeadlineExceeded));
      const data = await open();
      entries = [];
      const answer = await data.use(apply, 'k1');
      expect(answer.close).toBe(true);
      expect(consumables.result()).toMatchObject({
        headline: 'Não sabemos se a unidade foi usada:',
        detail: 'confira a ficha.',
      });
    });
  });
});
