import { Code, ConnectError } from '@connectrpc/connect';

import { NoteEditing } from './note-editing';
import { FakeNotesClient, note, scene } from './notes-testing';
import { NotesState } from './notes-state';

describe('NoteEditing', () => {
  let api: FakeNotesClient;
  let state: NotesState;
  let editing: NoteEditing;
  let open = '';

  beforeEach(async () => {
    api = new FakeNotesClient();
    api.scenesList = [scene('s1', 'A carroça tombada'), scene('s2', 'A ponte do rio')];
    api.notes = [
      note('n1', 'Brisa me deve 5 PO', new Date(2026, 9, 1, 22, 3), {
        sceneId: 's2',
        sceneName: 'A ponte do rio',
      }),
    ];
    state = new NotesState(api as never, () => 'c1');
    open = '';
    editing = new NoteEditing(state, () => open);
    await state.refresh();
  });

  const type = (text: string) => editing.text.setValue(text);

  it('opens a new note with the open scene tagged, or with no scene', () => {
    open = 's1';
    editing.openNew();
    expect(editing.stage()).toBe('form');
    expect(editing.sceneId()).toBe('s1');
    editing.close();
    open = 'not-discovered';
    editing.openNew();
    expect(editing.sceneId()).toBe('');
  });

  it('opens a note filled', () => {
    editing.openEdit(state.notes()[0]);
    expect(editing.text.value).toBe('Brisa me deve 5 PO');
    expect(editing.sceneId()).toBe('s2');
    expect(editing.editing()?.id).toBe('n1');
  });

  it('says an empty note in words and keeps the form', async () => {
    editing.openNew();
    type('   ');
    expect(await editing.save()).toBe(false);
    expect(editing.error()).toBe('Escreva a anotação antes de salvar.');
    expect(editing.stage()).toBe('form');
    expect(api.calls.some((c) => c.startsWith('create'))).toBe(false);
    type('x');
    expect(editing.error()).toBe('');
  });

  it('counts the characters as the person types', () => {
    editing.openNew();
    type('Perguntar ao ferreiro');
    expect(editing.length()).toBe(21);
  });

  it('saves a new note with its scene and goes back to the list', async () => {
    editing.openNew();
    type('  Perguntar ao ferreiro  ');
    editing.sceneId.set('s1');
    expect(await editing.save()).toBe(true);
    expect(api.calls).toContain('create Perguntar ao ferreiro s1');
    expect(editing.stage()).toBe('list');
    expect(state.notes()[0].text).toBe('Perguntar ao ferreiro');
  });

  it('sends only what changed when editing', async () => {
    editing.openEdit(state.notes()[0]);
    type('Brisa me deve 10 PO');
    expect(await editing.save()).toBe(true);
    expect(api.calls).toContain('update n1 {"text":"Brisa me deve 10 PO"}');
    editing.openEdit(state.notes()[0]);
    editing.sceneId.set('');
    await editing.save();
    expect(api.calls).toContain('update n1 {"scenePointId":""}');
  });

  it('discards at once when nothing was written, and asks in place when there is something to lose', () => {
    editing.openNew();
    editing.cancel();
    expect(editing.stage()).toBe('list');
    editing.openNew();
    type('Algo');
    editing.cancel();
    expect(editing.stage()).toBe('form');
    expect(editing.confirmingDiscard()).toBe(true);
    editing.keepWriting();
    expect(editing.confirmingDiscard()).toBe(false);
    expect(editing.text.value).toBe('Algo');
    editing.cancel();
    editing.close();
    expect(editing.stage()).toBe('list');
  });

  it('keeps the text and the form when cancelled again while the question shows', () => {
    editing.openNew();
    type('Algo importante');
    editing.cancel();
    expect(editing.confirmingDiscard()).toBe(true);
    editing.cancel();
    expect(editing.stage()).toBe('form');
    expect(editing.confirmingDiscard()).toBe(true);
    expect(editing.text.value).toBe('Algo importante');
  });

  it('counts a new note tagged with the open scene as untouched until something is written', () => {
    open = 's1';
    editing.openNew();
    expect(editing.dirty()).toBe(false);
    editing.sceneId.set('s2');
    expect(editing.dirty()).toBe(true);
  });

  it('asks before deleting, deletes on the answer and goes back to the list', async () => {
    editing.openEdit(state.notes()[0]);
    editing.askDelete();
    expect(editing.confirmingDelete()).toBe(true);
    expect(api.calls.some((c) => c.startsWith('delete'))).toBe(false);
    editing.keepNote();
    expect(editing.confirmingDelete()).toBe(false);
    editing.askDelete();
    expect(await editing.remove()).toBe(true);
    expect(api.calls).toContain('delete n1');
    expect(state.notes()).toEqual([]);
    expect(editing.stage()).toBe('list');
  });

  it('says the 300-note limit by its code and keeps the text', async () => {
    editing.openNew();
    type('Mais uma');
    api.failWith = new ConnectError('x', Code.ResourceExhausted);
    expect(await editing.save()).toBe(false);
    expect(editing.error()).toBe('Limite de 300 anotações. Apague uma para escrever outra.');
    expect(editing.text.value).toBe('Mais uma');
    expect(editing.stage()).toBe('form');
  });

  it('closes the form and reads the list again when the note is gone', async () => {
    editing.openEdit(state.notes()[0]);
    type('Outro texto');
    api.failWith = new ConnectError('x', Code.NotFound);
    expect(await editing.save()).toBe(false);
    expect(editing.stage()).toBe('list');
    expect(api.calls.filter((c) => c === 'list').length).toBeGreaterThan(1);
  });

  it('ignores a second save while the first is in flight', async () => {
    editing.openNew();
    type('Uma só vez');
    const first = editing.save();
    expect(editing.busy()).toBe(true);
    expect(await editing.save()).toBe(false);
    await first;
    expect(api.calls.filter((c) => c.startsWith('create'))).toHaveLength(1);
  });

  describe('saving an edit that changed nothing', () => {
    // The server refuses an UpdateNote with neither field as "nothing to change".
    beforeEach(() => {
      const update = api.update.bind(api);
      api.update = async (campaignId, noteId, changes) => {
        if (changes.text === undefined && changes.scenePointId === undefined) {
          api.calls.push(`update ${noteId} {}`);
          throw new ConnectError('nothing to change', Code.InvalidArgument);
        }
        return update(campaignId, noteId, changes);
      };
    });

    it('closes the form without sending anything', async () => {
      editing.openEdit(state.notes()[0]);
      expect(await editing.save()).toBe(true);
      expect(api.calls.filter((c) => c.startsWith('update'))).toEqual([]);
      expect(editing.error()).toBe('');
      expect(editing.stage()).toBe('list');
    });

    it('treats spaces around the text as no change', async () => {
      editing.openEdit(state.notes()[0]);
      type('  Brisa me deve 5 PO  ');
      expect(await editing.save()).toBe(true);
      expect(api.calls.filter((c) => c.startsWith('update'))).toEqual([]);
      expect(editing.stage()).toBe('list');
    });

    it('still sends a real change', async () => {
      editing.openEdit(state.notes()[0]);
      type('Brisa me deve 10 PO');
      expect(await editing.save()).toBe(true);
      expect(api.calls).toContain('update n1 {"text":"Brisa me deve 10 PO"}');
    });
  });
});
