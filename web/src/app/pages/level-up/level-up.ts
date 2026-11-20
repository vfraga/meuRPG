import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, Router } from '@angular/router';

import { LevelUpClient } from '../../core/levelup/levelup-client';
import { LiveSessionSourceLive } from '../../core/play/live-session-source.live';
import {
  describeLevelUpFailure,
  refusalMessage,
  refusalStep,
  type LevelUpFailure,
} from '../../core/levelup/levelup-errors';
import { STEP_LABELS, type LevelUpDone, type StepKey } from '../../core/levelup/levelup-flow';
import { ContentWatcher } from '../../core/content/content-watcher';
import { openSpellDetails } from '../../shared/spell-details/open-spell-details';
import { AbilitiesStep } from './abilities-step/abilities-step';
import { ClassPick } from './class-pick/class-pick';
import { HpStep } from './hp-step/hp-step';
import { LevelUpFocus } from './level-up-focus';
import { LevelUpReader, type PageState } from './level-up-reader';
import { LevelUpSession } from './level-up-session';
import { PicksStep } from './picks-step/picks-step';
import { SideColumn } from './side-column/side-column';
import { SpellsStep } from './spells-step/spells-step';
import { StepsBar } from './steps-bar/steps-bar';
import { SummaryStep } from './summary-step/summary-step';

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
    ClassPick,
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
  private readonly focus = new LevelUpFocus(this.host, this.injector);

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
  /** "Uma classe nova" is open in the class step: the cards of the new classes show. It follows the options
   * when they are a new class's, and stays open while the player has not picked one. */
  protected readonly newMode = signal(false);
  /** The question "Subir em Mago 1?" asked in place of the footer's buttons, for a class new to the sheet. */
  protected readonly confirmingNew = signal(false);
  /** The class a tap asked for while choices were made: the question "Trocar de classe?" is open for it. */
  protected readonly classAsking = signal<string | null>(null);
  /** The options of the other class are being read. */
  protected readonly classBusy = signal(false);
  /** What a screen reader hears after the class changes. */
  protected readonly classStatus = signal('');

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
    if (this.step() === 'class' && this.newMode() && !s?.options.isNewClass) {
      return 'Escolha a classe nova para continuar.';
    }
    const p = s?.preview.state();
    if (
      p &&
      !p.loading &&
      p.refusal &&
      this.step() !== 'summary' &&
      refusalStep(p.refusal.reason, p.refusal.field, s?.options.abilityScoreImprovement) ===
        this.step()
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
    this.route.paramMap.pipe(takeUntilDestroyed(destroyRef)).subscribe((params) => {
      const campaignId = params.get('id');
      const characterId = params.get('characterId');
      if (campaignId && characterId) {
        this.campaignId.set(campaignId);
        this.characterId.set(characterId);
        void this.reader.load(campaignId, characterId);
      }
    });
    this.watcher.whileLive(this.campaignId, () => void this.contentChanged());
    destroyRef.onDestroy(() => {
      this.session()?.stop();
      this.focus.disconnect();
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

  /** What the page reads from the server to make its sessions (see `LevelUpReader`). */
  private readonly reader = new LevelUpReader(this.client, {
    campaignId: () => this.campaignId(),
    characterId: () => this.characterId(),
    session: () => this.session(),
    setState: (state) => this.state.set(state),
    clearFailure: () => this.failure.set(null),
    show: (failure) => this.show(failure),
    describeSpell: (key, name) => this.describeSpell(key, name),
    ready: () => this.focus.firstScreen(),
    classBusy: (busy) => this.classBusy.set(busy),
    classSwitched: (session) => {
      this.index.set(0);
      this.confirmingNew.set(false);
      this.state.set({ status: 'ready', session });
      this.classStatus.set(
        `${session.options.classNamePt} escolhido. O nível tem ${session.draft.steps().length} passos.`,
      );
      this.focus.checkedClass();
    },
    classFailed: (failure) => {
      this.show(failure);
      this.focus.checkedClass();
    },
  });

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
    if (o.isNewClass) {
      const have = s.before.classes.map((c) => `${c.namePt} ${c.level}`).join(' · ');
      return `${s.character.name} · ${have} → ${have} · ${o.classNamePt} 1`;
    }
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
      this.focus.missing(this.missingHere()[0]?.id);
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
    this.focus.clearAttention();
    this.index.set(Math.max(0, Math.min(index, this.steps().length - 1)));
    this.focus.step();
  }

  /** A pick anywhere in the step: the mark and the repeated reason are over. */
  protected onPick(): void {
    this.attempted.set(false);
    this.focus.clearAttention();
  }

  /** The mark on a card goes when the player picks anything, or leaves the step. */
  protected clearAttention(): void {
    this.focus.clearAttention();
  }

  /** "Cancelar" and "Voltar para a ficha": with choices made, ask in place before discarding. */
  protected askToLeave(from: 'top' | 'foot'): void {
    const s = this.session();
    if (!s?.draft.dirty()) {
      void this.leave();
      return;
    }
    this.asking.set(from);
    this.focus.question('.js-keep');
  }

  protected keepChoosing(): void {
    const from = this.asking();
    this.asking.set(null);
    this.focus.afterKeepChoosing(from);
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
    // A class new to the sheet does not come undone: the footer asks first, in place (SRD 5.1 "Multiclassing").
    if (s.options.isNewClass && !this.confirmingNew()) {
      this.confirmingNew.set(true);
      this.focus.question('.js-new-keep');
      return;
    }
    this.confirmingNew.set(false);
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
      const failure = describeLevelUpFailure(err, s.options.abilityScoreImprovement);
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
    this.focus.failure();
  }

  /** What the page says after the table's content changed under the person: that the lists changed, or that a choice left
   * them. It stays empty while what the page shows (the offers and the lists) is the same. */
  protected readonly contentNote = signal('');

  /** `content_changed` (RN-23): the options and the lists are read again, and what the person is told about it goes in the note. */
  private async contentChanged(): Promise<void> {
    const note = await this.reader.contentChanged();
    if (note) {
      this.contentNote.set(note);
    }
  }

  /** "Ler a ficha de novo" after a stale revision: see `LevelUpReader.reread`. */
  protected rereadSheet(afterContent = false): Promise<boolean> {
    return this.reader.reread(afterContent);
  }

  /** A tap on a class card: with nothing chosen yet the level goes to that class at once, else the page asks first. */
  protected pickClass(key: string): void {
    const s = this.session();
    if (!s || this.classBusy()) {
      return;
    }
    const isNew = s.before.classes.every((c) => c.classKey !== key);
    if (key === s.options.classKey) {
      this.newMode.set(isNew);
      return;
    }
    if (!isNew) {
      this.newMode.set(false);
    }
    if (s.draft.dirty()) {
      this.classAsking.set(key);
      this.focus.question('.js-class-keep');
      return;
    }
    void this.reader.switchClass(key);
  }

  /** "Uma classe nova": the cards of the new classes open; the options stay the current class's until one is picked. */
  protected openNewClasses(): void {
    this.newMode.set(true);
  }

  /** "Trocar para o ...": the choices of the draft go, and the options of the other class are read. */
  protected confirmClass(): void {
    const key = this.classAsking();
    this.classAsking.set(null);
    if (key) {
      this.newMode.set(this.session()?.before.classes.every((c) => c.classKey !== key) ?? false);
      void this.reader.switchClass(key);
    }
  }

  /** "Voltar" of the question: nothing is saved, the footer comes back. */
  protected keepNotNew(): void {
    this.confirmingNew.set(false);
    this.focus.nextButton();
  }

  /** "Continuar com o ...": the question closes and everything stays as it was. */
  protected keepClass(): void {
    this.classAsking.set(null);
    this.focus.checkedClass();
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
