import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import { WatchGameSessionResponse_TableScreenChangedSchema } from '../../../gen/meurpg/play/v1/play_pb';
import { toLiveEvent } from './live-event-map';

describe('toLiveEvent: table_screen_changed', () => {
  it('maps the presence and the revision', () => {
    const lostAt = new Date(2026, 0, 5, 21, 14);
    const value = create(WatchGameSessionResponse_TableScreenChangedSchema, {
      open: false,
      lostAt: timestampFromDate(lostAt),
      choiceRevision: 6,
      presenceKnown: true,
    });
    expect(toLiveEvent({ case: 'tableScreenChanged', value })).toEqual({
      kind: 'tableScreenChanged',
      open: false,
      lostAt,
      choiceRevision: 6,
      presenceKnown: true,
    });
  });

  it('has no lostAt while a tab is open', () => {
    const value = create(WatchGameSessionResponse_TableScreenChangedSchema, {
      open: true,
      choiceRevision: 1,
      presenceKnown: true,
    });
    expect(toLiveEvent({ case: 'tableScreenChanged', value })).toMatchObject({
      open: true,
      lostAt: null,
    });
  });
});
