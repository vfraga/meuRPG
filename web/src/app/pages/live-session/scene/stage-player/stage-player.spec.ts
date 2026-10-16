import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { of } from 'rxjs';

import { SceneState } from '../../../../core/play/scene-state';
import { masterScene, playerScene, stageNpc } from '../../../../core/play/scene-testing';
import { StagePlayer } from './stage-player';
import { StageViewer } from './stage-viewer';
import { nextTurn } from '../../../../../test-settle';

/** The text of an element as read, without the icons' ligature names ("check"). */
const flat = (e: Element | null | undefined) => {
  if (!e) {
    return undefined;
  }
  const copy = e.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return copy.textContent
    ?.replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
};

const mira = stageNpc('s1', 'Mira', { portraitUrl: '/images/i1' });
const capitao = stageNpc('s2', 'Capitão Goblin', { portraitUrl: '/images/i2' });
const aldo = stageNpc('s3', 'Aldo');
const ivo = stageNpc('s4', 'Barão Ivo', { portraitUrl: '/images/i4' });

describe('StagePlayer', () => {
  const dialogOpen = vi.fn();

  function setup(stage = [mira, capitao]) {
    const state = new SceneState(
      () => Promise.resolve(playerScene([], stage)),
      () => false,
    );
    state.apply(playerScene([], stage));
    dialogOpen
      .mockReset()
      .mockReturnValue({ afterClosed: () => of(undefined), afterDismissed: () => of(undefined) });
    TestBed.configureTestingModule({
      providers: [
        { provide: MatDialog, useValue: { open: dialogOpen } },
        { provide: MatBottomSheet, useValue: { open: dialogOpen } },
      ],
    });
    const fixture = TestBed.createComponent(StagePlayer);
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    document.body.append(fixture.nativeElement);
    return { fixture, state, el: fixture.nativeElement as HTMLElement };
  }

  async function settle(fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) {
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      await nextTurn();
      fixture.detectChanges();
    }
  }

  const figures = (el: HTMLElement) => Array.from(el.querySelectorAll<HTMLElement>('.stage__btn'));
  afterEach(() => {
    document.body.querySelectorAll('app-stage-player').forEach((e) => e.remove());
  });

  it('disappears when the stage is empty: no block, no "Ninguém em cena"', () => {
    const { el } = setup([]);
    expect(el.querySelector('.stage')).toBeNull();
    expect(el.textContent?.trim()).toBe('');
  });

  it.each([
    [1, [mira]],
    [2, [mira, capitao]],
    [3, [mira, capitao, aldo]],
    [4, [mira, capitao, aldo, ivo]],
  ])(
    'draws %i NPC(s) as a group named by who is on it, one figure each, whole names',
    (count, stage) => {
      const { el } = setup(stage);
      const group = el.querySelector('[role="group"]')!;
      expect(group.getAttribute('aria-label')).toBe(
        `Em cena: ${stage
          .map((n) => n.name)
          .join(count === 2 ? ' e ' : ', ')
          .replace(/, ([^,]+)$/, ' e $1')}`,
      );
      expect(el.querySelector('.stage')?.getAttribute('data-count')).toBe(String(count));
      expect(figures(el).map((f) => flat(f.querySelector('.stage__name')))).toEqual(
        stage.map((n) => n.name),
      );
    },
  );

  it('draws the hint, in a quiet line, when there are portraits to choose among', () => {
    const { el } = setup();
    expect(flat(el.querySelector('.stage__hint'))).toContain('Toque num retrato para ver maior');
  });

  it('has no hint for a single portrait', () => {
    const { el } = setup([mira]);
    expect(el.querySelector('.stage__hint')).toBeNull();
  });

  it('puts the speaker in front: the base line, the bold name and "Fala agora" in words', () => {
    const { el } = setup([mira, { ...capitao, speaking: true }]);
    const items = Array.from(el.querySelectorAll('.stage__item'));
    expect(items.map((i) => i.classList.contains('stage__item--speaking'))).toEqual([false, true]);
    expect(items[1].querySelector('app-stage-figure .fig--speaking')).not.toBeNull();
    expect(items[0].querySelector('app-stage-figure .fig--speaking')).toBeNull();
    expect(flat(items[1].querySelector('.stage__tag'))).toContain('Fala agora');
    expect(items[0].querySelector('.stage__tag')).toBeNull();
    expect(figures(el)[1].getAttribute('aria-label')).toBe('Ver Capitão Goblin maior, fala agora');
    expect(figures(el)[0].getAttribute('aria-label')).toBe('Ver Mira maior');
  });

  it('draws the portrait with no box: a bare image with its base line, the thumbnail of the stage', () => {
    const { el } = setup([mira]);
    const img = el.querySelector<HTMLImageElement>('.fig__img')!;
    expect(img.getAttribute('src')).toBe('/images/i1/thumb');
    expect(img.getAttribute('alt')).toBe('');
    expect(el.querySelector('app-stage-figure')?.getAttribute('aria-label')).toBe(
      'Retrato de Mira',
    );
    expect(el.querySelector('.fig__tile')).toBeNull();
  });

  it('keeps the tile only for the initials: an NPC with no portrait', () => {
    const { el } = setup([aldo]);
    expect(el.querySelector('.fig__img')).toBeNull();
    expect(flat(el.querySelector('.fig__tile'))).toBe('AL');
    expect(el.querySelector('app-stage-figure')?.getAttribute('aria-label')).toBe(
      'Sem retrato: Aldo',
    );
  });

  it('falls back to the initials, with no broken image, when the portrait is a 404', async () => {
    const { el, fixture } = setup([capitao]);
    el.querySelector('.fig__img')!.dispatchEvent(new Event('error'));
    await settle(fixture);
    expect(el.querySelector('.fig__img')).toBeNull();
    expect(flat(el.querySelector('.fig__tile'))).toBe('CG');
  });

  it('follows the stage: an NPC comes in, the speaker changes, one leaves', async () => {
    const { el, state, fixture } = setup([mira]);
    state.apply(playerScene([], [mira, capitao]));
    await settle(fixture);
    expect(figures(el)).toHaveLength(2);
    state.apply(playerScene([], [mira, { ...capitao, speaking: true }]));
    await settle(fixture);
    expect(flat(el.querySelector('.stage__tag'))).toContain('Fala agora');
    state.apply(playerScene([], [{ ...capitao, speaking: true }]));
    await settle(fixture);
    expect(figures(el).map((f) => flat(f.querySelector('.stage__name')))).toEqual([
      'Capitão Goblin',
    ]);
  });

  it("is built for the master's scene too, without needing a character id", () => {
    const state = new SceneState(
      () => Promise.resolve(masterScene()),
      () => true,
    );
    state.apply(masterScene([], [stageNpc('s1', 'Mira', { master: true })]));
    const fixture = TestBed.createComponent(StagePlayer);
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelectorAll('.stage__btn')).toHaveLength(1);
  });

  describe('tapping a character opens it larger', () => {
    it('opens the view for that entry, named by its own title, from the figure that was tapped', () => {
      const { el } = setup([mira, capitao]);
      figures(el)[1].click();
      expect(dialogOpen).toHaveBeenCalledTimes(1);
      const [component, config] = dialogOpen.mock.calls[0] as [
        unknown,
        { data: { entryId: string }; ariaLabelledBy: string },
      ];
      expect(component).toBe(StageViewer);
      expect(config.data.entryId).toBe('s2');
      expect(config.ariaLabelledBy).toBe('stage-viewer-title');
    });

    it('opens by keyboard too: the figure is a real button', () => {
      const { el } = setup([mira]);
      expect(figures(el)[0].tagName).toBe('BUTTON');
      expect(figures(el)[0].getAttribute('type')).toBe('button');
    });

    it('gives focus back to the tapped figure when the view closes', async () => {
      const { el, fixture } = setup([mira, capitao]);
      figures(el)[0].click();
      await settle(fixture);
      expect(document.activeElement).toBe(figures(el)[0]);
    });

    it('leaves focus alone when the person already moved it somewhere else', async () => {
      const { el, fixture } = setup([mira, capitao]);
      const other = document.createElement('button');
      document.body.append(other);
      figures(el)[0].click();
      other.focus();
      await settle(fixture);
      expect(document.activeElement).toBe(other);
      other.remove();
    });
  });
});
