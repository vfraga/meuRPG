import { type ComponentFixture, TestBed } from '@angular/core/testing';
import { MatDialog } from '@angular/material/dialog';
import { Subject } from 'rxjs';

import { FavorBlockedReason } from '../../../../gen/meurpg/play/v1/favor_pb';
import { FavorClient } from '../../../core/play/favor-client';
import { FavorRecipients } from '../../../core/play/favor-recipients';
import { FakeFavorClient, favorPass, favorRefused } from '../../../core/play/favor-testing';
import { pensantusVitals } from '../../../core/play/live-testing';
import { FavorPanel } from './favor-panel';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

const tor = { characterId: 'tor', name: 'Tor', detail: 'Guerreiro 5' };

function setup(viewer: 'owner' | 'master', holders: readonly string[], others = [tor]) {
  const client = new FakeFavorClient();
  client.holders = holders.map((characterId) => ({ characterId, name: 'Pensantus' }));
  const recipients = { list: vi.fn().mockResolvedValue(others) };
  const closed = new Subject<boolean>();
  const open = vi.fn().mockReturnValue({ afterClosed: () => closed });
  TestBed.configureTestingModule({
    providers: [
      { provide: FavorClient, useValue: client.asClient() },
      { provide: FavorRecipients, useValue: recipients },
      { provide: MatDialog, useValue: { open } },
    ],
  });
  const fixture = TestBed.createComponent(FavorPanel);
  fixture.componentRef.setInput('campaignId', 'camp');
  fixture.componentRef.setInput('characterId', 'char-1');
  fixture.componentRef.setInput('viewer', viewer);
  fixture.componentRef.setInput('vitals', pensantusVitals({ characterId: 'char-1' }));
  fixture.detectChanges();
  return { fixture, client, recipients, open, closed, el: fixture.nativeElement as HTMLElement };
}

async function settle(fixture: ComponentFixture<FavorPanel>) {
  for (let i = 0; i < 3; i++) {
    await fixture.whenStable();
    await Promise.resolve();
    fixture.detectChanges();
  }
}

