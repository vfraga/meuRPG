import { Code, ConnectError } from '@connectrpc/connect';

import { NoteEditing } from './note-editing';
import { NotesState } from './notes-state';
import { FakeNotesClient, note, scene } from './notes-testing';

// Finding U17-4 (review/unit-17-contract.md)
describe('Review17 U17-4: saving a note edit with no change shows a wrong validation error', () => {
  let api: FakeNotesClient;
  let state: NotesState;
  let editing: NoteEditing;

  beforeEach(async () => {
    api = new FakeNotesClient();
    api.scenesList = [scene('s2', 'A ponte do rio')];
    api.notes = [
      note('n1', 'Brisa me deve 5 PO', new Date(2026, 9, 1, 22, 3), {
        sceneId: 's2',
        sceneName: 'A ponte do rio',
      }),
    ];
    // Like the server (backend/internal/notes/rpc.go UpdateNote): neither field -> invalid_argument.
    const update = api.update.bind(api);
    api.update = async (campaignId, noteId, changes) => {
      if (changes.text === undefined && changes.scenePointId === undefined) {
        api.calls.push(`update ${noteId} rejected`);
        throw new ConnectError('nothing to change', Code.InvalidArgument);
      }
      return update(campaignId, noteId, changes);
    };
    state = new NotesState(api as never, () => 'c1');
    editing = new NoteEditing(state, () => '');
    await state.refresh();
  });

  it('does not show the validation error when nothing was changed', async () => {
    editing.openEdit(state.notes()[0]);
    await editing.save();
    expect(editing.error()).toBe('');
  });

  it('does not show it when the text differs only by surrounding whitespace, and closes the form', async () => {
    editing.openEdit(state.notes()[0]);
    editing.text.setValue('  Brisa me deve 5 PO  ');
    await editing.save();
    expect(editing.error()).toBe('');
    expect(editing.stage()).toBe('list');
  });
});
