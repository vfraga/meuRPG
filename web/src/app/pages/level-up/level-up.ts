import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, Router } from '@angular/router';

import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterBlockedReason,
  type Character,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { LevelUpClient } from '../../core/levelup/levelup-client';
import { LiveSessionSourceLive } from '../live-session/live-session-source.live';
import { LevelUpDraft } from '../../core/levelup/levelup-draft';
import {
  cannotLevelUpMessage,
  describeLevelUpFailure,
  refusalMessage,
  refusalStep,
  type LevelUpFailure,
} from '../../core/levelup/levelup-errors';
import {
  STEP_LABELS,
  type LevelUpDone,
  type SheetKeys,
  type StepKey,
} from '../../core/levelup/levelup-flow';
import { ContentWatcher } from '../../core/content/content-watcher';
import { openSpellDetails } from '../../shared/spell-details/open-spell-details';
import { AbilitiesStep } from './abilities-step/abilities-step';
import { HpStep } from './hp-step/hp-step';
import { LevelUpSession } from './level-up-session';
import { PicksStep } from './picks-step/picks-step';
import { SideColumn } from './side-column/side-column';
import { SpellsStep } from './spells-step/spells-step';
import { StepsBar } from './steps-bar/steps-bar';
import { SummaryStep } from './summary-step/summary-step';

type PageState =
  | { readonly status: 'loading' }
  /** Nothing to level up here: not the player's, not allowed yet, dead, or not found. */
  | { readonly status: 'blocked'; readonly message: string }
  | { readonly status: 'error'; readonly message: string }
  | { readonly status: 'ready'; readonly session: LevelUpSession };

/** The keys the sheet already has, for the pickers (a content key is never offered twice). */
function sheetKeys(character: Character): SheetKeys {
  const full = character.sheet?.content.case === 'full' ? character.sheet.content.value : null;
  return {
    cantrips: full?.cantripKeys ?? [],
    known: full?.knownSpellKeys ?? [],
    prepared: full?.preparedSpellKeys ?? [],
    skills: full?.skillProficiencyKeys ?? [],
    expertise: full?.expertiseSkillKeys ?? [],
  };
}

/**
 * "/campaigns/:id/characters/:characterId/level-up" (MR-040, RN-01's exception, RN-12): the
 * guided level-up of a locked sheet. The player goes step by step (Habilidades, Vida, Escolhas
 * and Magias when the level has them, Resumo) and nothing is saved until "Confirmar o nível N":
 * the server checks every choice with the rules engine and answers with the new sheet, or with
 * the reason, in place. Every number comes from `PreviewLevelUp` (ADR-0008). A step with nothing to
 * choose does not appear. Desktop: the step on the left, "O que muda até aqui" and "O resto da ficha"
 * on the right (below the step under 1100px). Phone: the footer with the two buttons is fixed,
 * and on a short screen so is the title with "Passo N de M".
 */
@Component({
  selector: 'app-level-up',
  imports: [
    AbilitiesStep,
    HpStep,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    PicksStep,
    SideColumn,
    SpellsStep,
    StepsBar,
    SummaryStep,
  ],
  // Its own client: the catalog and the spell descriptions it keeps live as long as the page, never past a reload of the
  // table's content. The session's stream tells the page when the content changes (`ContentWatcher`: one stream, debounced).
  providers: [LevelUpClient, LiveSessionSourceLive, ContentWatcher],
  templateUrl: './level-up.html',
  styleUrl: './level-up.scss',
})
export class LevelUpPage {
  private readonly client = inject(LevelUpClient);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly watcher = inject(ContentWatcher);

  protected readonly state = signal<PageState>({ status: 'loading' });
  protected readonly campaignId = signal('');
  protected readonly characterId = signal('');
  protected readonly index = signal(0);
  protected readonly busy = signal(false);
  protected readonly failure = signal<LevelUpFailure | null>(null);
  /** The question "Descartar as escolhas?", asked in place, and which control asked it. */
  protected readonly asking = signal<'top' | 'foot' | null>(null);