describe('FavorPanel', () => {
  beforeEach(() => TestBed.resetTestingModule());

  it('reads to the owner what it is, from whom and how to use it, with "Passar a…" beside the title', async () => {
    const { fixture, el } = setup('owner', ['char-1']);
    await settle(fixture);
    expect(flat(el.querySelector('h2'))).toBe('starInspiração');
    expect(flat(el.querySelector('.favor__text'))).toBe(
      'Do mestre. Vantagem num d20 seu: marque “Usar a Inspiração” antes de rolar.',
    );
    const pass = el.querySelector('button.favor__pass');
    expect(flat(pass)).toBe('keyboard_tabPassar a…');
    expect(pass?.querySelector('mat-icon')?.getAttribute('aria-hidden')).toBe('true');
  });

  it("says the title alone to the master: the sentence is the player's, and the master asks nothing", async () => {
    const { fixture, el, recipients } = setup('master', ['char-1']);
    await settle(fixture);
    expect(el.querySelector('h2')).not.toBeNull();
    expect(el.querySelector('.favor__text')).toBeNull();
    expect(el.querySelector('button')).toBeNull();
    expect(recipients.list).not.toHaveBeenCalled();
  });

  it('offers no "Passar a…" when there is no one to pass it to', async () => {
    const { fixture, el } = setup('owner', ['char-1'], []);
    await settle(fixture);
    expect(el.querySelector('h2')).not.toBeNull();
    expect(el.querySelector('button')).toBeNull();
  });

  it('draws nothing when the character has none, whoever the read says has it', async () => {
    const { fixture, el, recipients } = setup('owner', ['char-2']);
    await settle(fixture);
    expect(el.querySelector('section')).toBeNull();
    expect(recipients.list).not.toHaveBeenCalled();
  });

  it('draws nothing when the read fails', async () => {
    const { fixture, client, el } = setup('owner', ['char-1']);
    await settle(fixture);
    client.holders = () => {
      throw new Error('down');
    };
    fixture.componentRef.setInput(
      'vitals',
      pensantusVitals({ characterId: 'char-1', revision: 5 }),
    );
    fixture.detectChanges();
    await settle(fixture);
    expect(el.querySelector('section')).toBeNull();
  });

  it('reads again when the live numbers change: a grant tells nothing else', async () => {
    const { fixture, client, el } = setup('owner', []);
    await settle(fixture);
    expect(el.querySelector('section')).toBeNull();
    const reads = client.reads.length;
    client.holders = [{ characterId: 'char-1', name: 'Pensantus' }];
    fixture.componentRef.setInput(
      'vitals',
      pensantusVitals({ characterId: 'char-1', revision: 2 }),
    );
    fixture.detectChanges();
    await settle(fixture);
    expect(client.reads.length).toBeGreaterThan(reads);
    expect(el.querySelector('section')).not.toBeNull();
  });

  describe('"Passar a…"', () => {
    it('opens the dialog with the giver and the others, and hands the focus to "Cancelar o pedido" once asked', async () => {
      const { fixture, client, el, open, closed } = setup('owner', ['char-1']);
      await settle(fixture);
      el.querySelector<HTMLButtonElement>('.favor__pass')!.click();
      expect(open).toHaveBeenCalledTimes(1);
      const config = open.mock.calls[0][1] as {
        data: { fromId: string; fromName: string; recipients: unknown[] };
        role: string;
      };
      expect(config.data).toMatchObject({
        fromId: 'char-1',
        fromName: 'Pensantus',
        recipients: [tor],
      });
      expect(config.role).toBe('dialog');
      // The ask was made (the state has it) and the dialog closes: the row of the waiting ask takes the focus.
      client.passes = [favorPass({ fromId: 'char-1' })];
      await fixture.componentInstance['favor'].refresh();
      closed.next(true);
      closed.complete();
      await settle(fixture);
      expect(document.activeElement).toBe(el.querySelector('.favor__cancel'));
    });
  });

  describe('while the ask waits (581-07, item 4)', () => {
    async function waiting() {
      const s = setup('owner', ['char-1']);
      s.client.passes = [favorPass({ fromId: 'char-1' })];
      await settle(s.fixture);
      await fixture_refresh(s.fixture);
      return s;
    }
    async function fixture_refresh(fixture: ComponentFixture<FavorPanel>) {
      await fixture.componentInstance['favor'].refresh();
      await settle(fixture);
    }

    it('shows "Para Tor · esperando o mestre" with "Cancelar o pedido", and no "Passar a…"', async () => {
      const { el } = await waiting();
      expect(flat(el.querySelector('[data-testid=favor-ask]'))).toBe(
        'keyboard_tabPara Tor · esperando o mestre Cancelar o pedido',
      );
      expect(el.querySelector('b')?.textContent).toBe('Para Tor');
      expect(el.querySelector('.favor__pass')).toBeNull();
    });

    it('cancels the ask, and the focus goes back to "Passar a…"', async () => {
      const { fixture, client, el } = await waiting();
      client.passes = [];
      el.querySelector<HTMLButtonElement>('.favor__cancel')!.click();
      await settle(fixture);
      expect(client.cancels[0]).toMatchObject({ passId: 'pass-1' });
      expect(el.querySelector('[data-testid=favor-ask]')).toBeNull();
      expect(document.activeElement).toBe(el.querySelector('.favor__pass'));
    });

    it('reads "Cancelando…" while the cancel is on its way, dashed, and does not send it twice', async () => {
      const { fixture, client, el } = await waiting();
      client.hold = 'cancel';
      el.querySelector<HTMLButtonElement>('.favor__cancel')!.click();
      fixture.detectChanges();
      const button = el.querySelector<HTMLButtonElement>('.favor__cancel')!;
      expect(flat(button)).toBe('Cancelando…');
      expect(button.getAttribute('aria-disabled')).toBe('true');
      button.click();
      expect(client.cancels).toHaveLength(1);
      client.passes = [];
      client.release[0]();
      await settle(fixture);
    });

    it('says in an alert why a cancel was refused, and keeps the ask', async () => {
      const { fixture, client, el } = await waiting();
      client.cancelError = favorRefused(FavorBlockedReason.PASS_CLOSED);
      el.querySelector<HTMLButtonElement>('.favor__cancel')!.click();
      await settle(fixture);
      expect(flat(el.querySelector('[role=alert]'))).toBe('errorEsse pedido já foi respondido.');
    });
  });
});
