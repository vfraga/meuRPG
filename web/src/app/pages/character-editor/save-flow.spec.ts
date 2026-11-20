import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';
import { vi } from 'vitest';

import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
  InvalidFieldSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type {
  BasicCharacterFormValue,
  CharacterFormValue,
  CreateCharacterInput,
  UpdateCharacterInput,
} from './character-editor.types';
import type { ReadyState } from './editor-state';
import {
  SaveFlow,
  type SaveFlowContext,
  afterCreateTarget,
  classBlockOfRefusal,
  createInputOf,
} from './save-flow';

const FULL = { name: 'Tharn' } as CharacterFormValue;
const BASIC = { name: 'Lacaio' } as BasicCharacterFormValue;

const ready = (over: Partial<ReadyState> = {}): ReadyState => ({
  status: 'ready',
  mode: 'create',
  kind: 'player',
  campaignId: 'camp',
  characterId: null,
  revision: 3,
  catalog: {} as never,
  xpLocked: false,
  reserved: false,
  ...over,
});

const invalidField = (field: string) =>
  new ConnectError('x', Code.InvalidArgument, undefined, [
    { desc: InvalidFieldSchema, value: create(InvalidFieldSchema, { field }) },
  ]);

const switchedOff = (key: string) =>
  new ConnectError('x', Code.FailedPrecondition, undefined, [
    {
      desc: CharacterBlockedSchema,
      value: create(CharacterBlockedSchema, {
        reason: CharacterBlockedReason.SWITCHED_OFF_CONTENT,
        contentKey: key,
      }),
    },
  ]);

describe('classBlockOfRefusal', () => {
  it('reads the index of the class block a refusal points at', () => {
    expect(classBlockOfRefusal(invalidField('full.classes[2].level'))).toBe(2);
  });

  it('is null for a field outside the class blocks, a refusal without a field and any other error', () => {
    expect(classBlockOfRefusal(invalidField('full.name'))).toBeNull();
    expect(classBlockOfRefusal(new ConnectError('x', Code.InvalidArgument))).toBeNull();
    expect(classBlockOfRefusal(new Error('x'))).toBeNull();
  });
});

describe('createInputOf', () => {
  it('carries the campaign, the kind and the sheet of the form that is saved', () => {
    expect(createInputOf(ready(), { full: FULL, basic: null, abilityMethod: null })).toEqual({
      campaignId: 'camp',
      kind: 'player',
      full: FULL,
      basic: null,
    });
  });

  it('adds the way the scores were made to a full sheet only, and the reserved mark when it is', () => {
    expect(
      createInputOf(ready({ reserved: true }), {
        full: FULL,
        basic: null,
        abilityMethod: 'point_buy',
      }),
    ).toMatchObject({
      abilityMethod: 'point_buy',
      forPlayer: true,
    });
    expect(
      createInputOf(ready({ kind: 'minion' }), {
        full: null,
        basic: BASIC,
        abilityMethod: 'point_buy',
      }),
    ).not.toHaveProperty('abilityMethod');
  });
});

describe('afterCreateTarget', () => {
  it('sends a reserved character to the list and any other to its sheet', () => {
    expect(afterCreateTarget(ready({ reserved: true }), 'new')).toEqual(['/campaigns', 'camp']);
    expect(afterCreateTarget(ready(), 'new')).toEqual(['/campaigns', 'camp', 'characters', 'new']);
  });
});