  protected readonly session = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? s.session : null;
  });
  protected readonly steps = computed<StepKey[]>(() => this.session()?.draft.steps() ?? []);
  protected readonly step = computed<StepKey>(
    () => this.steps()[Math.min(this.index(), this.steps().length - 1)] ?? 'hp',
  );
  protected readonly stepLabel = computed(() => STEP_LABELS[this.step()]);
  protected readonly isLast = computed(() => this.step() === 'summary');
  protected readonly isFirst = computed(() => this.index() === 0);
  protected readonly labels = STEP_LABELS;

  /** What blocks "Próximo": the choices still missing in this step. */
  protected readonly missingHere = computed(
    () => this.session()?.draft.missingIn(this.step()) ?? [],
  );
  /** Why "Próximo" waits: a choice still missing, or a rule the server says the step's choices break. */
  protected readonly blockReason = computed(() => {
    const s = this.session();
    const first = this.missingHere()[0];
    if (first) {
      return first.text;
    }
    const p = s?.preview.state();
    if (
      p &&
      !p.loading &&
      p.refusal &&
      this.step() !== 'summary' &&
      refusalStep(p.refusal.reason, p.refusal.field) === this.step()
    ) {
      return refusalMessage(p.refusal);
    }
    return '';
  });
  protected readonly blocked = computed(() => this.blockReason() !== '');
  /** After a tap on the blocked "Próximo": the reason is said again to a screen reader, and the choice is marked. */
  protected readonly attempted = signal(false);

  /** The summary's own refusal: the server says the choices break a rule (never while it still answers). */
  protected readonly refusal = computed(() => {
    const p = this.session()?.preview.state();
    return p && !p.loading ? p.refusal : null;
  });

  protected readonly sheetHref = computed(() => this.sheetLink().join('/'));
  protected readonly sheetLink = computed(() => [
    '/campaigns',
    this.campaignId(),
    'characters',
    this.characterId(),
  ]);

  constructor() {
    const destroyRef = inject(DestroyRef);
    this.route.paramMap.pipe().subscribe((params) => {
      const campaignId = params.get('id');
      const characterId = params.get('characterId');
      if (campaignId && characterId) {
        this.campaignId.set(campaignId);
        this.characterId.set(characterId);
        void this.load(campaignId, characterId);
      }
    });
    this.watcher.whileLive(this.campaignId, () => void this.contentChanged());
    destroyRef.onDestroy(() => {
      this.session()?.stop();
      this.footObserver?.disconnect();
    });

    // Every change of the choices asks the server what the sheet would be.
    let first = true;
    effect(() => {
      const s = this.session();
      if (!s) {
        return;
      }
      const d = s.draft;
      const choices = d.choices();
      // A table that makes everybody roll never shows the average, and the server would refuse to preview it.
      const average =
        d.hpCard() === 'roll' && d.rolled() !== null && s.hpFixed !== 'roll'
          ? d.averageChoices()
          : null;
      untracked(() => {
        s.preview.request(choices, average, first);
        first = false;
      });
    });
    // An ability increase can move the maximum of prepared spells: the preview says the new one.
    effect(() => {
      const s = this.session();
      const after = s?.preview.state().after;
      if (s && after && s.draft.effective().prepares) {
        const max =
          after.spellcasting.find((c) => c.classKey === s.options.classKey)?.preparedMax ?? 0;
        if (max > 0) {
          untracked(() => s.draft.preparedMaxAfter.set(max));
        }
      }
    });
    // A step that goes away (the subclass changed what the level asks) never leaves the index past the end.
    effect(() => {
      const n = this.steps().length;
      if (n > 0 && this.index() > n - 1) {
        untracked(() => this.index.set(n - 1));
      }
    });
  }

  /** Numbers every read that makes a session (the first one and each re-read): an answer older than the latest is dropped,
   * so the reading on screen is always the newest one asked for. */
  private reads = 0;

  private async load(campaignId: string, characterId: string): Promise<void> {
    const seq = ++this.reads;
    this.state.set({ status: 'loading' });
    let character: Character | null = null;
    try {
      character = await this.client.character(campaignId, characterId);
      if (seq !== this.reads) {
        return;
      }
      if (character.canAccessMasterNotes) {
        // The master's own call: the owning player levels up; the master edits the sheet.
        this.state.set({
          status: 'blocked',
          message:
            'Quem sobe o nível é o jogador, pelo botão da ficha. O mestre ajusta a ficha pelo editor.',
        });
        return;
      }
      const [options, catalog, preference] = await Promise.all([
        this.client.options(campaignId, characterId),
        this.client.catalog(campaignId, characterId),
        this.client.dicePreference(campaignId).catch(() => 0),
      ]);
      if (seq !== this.reads) {
        return;
      }
      const draft = new LevelUpDraft(options, sheetKeys(character), catalog);
      const session = new LevelUpSession(
        campaignId,
        character,
        options,
        draft,
        this.client,
        preference,
        (key, name) => this.describeSpell(key, name),
      );
      this.state.set({ status: 'ready', session });
      afterNextRender(() => this.watchFoot(), { injector: this.injector });
    } catch (err) {
      if (seq !== this.reads) {
        return;
      }
      const failure = describeLevelUpFailure(err);
      const message = await this.refusalMessage(campaignId, character, failure);
      if (seq !== this.reads) {
        return;
      }
      this.state.set({ status: failure.kind === 'blocked' ? 'blocked' : 'error', message });
    }
  }

  /** The campaign's mode and the sheet's XP are known: say what is missing, not only that something is. */
  private async refusalMessage(
    campaignId: string,
    character: Character | null,
    failure: LevelUpFailure,
  ): Promise<string> {
    if (
      failure.kind !== 'blocked' ||
      failure.reason !== CharacterBlockedReason.CANNOT_LEVEL_UP ||
      !character
    ) {
      return failure.message;
    }
    const mode = await this.client.xpMode(campaignId).catch(() => XpMode.UNSPECIFIED);
    const full = character.sheet?.content.case === 'full' ? character.sheet.content.value : null;
    return cannotLevelUpMessage(
      mode,
      full?.experiencePoints ?? 0,
      character.derived?.nextLevelXp ?? 0,
      character.derived?.totalLevel ?? 0,
    );
  }

  private footObserver: ResizeObserver | undefined;

  /** The footer is fixed on a phone: the page keeps its height free at the bottom, whatever it holds (the reason, the
   * question, the stacked buttons), so nothing of the step is hidden behind it. */
  private watchFoot(): void {
    const foot = this.host.nativeElement.querySelector<HTMLElement>('.foot');
    if (!foot || typeof ResizeObserver === 'undefined') {
      return;
    }
    this.footObserver?.disconnect();
    this.footObserver = new ResizeObserver(() =>
      this.host.nativeElement.style.setProperty('--foot-h', `${foot.offsetHeight}px`),
    );
    this.footObserver.observe(foot);
  }

  /** "Voltar para a ficha" of the blocked page. */
  protected onBlockedBack(event: Event): void {
    event.preventDefault();
    void this.leave();
  }

  private describeSpell(key: string, name: string): void {
    openSpellDetails(this.dialog, this.bottomSheet, {
      namePt: name,
      load: () => this.client.spellDetails(this.campaignId(), key),
    });
  }

  protected title(s: LevelUpSession): string {
    return `Subir para o nível ${s.options.totalToLevel}`;
  }

  protected subtitle(s: LevelUpSession): string {
    const o = s.options;
    return `${s.character.name} · ${o.classNamePt} ${o.fromLevel} → ${o.classNamePt} ${o.toLevel}`;
  }

  protected stepOf(): string {
    return `Passo ${this.index() + 1} de ${this.steps().length}`;
  }

  protected confirmLabel(s: LevelUpSession): string {
    return `Confirmar o nível ${s.options.totalToLevel}`;
  }

  /** "Próximo": on to the next step, or, with a choice missing, to the first one that is. */
  protected next(): void {
    if (this.blocked()) {
      this.attempted.set(true);
      this.focusMissing();
      return;
    }
    this.failure.set(null);
    this.go(this.index() + 1);
  }

  protected back(): void {
    if (this.isFirst()) {
      this.askToLeave('foot');
    } else {
      this.failure.set(null);
      this.go(this.index() - 1);
    }
  }

  private go(index: number): void {
    this.attempted.set(false);
    this.clearAttention();
    this.index.set(Math.max(0, Math.min(index, this.steps().length - 1)));
    afterNextRender(
      () => {
        window.scrollTo({ top: 0 });
        this.host.nativeElement
          .querySelector<HTMLElement>('.js-step')
          ?.focus({ preventScroll: true });
      },
      { injector: this.injector },
    );
  }

  /** The first choice still missing is marked, comes into view between the bars and gets a visible focus. */
  private focusMissing(): void {
    const first = this.missingHere()[0];
    afterNextRender(
      () => {
        this.clearAttention();
        const root = this.host.nativeElement;
        const card = first
          ? root.querySelector<HTMLElement>(`#pick-${first.id}`)
          : root.querySelector<HTMLElement>('.body section');
        const target = card?.querySelector<HTMLElement>(
          '.rows input:not(:disabled), input:not(:disabled), button',
        );
        card?.setAttribute('data-attn', '');
        target?.scrollIntoView({ block: 'center' });
        target?.focus({ preventScroll: true, focusVisible: true } as FocusOptions);
      },
      { injector: this.injector },
    );
  }

  /** A pick anywhere in the step: the mark and the repeated reason are over. */
  protected onPick(): void {
    this.attempted.set(false);
    this.clearAttention();
  }

  /** The mark on a card goes when the player picks anything, or leaves the step. */
  protected clearAttention(): void {
    this.host.nativeElement
      .querySelectorAll('[data-attn]')
      .forEach((el) => el.removeAttribute('data-attn'));
  }

  /** "Cancelar" and "Voltar para a ficha": with choices made, ask in place before discarding. */
  protected askToLeave(from: 'top' | 'foot'): void {
    const s = this.session();
    if (!s?.draft.dirty()) {
      void this.leave();
      return;
    }
    this.asking.set(from);
    afterNextRender(
      () => {
        const keep = this.host.nativeElement.querySelector<HTMLElement>('.js-keep');
        keep?.scrollIntoView({ block: 'center' });
        keep?.focus({ preventScroll: true });
      },
      { injector: this.injector },
    );
  }

  protected keepChoosing(): void {
    const from = this.asking();
    this.asking.set(null);
    afterNextRender(
      () =>
        this.host.nativeElement
          .querySelector<HTMLElement>(from === 'top' ? '.js-leave-top' : '.js-leave-foot, .js-next')
          ?.focus(),
      { injector: this.injector },
    );
  }

  protected leave(): Promise<boolean> {
    return this.router.navigate(this.sheetLink());
  }

  /** The link at the top: the page decides, so it never follows the link behind the question. */
  protected onTopBack(event: Event): void {
    event.preventDefault();
    this.askToLeave('top');
  }

  /** "Confirmar o nível N": the one call that saves. The answer is the new sheet, or the reason, in place. */
  protected async confirm(): Promise<void> {
    const s = this.session();
    // The button is disabledInteractive (it still gets clicks), so the guards live here.
    if (!s || this.busy() || s.draft.missing().length > 0) {
      return;
    }
    this.busy.set(true);
    this.failure.set(null);
    try {
      const character = await this.client.levelUp(
        this.campaignId(),
        this.characterId(),
        s.revision(),
        s.draft.choices(),
      );
      const done: LevelUpDone = {
        name: character.name,
        level: character.derived?.totalLevel ?? s.options.totalToLevel,
      };
      await this.router.navigate(this.sheetLink(), { replaceUrl: true, state: { levelUp: done } });
    } catch (err) {
      const failure = describeLevelUpFailure(err);
      if (failure.kind === 'stale' && (await this.levelAlreadyApplied(s))) {
        return;
      }
      this.show(failure);
    } finally {
      this.busy.set(false);
    }
  }

  /** A confirmation whose answer was lost is applied but its retry is stale: when the sheet is already at the level asked for, the
   * person goes to the sheet as after a confirmation, instead of being told to read it again. */
  private async levelAlreadyApplied(s: LevelUpSession): Promise<boolean> {
    try {
      const character = await this.client.character(this.campaignId(), this.characterId());
      const level = character.derived?.totalLevel ?? 0;
      if (level < s.options.totalToLevel) {
        return false;
      }
      await this.router.navigate(this.sheetLink(), {
        replaceUrl: true,
        state: { levelUp: { name: character.name, level } satisfies LevelUpDone },
      });
      return true;
    } catch {
      return false;
    }
  }

  private show(failure: LevelUpFailure): void {
    this.failure.set(failure);
    afterNextRender(
      () =>
        this.host.nativeElement
          .querySelector<HTMLElement>('.js-failure')
          ?.scrollIntoView({ block: 'center' }),
      { injector: this.injector },
    );
  }

  /** What the page says after the table's content changed under the person: that the lists changed, or that a choice left
   * them. It stays empty while what the page shows (the offers and the lists) is the same. */
  protected readonly contentNote = signal('');

  /** `content_changed` (RN-23): the options and the lists are read again with this person's role; the choices that are still
   * offered stay (`adopt`), and one that is not any more is said, so the person picks again before confirming. */
  private async contentChanged(): Promise<void> {
    const old = this.session();
    if (!old) {
      return;
    }
    const before = pickedCount(old.draft);
    const offers = offerSignature(old);
    const changed = await this.rereadSheet(true);
    const now = this.session();
    if (!changed || !now) {
      return;
    }
    if (pickedCount(now.draft) < before) {
      this.contentNote.set(
        'O mestre mudou as opções da mesa e uma das suas escolhas saiu da lista. Escolha de novo antes de confirmar.',
      );
    } else if (offerSignature(now) !== offers) {
      this.contentNote.set(
        'O mestre mudou as opções da mesa. As listas deste nível estão atualizadas.',
      );
    }
  }

  /** After a stale revision or a content change: the sheet, what the level gives and the lists (read fresh, never from the
   * client's memory) are read again, the session is made anew, and the picks that are still offered are carried over.
   * Resolves true when the page now shows the new reading. */
  protected async rereadSheet(afterContent = false): Promise<boolean> {
    const old = this.session();
    if (!old) {
      return false;
    }
    const seq = ++this.reads;
    try {
      // The table's content is read again too: the master may have retired an option, or written a new one.
      const [character, options, catalog] = await Promise.all([
        this.client.character(this.campaignId(), this.characterId()),
        this.client.options(this.campaignId(), this.characterId()),
        this.client.catalog(this.campaignId(), this.characterId(), true),
      ]);
      if (seq !== this.reads) {
        return false; // a newer reading was asked for: its answer is the one that counts
      }
      const draft = new LevelUpDraft(options, sheetKeys(character), catalog);
      draft.adopt(old.draft);
      const session = new LevelUpSession(
        this.campaignId(),
        character,
        options,
        draft,
        this.client,
        old.preference,
        (key, name) => this.describeSpell(key, name),
      );
      // A roll answered after this point lands on the new session (the click was made on the old one).
      old.handOver(session);
      this.failure.set(null);
      this.state.set({ status: 'ready', session });
      return true;
    } catch (err) {
      if (seq !== this.reads) {
        return false;
      }
      const failure = describeLevelUpFailure(err);
      if (failure.kind === 'blocked') {
        old.stop();
        this.state.set({ status: 'blocked', message: failure.message });
      } else if (!afterContent) {
        this.show(failure);
      }
      return false;
    }
  }

  /** A refusal names the step that owns the rule: the button goes there. */
  protected goToStep(step: StepKey | null): void {
    const i = step ? this.steps().indexOf(step) : -1;
    if (i >= 0) {
      this.failure.set(null);
      this.go(i);
    }
  }

  protected stepWords(step: StepKey | null): string {
    return step ? STEP_LABELS[step] : '';
  }
}

/** How many choices the person has made, to tell whether a re-read dropped one. */
function pickedCount(d: LevelUpDraft): number {
  return (
    (d.subclassKey() ? 1 : 0) +
    d.cantrips().size +
    d.spells().size +
    d.prepared().size +
    d.features().size +
    d.skills().size +
    d.expertise().size
  );
}

/** What the page offers to choose from (the subclasses, the spells, the classes): a change in it is what the note is about. */
function offerSignature(s: LevelUpSession): string {
  const c = s.draft.catalog;
  return JSON.stringify([
    s.options.subclasses.map((k) => [k.key, k.off, k.archived]),
    c.spells.map((x) => x.key),
    (c.classes ?? []).map((x) => x.key),
  ]);
}
