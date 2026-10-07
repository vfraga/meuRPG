// Finding U11-16: saving an edited note that was not changed sends UpdateNote
// with neither text nor scenePointId; the server answers InvalidArgument
// "nothing to change", which the UI shows as a wrong message about text length
// and scenes, and the form stays open.
import { Code, ConnectError } from '@connectrpc/connect';

import { NoteEditing } from './note-editing';
import { NotesState } from './notes-state';
import { FakeNotesClient, note, scene } from './notes-testing';

describe('Review11 U11-16: saving an unchanged note', () => {
  it('sends no empty UpdateNote and shows no misleading error', async () => {
    const api = new FakeNotesClient();
    api.scenesList = [scene('s2', 'A ponte do rio')];
    api.notes = [
      note('n1', 'Brisa me deve 5 PO', new Date(2026, 9, 1, 22, 3), {
        sceneId: 's2',
        sceneName: 'A ponte do rio',
      }),
    ];
    const state = new NotesState(api as never, () => 'c1');
    const editing = new NoteEditing(state, () => '');
    await state.refresh();

    // The server rejects an update with no field, as UpdateNote does.
    const realUpdate = api.update.bind(api);
    api.update = async (campaignId: string, id: string, changes: object) => {
      if (Object.keys(changes).length === 0) {
        api.calls.push(`update ${id} {}`);
        throw new ConnectError('nothing to change', Code.InvalidArgument);
      }
      return realUpdate(campaignId, id, changes);
    };

    editing.openEdit(state.notes()[0]);
    expect(editing.dirty()).toBe(false);
    await editing.save();

    const updates = api.calls.filter((c) => c.startsWith('update'));
    expect(updates, 'no RPC should be sent for an unchanged note').toEqual([]);
    expect(editing.error()).not.toContain('escolha uma cena');
  });
});
