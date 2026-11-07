import { Code, ConnectError } from '@connectrpc/connect';

import type { AbilityTableVm, CharacterForEdit, RulesCatalogVm } from './character-editor.types';
import {
  type PageState,
  type ReadyState,
  actionsNoteOf,
  cancelLinkOf,
  createFailed,
  createLabelOf,
  editBlocked,
  editFailed,
  leadPartsOf,
  loadingState,
  pageTitleOf,
  phoneNoteOf,
  previewInputOf,
  readyForCreate,
  readyForEdit,
  startingXp,
  tableHitPointsMethod,
  titleFor,
} from './editor-state';

const catalog = (over: Partial<RulesCatalogVm> = {}): RulesCatalogVm => ({
  races: [],
  classes: [],
  backgrounds: [],
  skills: [],
  armor: [],
  weapons: [],
  spells: [],
  viewerIsMaster: false,
  toolsAndLanguages: [],
  challengeRatings: [],
  ...over,
});

const ready = (over: Partial<ReadyState> = {}): ReadyState => ({
  status: 'ready',
  mode: 'create',
  kind: 'player',
  campaignId: 'camp',
  characterId: null,
  revision: 1,
  catalog: catalog(),
  xpLocked: false,
  reserved: false,
  ...over,
});

const existing = (over: Partial<CharacterForEdit> = {}): CharacterForEdit => ({
  kind: 'enemy',
  revision: 6,
  blocked: null,
  sheetLocked: true,
  full: null,
  basic: null,
  ...over,
});

describe('titleFor', () => {
  it('names an edit, a reserved character, a player and an NPC', () => {
    expect(titleFor('edit', 'player')).toBe('Editar ficha');
    expect(titleFor('create', 'player', true)).toBe('Criar personagem para um jogador');
    expect(titleFor('create', 'player')).toBe('Criar personagem');
    expect(titleFor('create', 'boss')).toBe('Criar NPC');
  });
});

describe('the state of the page', () => {
  it('starts loading with the title the route already tells', () => {
    expect(loadingState('create', 'story')).toEqual({ status: 'loading', title: 'Criar NPC' });
  });

  it('opens a new sheet at revision 1, unlocked', () => {
    const lists = catalog();
    expect(readyForCreate('player', 'camp', lists, true)).toEqual({
      status: 'ready',
      mode: 'create',
      kind: 'player',
      campaignId: 'camp',
      characterId: null,
      revision: 1,
      catalog: lists,
      xpLocked: false,
      reserved: true,
    });
  });

  it('opens an existing sheet with its kind, revision and lock', () => {
    const lists = catalog();
    expect(readyForEdit(existing({ reserved: true }), 'camp', 'char', lists)).toMatchObject({
      mode: 'edit',
      kind: 'enemy',
      characterId: 'char',
      revision: 6,
      xpLocked: true,
      reserved: true,
    });
    expect(readyForEdit(existing(), 'camp', 'char', lists).reserved).toBe(false);
  });

  it('sends a failed read back to the campaign, or to the sheet', () => {
    const err = new ConnectError('x', Code.NotFound);
    expect(createFailed(err, 'camp')).toEqual({
      status: 'error',
      message: 'Personagem não encontrado.',
      backLink: ['/campaigns', 'camp'],
      backLabel: 'Voltar para a campanha',
    });
    expect(editFailed(err, 'camp', 'char')).toEqual({
      status: 'error',
      message: 'Personagem não encontrado.',
      backLink: ['/campaigns', 'camp', 'characters', 'char'],
      backLabel: 'Voltar para a ficha',
    });
  });

  it('names a blocked sheet by why it is blocked, not as an error', () => {
    expect(editBlocked('character_dead', 'camp', 'char')).toMatchObject({
      blocked: { title: 'Personagem morto' },
      message: expect.stringContaining('morto'),
    });
    expect(editBlocked('sheet_locked', 'camp', 'char').blocked).toEqual({ title: 'Ficha travada' });
  });
});

describe('the texts of the page', () => {
  const loading: PageState = { status: 'loading', title: 'Ficha' };
  const failed: PageState = createFailed(new Error('x'), 'camp');

  it('titles the page by its state', () => {
    expect(pageTitleOf(loading)).toBe('Ficha');
    expect(pageTitleOf(failed)).toBe('Não foi possível abrir o formulário');
    expect(pageTitleOf(editBlocked('sheet_locked', 'c', 'd'))).toBe('Ficha travada');
    expect(pageTitleOf(ready({ mode: 'edit' }))).toBe('Editar ficha');
  });

  it('labels the create button, "Salvar como reservado" for a reserved character', () => {
    expect(createLabelOf(ready({ reserved: true }))).toBe('Salvar como reservado');
    expect(createLabelOf(ready())).toBe('Criar personagem');
    expect(createLabelOf(loading)).toBe('Ficha');
  });

  it('cancels to the sheet being edited, the campaign, or the home page', () => {
    expect(cancelLinkOf(loading)).toEqual(['/']);
    expect(cancelLinkOf(ready())).toEqual(['/campaigns', 'camp']);
    expect(cancelLinkOf(ready({ mode: 'edit', characterId: 'char' }))).toEqual([
      '/campaigns',
      'camp',
      'characters',
      'char',
    ]);
  });

  it('says what the buttons do for each mode and size of sheet', () => {
    expect(actionsNoteOf(loading)).toBe('');
    expect(actionsNoteOf(ready())).toContain('Cria com o que estiver preenchido');
    expect(actionsNoteOf(ready({ kind: 'minion' }))).toBe(
      'Cancelar volta para a campanha sem criar.',
    );
    expect(actionsNoteOf(ready({ mode: 'edit' }))).toContain('Salva todos os passos');
    expect(actionsNoteOf(ready({ mode: 'edit', kind: 'story' }))).toBe(
      'Cancelar volta para a ficha sem salvar.',
    );
  });

  const leadText = (s: PageState) => {
    const { first, more } = leadPartsOf(s);
    return more === '' ? first : `${first} ${more}`;
  };

  it('says in the lead what the form is for', () => {
    expect(leadText(loading)).toBe('');
    expect(leadText(ready({ mode: 'edit' }))).toContain('recalculados na hora');
    expect(leadText(ready({ mode: 'edit', kind: 'minion' }))).toBe('Ficha curta de minion.');
    expect(leadText(ready({ reserved: true }))).toContain('O personagem fica reservado');
    expect(leadText(ready())).toContain('calculados quando você criar');
    expect(leadText(ready({ kind: 'boss' }))).toContain('com ficha completa');
    expect(leadText(ready({ kind: 'minion' }))).toContain(
      'ficha curta, só com o que se usa na mesa',
    );
  });
});