describe('SaveFlow', () => {
  let created: CreateCharacterInput[];
  let updated: UpdateCharacterInput[];
  let navigated: string[][];
  let here: boolean;
  let createResult: () => Promise<{ characterId: string }>;
  let updateResult: () => Promise<{ revision: number }>;
  let switchedOffFields: Set<string>;
  let refreshes: number;

  function flow(over: Partial<SaveFlowContext> = {}): SaveFlow {
    return new SaveFlow({
      source: {
        createCharacter: (input) => {
          created.push(input);
          return createResult();
        },
        updateCharacter: (input) => {
          updated.push(input);
          return updateResult();
        },
      },
      navigate: (commands) => {
        navigated.push(commands);
        return Promise.resolve(true);
      },
      full: () => FULL,
      basic: () => BASIC,
      name: (isBasic) => (isBasic ? 'Lacaio' : 'Tharn'),
      abilityMethod: () => null,
      stillHere: () => () => here,
      nameOfKey: (key) => (key === 'race:gnome' ? 'Gnomo' : undefined),
      markSwitchedOff: (key) => switchedOffFields.has(key),
      refreshCatalog: () => {
        refreshes++;
        return Promise.resolve();
      },
      ...over,
    });
  }

  beforeEach(() => {
    created = [];
    updated = [];
    navigated = [];
    here = true;
    createResult = () => Promise.resolve({ characterId: 'new' });
    updateResult = () => Promise.resolve({ revision: 4 });
    switchedOffFields = new Set();
    refreshes = 0;
  });

  it('creates, goes to the new sheet and is idle again', async () => {
    const save = flow();
    await save.run(ready(), false);
    expect(created).toHaveLength(1);
    expect(created[0]).toMatchObject({ campaignId: 'camp', full: FULL, basic: null });
    expect(navigated).toEqual([['/campaigns', 'camp', 'characters', 'new']]);
    expect(save.state()).toEqual({ status: 'idle' });
  });

  it('sends the same key for the same sheet and a new one once it worked', async () => {
    const save = flow();
    createResult = () => Promise.reject(new Error('lost answer'));
    await save.run(ready(), false);
    await save.run(ready(), false);
    expect(created[0].idempotencyKey).toBe(created[1].idempotencyKey);
    createResult = () => Promise.resolve({ characterId: 'new' });
    await save.run(ready(), false);
    await save.run(ready(), false);
    expect(created[3].idempotencyKey).not.toBe(created[2].idempotencyKey);
  });

  it('saves a short sheet from its own form', async () => {
    await flow().run(ready({ kind: 'minion' }), true);
    expect(created[0]).toMatchObject({ full: null, basic: BASIC });
  });

  it('updates with the revision it loaded and goes back to the sheet', async () => {
    const save = flow();
    await save.run(ready({ mode: 'edit', characterId: 'char' }), false);
    expect(updated).toEqual([
      {
        campaignId: 'camp',
        characterId: 'char',
        revision: 3,
        name: 'Tharn',
        full: FULL,
        basic: null,
      },
    ]);
    expect(navigated).toEqual([['/campaigns', 'camp', 'characters', 'char']]);
    await save.run(ready({ mode: 'edit', characterId: 'char', kind: 'minion' }), true);
    expect(updated[1]).toMatchObject({ name: 'Lacaio', full: null, basic: BASIC });
  });

  it('keeps the server sentence for the Perícias step, opens it, says only where to look above the buttons, and clears together', async () => {
    const opened: string[] = [];
    const sentence =
      'Não há vaga para a perícia Furtividade: as listas que a oferecem já estão preenchidas pelas outras perícias marcadas. Desmarque uma delas para marcar Furtividade.';
    createResult = () =>
      Promise.reject(
        new ConnectError(
          `sheet.full.skill_proficiency_keys[2] ${sentence}`,
          Code.InvalidArgument,
          undefined,
          [
            {
              desc: InvalidFieldSchema,
              value: create(InvalidFieldSchema, { field: 'sheet.full.skill_proficiency_keys[2]' }),
            },
          ],
        ),
      );
    const save = flow({
      openStep: (step) => opened.push(step),
      full: () =>
        ({
          ...FULL,
          skillProficiencies: ['skill:arcana', 'skill:history', 'skill:stealth'],
        }) as CharacterFormValue,
    });
    await save.run(ready(), false);
    expect(save.pickRefusal()).toEqual({ kind: 'skill', key: 'skill:stealth', sentence });
    expect(opened).toEqual(['pericias']);
    expect(save.state()).toEqual({ status: 'error', message: 'Veja o passo Perícias.' });
    save.clearPickProblem();
    expect(save.pickRefusal()).toBeNull();
    expect(save.state()).toEqual({ status: 'idle' });
    // A save that is refused another way is not cleared by it.
    createResult = () => Promise.reject(new Error('down'));
    await save.run(ready(), false);
    save.clearPickProblem();
    expect(save.state().status).toBe('error');
  });

  it('keeps the server sentence for a tool on its key, opens the Escolhas step and names it', async () => {
    const opened: string[] = [];
    const sentence =
      'A ferramenta Kit de herbalismo não está em nenhuma das escolhas de ferramenta do personagem.';
    createResult = () =>
      Promise.reject(
        new ConnectError(
          `sheet.full.feature_choice_keys[1] ${sentence}`,
          Code.InvalidArgument,
          undefined,
          [
            {
              desc: InvalidFieldSchema,
              value: create(InvalidFieldSchema, { field: 'sheet.full.feature_choice_keys[1]' }),
            },
          ],
        ),
      );
    const save = flow({
      openStep: (step) => opened.push(step),
      full: () =>
        ({
          ...FULL,
          featureChoiceKeys: ['fighting-style:archery', 'tool:herbalism-kit'],
        }) as CharacterFormValue,
    });
    await save.run(ready(), false);
    expect(save.pickRefusal()).toEqual({ kind: 'tool', key: 'tool:herbalism-kit', sentence });
    expect(opened).toEqual(['escolhas']);
    expect(save.state()).toEqual({ status: 'error', message: 'Veja o passo Escolhas.' });
  });

  it('saves nothing for an edit with no character, and is idle', async () => {
    const save = flow();
    await save.run(ready({ mode: 'edit', characterId: null }), false);
    expect(updated).toEqual([]);
    expect(navigated).toEqual([]);
    expect(save.state()).toEqual({ status: 'idle' });
  });

  it('is saving while the answer is on the way', async () => {
    let answer: (v: { characterId: string }) => void = () => undefined;
    createResult = () => new Promise((resolve) => (answer = resolve));
    const save = flow();
    const done = save.run(ready(), false);
    expect(save.state()).toEqual({ status: 'saving' });
    answer({ characterId: 'new' });
    await done;
    expect(save.state()).toEqual({ status: 'idle' });
  });

  it('does not navigate, nor change the state, when the person left meanwhile', async () => {
    here = false;
    const save = flow();
    await save.run(ready(), false);
    expect(created).toHaveLength(1);
    expect(navigated).toEqual([]);
    expect(save.state()).toEqual({ status: 'saving' });
    await save.run(ready({ mode: 'edit', characterId: 'char' }), false);
    expect(navigated).toEqual([]);
    createResult = () => Promise.reject(new Error('x'));
    await save.run(ready(), false);
    expect(save.state()).toEqual({ status: 'saving' });
  });

  it('says what the server refused, by name, and marks the class block it points at', async () => {
    const save = flow();
    createResult = () => Promise.reject(invalidField('full.classes[1].level'));
    await save.run(ready(), false);
    expect(save.classProblem()).toBe(1);
    expect(save.state().status).toBe('error');
    createResult = () => Promise.reject(invalidField('full.name'));
    await save.run(ready(), false);
    expect(save.classProblem()).toBeNull();
  });

  it('puts a switched-off option on its field and reads the lists again, without a notice', async () => {
    switchedOffFields.add('race:gnome');
    const save = flow();
    createResult = () => Promise.reject(switchedOff('race:gnome'));
    await save.run(ready(), false);
    expect(refreshes).toBe(1);
    expect(save.state()).toEqual({ status: 'saving' });
  });

  it('says a switched-off option no field holds in the notice, by its name', async () => {
    const save = flow({ nameOfKey: () => 'Escudo Arcano' });
    createResult = () => Promise.reject(switchedOff('spell:shield'));
    await save.run(ready(), false);
    expect(refreshes).toBe(0);
    expect(save.state()).toMatchObject({
      status: 'error',
      message: expect.stringContaining('Escudo Arcano'),
    });
  });

  it('starts over when the page is reused', async () => {
    const save = flow();
    createResult = () => Promise.reject(invalidField('full.classes[0].level'));
    await save.run(ready(), false);
    save.reset();
    expect(save.classProblem()).toBeNull();
    expect(save.state()).toEqual({ status: 'idle' });
    void vi;
  });
});
