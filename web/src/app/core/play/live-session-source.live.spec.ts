import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterVitalsSchema,
  WatchGameSessionResponseSchema,
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
} from '../../../gen/meurpg/play/v1/play_pb';
import { FreeRollDieSchema, FreeRollSchema } from '../../../gen/meurpg/play/v1/free_dice_pb';
import { Recharge } from '../../../gen/meurpg/rules/v1/rules_pb';
import { TestBed } from '@angular/core/testing';
import { GetCharacterResponseSchema } from '../../../gen/meurpg/characters/v1/characters_pb';
import { ProficiencyLevel } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { Transport } from '@connectrpc/connect';

import {
  RequestRefusedReason,
  RequestRefusedSchema,
} from '../../../gen/meurpg/system/v1/request_limits_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import {
  LiveSessionSourceLive,
  classifyLiveError,
  toShownImageVm,
  toVitalsVm,
} from './live-session-source.live';

function tooLong(): ConnectError {
  return new ConnectError('request over its size limit', Code.InvalidArgument, undefined, [
    {
      desc: RequestRefusedSchema,
      value: create(RequestRefusedSchema, { reason: RequestRefusedReason.TEXT_TOO_LONG }),
    },
  ]);
}

function blocked(reason: GameSessionBlockedReason): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: GameSessionBlockedSchema, value: { reason } },
  ]);
}

describe('classifyLiveError', () => {
  it('reads the code and the typed detail, never the message', () => {
    expect(classifyLiveError(new ConnectError('x', Code.NotFound))).toBe('no-access');
    expect(classifyLiveError(new ConnectError('x', Code.Unauthenticated))).toBe('signed-out');
    expect(classifyLiveError(new ConnectError('x', Code.InvalidArgument))).toBe('invalid');
    expect(classifyLiveError(new ConnectError('x', Code.PermissionDenied))).toBe('forbidden');
    expect(classifyLiveError(blocked(GameSessionBlockedReason.NO_OPEN_SESSION))).toBe('no-session');
  });

  it('reads a request over its size limit as invalid, whatever its code', () => {
    expect(classifyLiveError(tooLong())).toBe('invalid');
  });

  it('treats everything else as worth another try', () => {
    expect(classifyLiveError(new ConnectError('x', Code.Unavailable))).toBe('transient');
    expect(classifyLiveError(new ConnectError('x', Code.Aborted))).toBe('transient');
    expect(classifyLiveError(new TypeError('Failed to fetch'))).toBe('transient');
    expect(classifyLiveError(blocked(GameSessionBlockedReason.SESSION_ALREADY_OPEN))).toBe(
      'transient',
    );
    expect(classifyLiveError(new ConnectError('no open session', Code.FailedPrecondition))).toBe(
      'transient',
    );
  });
});

describe('toVitalsVm', () => {
  it('maps the wire vitals, hit dice by size as "2d10 e 1d8" with what is spent of each', () => {
    const vm = toVitalsVm(
      create(CharacterVitalsSchema, {
        characterId: 'c1',
        name: 'Toren',
        hitPointsCurrent: 31,
        hitPointsMax: 31,
        hitDice: [
          { faces: 10, count: 2 },
          { faces: 8, count: 1 },
        ],
        hitDiceTotal: 3,
        hitDiceUsed: 1,
        hitDiceUsedByDie: { 8: 1 },
        spellSlots: [{ level: 1, total: 2, used: 1 }],
        pactSlots: { slotLevel: 2, total: 2, used: 0 },
        revision: 7,
      }),
    );
    expect(vm.hitDice).toBe('2d10 e 1d8');
    expect(vm.hitDiceSizes).toEqual([
      { faces: 10, total: 2, used: 0 },
      { faces: 8, total: 1, used: 1 },
    ]);
    expect(vm.hitDiceTotal).toBe(3);
    expect(vm.hitDiceUsed).toBe(1);
    expect(vm.spellSlots).toEqual([{ level: 1, total: 2, used: 1, created: 0 }]);
    expect(vm.pactSlots).toEqual({ slotLevel: 2, total: 2, used: 0 });
    expect(vm.revision).toBe(7);
  });

  it('keeps what Ajuda adds to the maximum, which the maximum already counts', () => {
    const vm = toVitalsVm(
      create(CharacterVitalsSchema, {
        characterId: 'c1',
        hitPointsCurrent: 43,
        hitPointsMax: 43,
        hitPointsMaxBonus: 5,
      }),
    );
    expect(vm.hitPointsMaxBonus).toBe(5);
    expect(vm.hitPointsMax).toBe(43);
  });
});