describe('the lead and the note on a phone', () => {
  const loading: PageState = { status: 'loading', title: 'Ficha' };

  it('cuts the lead of a full sheet to its first sentence and keeps the rest for a wide screen', () => {
    expect(leadPartsOf(loading)).toEqual({ first: '', more: '' });
    expect(leadPartsOf(ready())).toEqual({
      first: 'Preencha os passos na ordem que quiser.',
      more: 'Modificadores, Classe de Armadura e pontos de vida são calculados quando você criar.',
    });
    expect(leadPartsOf(ready({ mode: 'edit' })).first).toBe(
      'As mudanças valem quando você salvar.',
    );
    expect(leadPartsOf(ready({ kind: 'minion' })).more).toBe('');
  });

  it('keeps the numbers the server calculates in the one-line note of a full sheet', () => {
    expect(phoneNoteOf(loading)).toBe('');
    expect(phoneNoteOf(ready())).toBe('CA, PV e modificadores são calculados ao criar.');
    expect(phoneNoteOf(ready({ mode: 'edit' }))).toBe(
      'Salva todos os passos; CA e PV são recalculados.',
    );
    expect(phoneNoteOf(ready({ kind: 'minion' }))).toBe(actionsNoteOf(ready({ kind: 'minion' })));
  });
});

describe('what the table leaves the hit points of a new sheet', () => {
  const table = (hitPoints: AbilityTableVm['hitPoints']) => ({ hitPoints }) as AbilityTableVm;

  it('forces the one way the table allows, and none when the player chooses', () => {
    expect(tableHitPointsMethod(null)).toBeNull();
    expect(tableHitPointsMethod(table('roll'))).toBe('rolled');
    expect(tableHitPointsMethod(table('average'))).toBe('average');
    expect(tableHitPointsMethod(table('player_chooses'))).toBeNull();
  });
});

describe('what the server is asked to derive', () => {
  const full = { name: 'Tharn' } as never;

  it('carries the campaign, the character being edited and the draft', () => {
    expect(previewInputOf(ready(), full, true)).toEqual({
      campaignId: 'camp',
      characterId: null,
      kind: 'player',
      full,
    });
    expect(previewInputOf(ready({ mode: 'edit', characterId: 'char' }), full, true)).toMatchObject({
      characterId: 'char',
    });
  });

  it("says a reserved new sheet is the master's for a player, only when asked to", () => {
    expect(previewInputOf(ready({ reserved: true }), full, true).forPlayer).toBe(true);
    expect(previewInputOf(ready({ reserved: true }), full, false).forPlayer).toBeUndefined();
    expect(
      previewInputOf(ready({ reserved: true, mode: 'edit', characterId: 'c' }), full, true)
        .forPlayer,
    ).toBeUndefined();
  });
});

describe('the XP a new player sheet starts at', () => {
  const byXp = catalog({ xpMode: 'enemies', levelXp: [0, 300, 900] });

  it("is its level's in a campaign that levels by XP", () => {
    expect(startingXp(ready({ catalog: byXp }), 3)).toBe(900);
    expect(startingXp(ready({ catalog: byXp }), 1)).toBe(0);
  });

  it('clamps the level to the table, and takes level 1 for what is not a number', () => {
    expect(startingXp(ready({ catalog: byXp }), 20)).toBe(900);
    expect(startingXp(ready({ catalog: byXp }), 0)).toBe(0);
    expect(startingXp(ready({ catalog: byXp }), Number.NaN)).toBe(0);
  });

  it('is nothing for a milestone campaign, an empty table, an NPC, an edit or a page that is not open', () => {
    expect(
      startingXp(ready({ catalog: catalog({ xpMode: 'milestones', levelXp: [0, 300] }) }), 2),
    ).toBeNull();
    expect(
      startingXp(ready({ catalog: catalog({ xpMode: 'enemies', levelXp: [] }) }), 2),
    ).toBeNull();
    expect(startingXp(ready({ catalog: catalog({ xpMode: 'enemies' }) }), 2)).toBeNull();
    expect(startingXp(ready({ catalog: byXp, kind: 'enemy' }), 2)).toBeNull();
    expect(startingXp(ready({ catalog: byXp, mode: 'edit' }), 2)).toBeNull();
    expect(startingXp({ status: 'loading', title: 'x' }, 2)).toBeNull();
  });
});