describe('toVitalsVm and the resources', () => {
  it('maps the slots Flexible Casting created and when each resource comes back', () => {
    const vm = toVitalsVm(
      create(CharacterVitalsSchema, {
        characterId: 'c1',
        spellSlots: [{ level: 2, total: 3, used: 0, created: 1 }],
        resources: [
          { key: 'ki', namePt: 'Chi', total: 5, used: 2, recharge: Recharge.SHORT_REST },
          { key: 'rage', namePt: 'Fúria', total: 3, used: 0, recharge: Recharge.LONG_REST },
          { key: 'odd', namePt: 'Outro', total: 1, used: 0, recharge: Recharge.UNSPECIFIED },
        ],
      }),
    );
    expect(vm.spellSlots[0].created).toBe(1);
    expect(vm.resources?.map((r) => [r.key, r.recharge])).toEqual([
      ['ki', 'short_rest'],
      ['rage', 'long_rest'],
      ['odd', 'none'],
    ]);
  });

  it('maps the three recharge variants of a table resource', () => {
    const vm = toVitalsVm(
      create(CharacterVitalsSchema, {
        characterId: 'c1',
        resources: [
          { key: 'a', total: 2, recharge: Recharge.INITIATIVE },
          { key: 'b', total: 2, recharge: Recharge.RESTORE_ONE_ON_SHORT_REST },
          { key: 'c', total: 2, recharge: Recharge.RESTORE_ONE_ON_LONG_REST },
        ],
      }),
    );
    expect(vm.resources?.map((r) => r.recharge)).toEqual([
      'initiative',
      'restore_one_on_short_rest',
      'restore_one_on_long_rest',
    ]);
  });
});

describe('toShownImageVm', () => {
  it('maps the wire image, and nothing when no image is shown', () => {
    expect(
      toShownImageVm({
        $typeName: 'meurpg.play.v1.ShownImage',
        id: 'img-1',
        name: 'Capitão Goblin',
        width: 400,
        height: 500,
        url: '/images/img-1',
        thumbnailUrl: '/images/img-1/thumb',
      }),
    ).toEqual({
      id: 'img-1',
      name: 'Capitão Goblin',
      width: 400,
      height: 500,
      url: '/images/img-1',
    });
    expect(toShownImageVm(undefined)).toBeNull();
  });
});

describe('LiveSessionSourceLive.watch', () => {
  /** A transport whose one stream answers `responses`, so the mapping is tested through the real client. */
  function sourceAnswering(
    responses: ReturnType<typeof create<typeof WatchGameSessionResponseSchema>>[],
  ) {
    const transport = {
      stream: async () => ({
        stream: true,
        header: new Headers(),
        trailer: new Headers(),
        message: (async function* () {
          yield* responses;
        })(),
      }),
    } as unknown as Transport;
    TestBed.configureTestingModule({
      providers: [LiveSessionSourceLive, { provide: CONNECT_TRANSPORT, useValue: transport }],
    });
    return TestBed.inject(LiveSessionSourceLive);
  }

  async function events(responses: Parameters<typeof sourceAnswering>[0]) {
    const out = [];
    for await (const e of sourceAnswering(responses).watch(
      'camp-1',
      new AbortController().signal,
    )) {
      out.push(e.kind);
    }
    return out;
  }

  it('maps `xp_changed` to its own event (MR-016)', async () => {
    expect(
      await events([
        create(WatchGameSessionResponseSchema, { event: { case: 'ready', value: {} } }),
        create(WatchGameSessionResponseSchema, { event: { case: 'xpChanged', value: {} } }),
      ]),
    ).toEqual(['ready', 'xpChanged']);
  });

  it('maps `creatures_changed` to its own event (MR-037)', async () => {
    expect(
      await events([
        create(WatchGameSessionResponseSchema, { event: { case: 'ready', value: {} } }),
        create(WatchGameSessionResponseSchema, { event: { case: 'creaturesChanged', value: {} } }),
      ]),
    ).toEqual(['ready', 'creaturesChanged']);
  });

  it('maps `spell_casts_changed` to its own event, with no content (MR-048)', async () => {
    expect(
      await events([
        create(WatchGameSessionResponseSchema, { event: { case: 'ready', value: {} } }),
        create(WatchGameSessionResponseSchema, { event: { case: 'spellCastsChanged', value: {} } }),
      ]),
    ).toEqual(['ready', 'spellCastsChanged']);
  });

  it('maps `group_check_changed` to its own event, with no content: the roll and the DC are read with GetGroupCheck (W7-X)', async () => {
    expect(
      await events([
        create(WatchGameSessionResponseSchema, { event: { case: 'ready', value: {} } }),
        create(WatchGameSessionResponseSchema, { event: { case: 'groupCheckChanged', value: {} } }),
      ]),
    ).toEqual(['ready', 'groupCheckChanged']);
  });

  it('maps `held_rolls_changed` to its own event, with no content: the questions are read with ListHeldRolls', async () => {
    expect(
      await events([
        create(WatchGameSessionResponseSchema, { event: { case: 'ready', value: {} } }),
        create(WatchGameSessionResponseSchema, { event: { case: 'heldRollsChanged', value: {} } }),
      ]),
    ).toEqual(['ready', 'heldRollsChanged']);
  });

  it('maps `free_roll_rolled` to its own event with the whole roll, ready to draw and to log (MR-052)', async () => {
    const out = [];
    const responses = [
      create(WatchGameSessionResponseSchema, {
        event: {
          case: 'freeRollRolled',
          value: {
            roll: create(FreeRollSchema, {
              id: 'r-1',
              rollerName: 'Brisa',
              sides: 6,
              count: 2,
              modifier: 3,
              dice: [
                create(FreeRollDieSchema, { face: 4 }),
                create(FreeRollDieSchema, { face: 4 }),
              ],
              total: 11,
              formula: '2d6 + 3',
              logText: 'Rolagem livre · Brisa: 2d6 + 3 = 11 (4, 4)',
            }),
          },
        },
      }),
      // A message with no roll tells nothing.
      create(WatchGameSessionResponseSchema, { event: { case: 'freeRollRolled', value: {} } }),
    ];
    for await (const e of sourceAnswering(responses).watch(
      'camp-1',
      new AbortController().signal,
    )) {
      out.push(e);
    }
    expect(out).toHaveLength(1);
    expect(out[0]).toMatchObject({
      kind: 'freeRollRolled',
      roll: { id: 'r-1', rollerName: 'Brisa', total: 11, formula: '2d6 + 3', mode: 'normal' },
    });
  });

  it("maps `puzzle_changed` to its own event, with the puzzle's ID and nothing else (MR-038)", async () => {
    const out = [];
    const responses = [
      create(WatchGameSessionResponseSchema, { event: { case: 'ready', value: {} } }),
      create(WatchGameSessionResponseSchema, {
        event: { case: 'puzzleChanged', value: { puzzleId: 'p-1' } },
      }),
    ];
    for await (const e of sourceAnswering(responses).watch(
      'camp-1',
      new AbortController().signal,
    )) {
      out.push(e);
    }
    expect(out).toEqual([{ kind: 'ready' }, { kind: 'puzzleChanged', puzzleId: 'p-1' }]);
  });

  it('maps `reaction_window_opened` and `reaction_window_closed` to their own events (PM-04)', async () => {
    const out = [];
    const responses = [
      create(WatchGameSessionResponseSchema, { event: { case: 'ready', value: {} } }),
      create(WatchGameSessionResponseSchema, {
        event: { case: 'reactionWindowOpened', value: { encounterId: 'e1', windowId: 'w1' } },
      }),
      create(WatchGameSessionResponseSchema, {
        event: {
          case: 'reactionWindowClosed',
          value: {
            encounterId: 'e1',
            windowId: 'w1',
            closedByItself: true,
            textPt: 'Repreensão Infernal fechou. Você está inconsciente e não pode reagir.',
          },
        },
      }),
    ];
    for await (const e of sourceAnswering(responses).watch(
      'camp-1',
      new AbortController().signal,
    )) {
      out.push(e);
    }
    expect(out).toEqual([
      { kind: 'ready' },
      { kind: 'reactionWindowOpened', encounterId: 'e1', windowId: 'w1' },
      {
        kind: 'reactionWindowClosed',
        encounterId: 'e1',
        windowId: 'w1',
        closedByItself: true,
        text: 'Repreensão Infernal fechou. Você está inconsciente e não pode reagir.',
      },
    ]);
  });

  it('maps `content_changed` to its own event, with no content (RN-23, RN-10)', async () => {
    expect(
      await events([
        create(WatchGameSessionResponseSchema, { event: { case: 'ready', value: {} } }),
        create(WatchGameSessionResponseSchema, { event: { case: 'contentChanged', value: {} } }),
      ]),
    ).toEqual(['ready', 'contentChanged']);
  });

  it('maps the two hints of a character to one event with the character, and the revival to its own', async () => {
    expect(
      await events([
        create(WatchGameSessionResponseSchema, {
          event: { case: 'characterChangesRequested', value: { characterId: 'c-1' } },
        }),
        create(WatchGameSessionResponseSchema, {
          event: { case: 'characterResubmitted', value: { characterId: 'c-2' } },
        }),
        create(WatchGameSessionResponseSchema, {
          event: { case: 'characterRevived', value: { characterId: 'c-3' } },
        }),
      ]),
    ).toEqual(['characterChanged', 'characterChanged', 'characterRevived']);
  });

  it('maps `calling_you` to its own event, with no content (RN-10)', async () => {
    expect(
      await events([
        create(WatchGameSessionResponseSchema, { event: { case: 'ready', value: {} } }),
        create(WatchGameSessionResponseSchema, { event: { case: 'callingYou', value: {} } }),
      ]),
    ).toEqual(['ready', 'callingYou']);
  });

  it('maps the conversation: a line with its content, and a typing hint with only its seconds', async () => {
    const got = await eventsOf(
      {
        case: 'sessionMessageAdded',
        value: { message: { id: 'm1', text: 'Oi', forEveryone: true } },
      },
      { case: 'sessionMessageAdded', value: {} },
      { case: 'masterTyping', value: { expiresInSeconds: 6 } },
    );
    expect(got).toEqual([
      expect.objectContaining({
        kind: 'sessionMessage',
        message: expect.objectContaining({ id: 'm1', text: 'Oi' }),
      }),
      // A line with no body says nothing.
      { kind: 'masterTyping', expiresInSeconds: 6 },
    ]);
  });

  it('maps `attack_hit` with who hit, how much and of what type', async () => {
    expect(
      await eventsOf({
        case: 'attackHit',
        value: { id: 'p1', actorLabel: '', amount: 5, damageTypePt: 'cortante' },
      }),
    ).toEqual([
      { kind: 'attackHit', id: 'p1', actorLabel: '', amount: 5, damageTypePt: 'cortante' },
    ]);
  });

  it('maps `revivify_changed` to its own event, with no content (RN-10)', async () => {
    expect(
      await events([
        create(WatchGameSessionResponseSchema, { event: { case: 'revivifyChanged', value: {} } }),
      ]),
    ).toEqual(['revivifyChanged']);
  });

  /** Every event of the wire, each with its content, through the real client: the table of events stays whole. */
  async function eventsOf(...wire: { case: string; value?: object }[]) {
    const responses = wire.map((event) =>
      create(WatchGameSessionResponseSchema, { event } as unknown as Parameters<
        typeof create<typeof WatchGameSessionResponseSchema>
      >[1]),
    );
    const out = [];
    for await (const e of sourceAnswering(responses).watch(
      'camp-1',
      new AbortController().signal,
    )) {
      out.push(e);
    }
    return out;
  }

  it('maps the map, vision, image and session events with their content', async () => {
    expect(
      await eventsOf(
        { case: 'heartbeat', value: {} },
        { case: 'sessionEnded', value: {} },
        { case: 'currentMapChanged', value: { mapId: 'm1' } },
        { case: 'currentMapChanged', value: { mapId: '' } },
        { case: 'mapChanged', value: { mapId: 'm2' } },
        {
          case: 'tokenMoved',
          value: { mapId: 'm2', characterId: 'c1', xBp: 120, yBp: 340 },
        },
        { case: 'visionChanged', value: { mapId: 'm2' } },
        {
          case: 'shownImageChanged',
          value: { image: { id: 'i1', name: 'Mapa', width: 10, height: 20, url: '/i1' } },
        },
        { case: 'shownImageChanged', value: {} },
        { case: 'leftImagesChanged', value: {} },
        { case: 'trapNoticed', value: { mapId: 'm2', pointId: 'p1' } },
      ),
    ).toEqual([
      { kind: 'heartbeat' },
      { kind: 'ended' },
      { kind: 'currentMap', mapId: 'm1' },
      { kind: 'currentMap', mapId: null },
      { kind: 'mapChanged', mapId: 'm2' },
      { kind: 'tokenMoved', mapId: 'm2', characterId: 'c1', xBp: 120, yBp: 340 },
      { kind: 'visionChanged', mapId: 'm2' },
      { kind: 'shownImage', image: { id: 'i1', name: 'Mapa', width: 10, height: 20, url: '/i1' } },
      { kind: 'shownImage', image: null },
      { kind: 'leftImages' },
      { kind: 'trapNoticed', mapId: 'm2', pointId: 'p1' },
    ]);
  });

  it('maps the combat, scene and notes events with their content, and drops vitals without a body', async () => {
    expect(
      await eventsOf(
        { case: 'vitalsChanged', value: {} },
        { case: 'vitalsChanged', value: { vitals: { characterId: 'c1', name: 'Toren' } } },
        { case: 'encounterChanged', value: { encounterId: 'e1', revision: 4, mode: 2 } },
        {
          case: 'turnChanged',
          value: {
            encounterId: 'e1',
            round: 3,
            currentCombatantId: 'k1',
            masterTurn: true,
            revision: 5,
          },
        },
        { case: 'combatantMoved', value: { encounterId: 'e1', combatantId: 'k1', col: 2, row: 7 } },
        { case: 'combatLogChanged', value: {} },
        { case: 'sceneChanged', value: {} },
        { case: 'sceneCheckRolled', value: {} },
        { case: 'notesChanged', value: {} },
        { case: 'stageChanged', value: {} },
      ),
    ).toEqual([
      expect.objectContaining({
        kind: 'vitals',
        vitals: expect.objectContaining({ name: 'Toren' }),
      }),
      { kind: 'encounterChanged', encounterId: 'e1', revision: 4, mode: 2 },
      {
        kind: 'turnChanged',
        encounterId: 'e1',
        round: 3,
        currentCombatantId: 'k1',
        masterTurn: true,
        revision: 5,
      },
      { kind: 'combatantMoved', encounterId: 'e1', combatantId: 'k1', col: 2, row: 7 },
      { kind: 'combatLogChanged' },
      { kind: 'sceneChanged' },
      { kind: 'sceneCheckRolled' },
      { kind: 'notesChanged' },
      { kind: 'stageChanged' },
    ]);
  });

  it('still takes an event it does not know as a sign the stream is alive', async () => {
    // An empty `event` is what a newer server's oneof case looks like to this app.
    expect(await events([create(WatchGameSessionResponseSchema, {})])).toEqual(['heartbeat']);
  });
});

describe('LiveSessionSourceLive.getPlayerSheet', () => {
  function sheetWith(spells: { key: string; prepared: boolean }[]) {
    const characters = {
      getCharacter: async () => ({
        character: {
          derived: {
            classes: [{ namePt: 'Clérigo', level: 5 }],
            subraceNamePt: '',
            raceNamePt: 'Humano',
            skills: [],
            senses: [],
            features: [],
            spells: spells.map((s) => ({ spell: { key: s.key }, prepared: s.prepared })),
          },
        },
      }),
    };
    TestBed.configureTestingModule({
      providers: [LiveSessionSourceLive, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const source = TestBed.inject(LiveSessionSourceLive);
    (source as unknown as { characters: unknown }).characters = characters;
    return source;
  }

  it('says the character has Revivificar only when it is ready today, and gives the class line', async () => {
    const ready = await sheetWith([{ key: 'spell:revivify', prepared: true }]).getPlayerSheet(
      'c',
      'x',
    );
    expect(ready.revivify).toBe(true);
    expect(ready.classes).toBe('Clérigo 5');
    expect(ready.summary).toBe('Clérigo 5, Humano');
  });

  it('does not offer Revivificar for a spell on the sheet that is not prepared, nor for other spells', async () => {
    const notReady = await sheetWith([{ key: 'spell:revivify', prepared: false }]).getPlayerSheet(
      'c',
      'x',
    );
    expect(notReady.revivify).toBe(false);
    TestBed.resetTestingModule();
    const other = await sheetWith([{ key: 'spell:bless', prepared: true }]).getPlayerSheet(
      'c',
      'x',
    );
    expect(other.revivify).toBe(false);
  });
});

describe('LiveSessionSourceLive.getPlayerSheet (the skills a search and a check read)', () => {
  const skill = (key: string, namePt: string, bonus: number, proficiency: ProficiencyLevel) => ({
    key,
    namePt,
    bonus,
    proficiency,
  });

  function sourceWith(features: string[]) {
    const transport = {
      unary: async (method: { name: string }) => ({
        stream: false,
        service: {},
        method,
        header: new Headers(),
        trailer: new Headers(),
        message: create(GetCharacterResponseSchema, {
          character: {
            derived: {
              armorClass: 13,
              features: features.map((key) => ({ key })),
              skills: [
                skill('skill:perception', 'Percepção', 1, ProficiencyLevel.NONE),
                skill('skill:investigation', 'Investigação', 8, ProficiencyLevel.PROFICIENT),
                skill('skill:arcana', 'Arcanismo', 8, ProficiencyLevel.PROFICIENT),
                skill('skill:acrobatics', 'Acrobacia', 1, ProficiencyLevel.NONE),
                skill('skill:athletics', 'Atletismo', -1, ProficiencyLevel.NONE),
                skill('skill:stealth', 'Furtividade', 5, ProficiencyLevel.EXPERTISE),
              ],
            },
          },
        }),
      }),
    } as unknown as Transport;
    TestBed.configureTestingModule({
      providers: [LiveSessionSourceLive, { provide: CONNECT_TRANSPORT, useValue: transport }],
    });
    return TestBed.inject(LiveSessionSourceLive);
  }

  it('lists the other skills with the sheet bonus, alphabetical, without Percepção and Investigação', async () => {
    const sheet = await sourceWith([]).getPlayerSheet('camp', 'c1');
    expect(sheet.skills?.perception).toBe(1);
    expect(sheet.skills?.investigation).toBe(8);
    expect(sheet.skills?.others).toEqual([
      { key: 'skill:acrobatics', name: 'Acrobacia', bonus: 1 },
      { key: 'skill:arcana', name: 'Arcanismo', bonus: 8 },
      { key: 'skill:athletics', name: 'Atletismo', bonus: -1 },
      { key: 'skill:stealth', name: 'Furtividade', bonus: 5 },
    ]);
  });

  it('names the skills Talento Confiável raises (the proficient ones) only for a sheet that has the feature', async () => {
    expect((await sourceWith([]).getPlayerSheet('camp', 'c1')).skills?.reliableTalent).toEqual([]);
    TestBed.resetTestingModule();
    const rogue = await sourceWith(['feature:reliable-talent']).getPlayerSheet('camp', 'c1');
    expect(rogue.skills?.reliableTalent).toEqual([
      'skill:investigation',
      'skill:arcana',
      'skill:stealth',
    ]);
  });
});
