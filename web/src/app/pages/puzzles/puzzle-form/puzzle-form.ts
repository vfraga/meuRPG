import {
  ChangeDetectionStrategy,
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
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  type Draft,
  type FormKind,
  CLUE_MAX,
  JUDGED_KINDS,
  NAME_MAX,
  FORM_KINDS,
  NO_WRONG,
  draftErrors,
  draftOf,
  isValid,
  wrongTarget,
  wrongTargetOf,
  newDraft,
  protoKindOf,
  startRequestOf,
  textLength,
  toInit,
} from '../../../core/puzzles/puzzle-draft';
import { kindIcon, kindName } from '../../../core/puzzles/puzzle-format';
import {
  type FormSection,
  invalidSection,
  puzzleErrorMessage,
  puzzleInvalid,
} from '../../../core/puzzles/puzzle-errors';
import { type PuzzleAccess, PuzzleAccessCheck } from '../../../core/puzzles/puzzle-access';
import { ActionKey } from '../../../core/connect/idempotency';
import { PuzzlesClient } from '../../../core/puzzles/puzzles-client';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { HintCheckField } from '../fields/hint-check-field';
import { HintsField } from '../fields/hints-field';
import { type PickOption, PickGroup } from '../fields/pick-group';
import { PartsField } from '../fields/parts-field';
import { SolveField } from '../fields/solve-field';
import { WrongField } from '../fields/wrong-field';
import { CipherForm } from './cipher-form';
import { LightsForm } from './lights-form';
import { LockForm } from './lock-form';
import { PillarsForm } from './pillars-form';
import { RiddleForm } from './riddle-form';
import { SequenceForm } from './sequence-form';
import { NO_PREVIEW, type StartPreview } from './start-preview';

type Load =
  | { readonly status: 'loading' }
  | { readonly status: 'ready' }
  | { readonly status: 'blocked'; readonly message: string };

/** How long a change of the board waits before the server draws a start (the master is still clicking). */
const PREVIEW_PAUSE_MS = 300;

const KIND_OPTIONS: readonly PickOption<FormKind>[] = FORM_KINDS.map((kind) => ({
  value: kind,
  title: kindName(protoKindOf(kind)),
  icon: kindIcon(protoKindOf(kind)),
  sub:
    kind === 'lights'
      ? 'Uma grade de luzes. Cada toque troca a luz e as quatro vizinhas.'
      : kind === 'lock'
        ? 'Rodas de dígitos, letras ou runas. Você escolhe a solução.'
        : kind === 'pillars'
          ? 'Pilares com símbolos que giram, para copiar um mural.'
          : kind === 'riddle'
            ? 'Um enigma que os jogadores respondem por escrito.'
            : kind === 'sequence'
              ? 'Sinos que você toca e os jogadores repetem, na mesma ordem.'
              : 'Uma mensagem de letras trocadas, que eles decifram.',
}));

/**
 * "/campaigns/:id/puzzles/new" and ".../:puzzleId/edit" (MR-038, E10-06 state 2): the master makes or edits a puzzle of
 * the first three kinds. The kind first (a new puzzle only), then that kind's form, the clue, the hints and "Ao resolver".
 *
 * - **The start comes from the server.** The lights and the pillars start from a seed the server draws (`PreviewPuzzleStart`):
 *   a change of the size, the links or the mural asks for a new one after a short pause, "Gerar outro começo" asks for another
 *   at once, and the seed on screen is the seed saved, so the puzzle starts exactly as the master saw. The browser never draws
 *   a start or works out a minimum. A lock's start is the master's own.
 * - **An edit keeps the start** (seed 0 on the call) until the master changes something that asks for another.
 * - **A puzzle already shown cannot be edited** (the server refuses): the page says so instead of a form that would fail.
 * - The server's refusals come back as words by code and typed detail, never by message.
 */
@Component({
  selector: 'app-puzzle-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    CipherForm,
    HintCheckField,
    HintsField,
    LightsForm,
    LockForm,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatProgressSpinnerModule,
    PartsField,
    PickGroup,
    PillarsForm,
    RiddleForm,
    RouterLink,
    SequenceForm,
    SolveField,
    WrongField,
  ],
  templateUrl: './puzzle-form.html',
  styleUrl: './puzzle-form.scss',
})
export class PuzzleForm {
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly api = inject(PuzzlesClient);
  private readonly createKey = new ActionKey();
  private readonly accessCheck = inject(PuzzleAccessCheck);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  protected readonly campaignId = this.route.snapshot.paramMap.get('id') ?? '';
  /** The puzzle being edited, or `''` for a new one. */
  protected readonly puzzleId = this.route.snapshot.paramMap.get('puzzleId') ?? '';
  protected readonly editing = this.puzzleId !== '';
  protected readonly kindOptions = KIND_OPTIONS;
  protected readonly nameMax = NAME_MAX;
  protected readonly clueMax = CLUE_MAX;

  protected readonly access = signal<PuzzleAccess | { status: 'loading' }>({ status: 'loading' });
  protected readonly load = signal<Load>({ status: 'loading' });
  protected readonly draft = signal<Draft>(newDraft('lights'));
  protected readonly preview = signal<StartPreview>(NO_PREVIEW);
  protected readonly submitted = signal(false);
  protected readonly saving = signal(false);
  protected readonly notice = signal('');
  protected readonly nameServerError = signal('');
  /** What the server refused, by the part of the form it belongs to (`invalidSection`). It clears when the master changes anything. */
  protected readonly serverErrors = signal<Partial<Record<FormSection, string>>>({});
  /** The request field the server's refusal named (`parts[1].character_id`, `on_wrong.max_moves`...): it picks the field the words stand under. */
  protected readonly serverField = signal('');

  protected readonly errors = computed(() => draftErrors(this.draft()));
  protected readonly nameError = computed(() =>
    this.submitted() ? (this.errors().name ?? this.nameServerError()) : this.nameServerError(),
  );
  /** What is wrong in a part of the form: the form's own check once the master tried to save, or the server's refusal. */
  private shown(section: FormSection, own: string | undefined): string {
    return (this.submitted() ? own : undefined) || this.serverErrors()[section] || '';
  }
  protected readonly clueError = computed(() => this.shown('clue', this.errors().clue));
  protected readonly hintErrors = computed(() => (this.submitted() ? this.errors().hints : {}));
  protected readonly targetError = computed(() => this.shown('target', this.errors().target));
  protected readonly messageError = computed(() => this.shown('message', this.errors().message));
  protected readonly lockError = computed(() =>
    this.submitted() ? (this.errors().lock ?? '') : '',
  );
  protected readonly riddleError = computed(() => this.shown('riddle', this.errors().riddle));
  protected readonly answersError = computed(() => this.shown('answers', this.errors().answers));
  protected readonly answerRows = computed(() =>
    this.submitted() ? this.errors().answerRows : {},
  );
  protected readonly sequenceError = computed(() => this.shown('sequence', this.errors().sequence));
  // The server says only "solution.cipher" for a message or a key it does not take: the words stand under the key, which the master chooses last.
  protected readonly cipherMessageError = computed(() =>
    this.submitted() ? (this.errors().cipherMessage ?? '') : '',
  );
  protected readonly cipherKeyError = computed(
    () =>
      (this.submitted() ? (this.errors().cipherKey ?? '') : '') ||
      this.serverErrors().cipherMessage ||
      '',
  );
  protected readonly checkError = computed(() => this.shown('check', this.errors().check));
  protected readonly wrongError = computed(() => this.shown('wrong', this.errors().wrong));
  protected readonly partsErrors = computed<
    Readonly<Record<number, { owner?: string; text?: string }>>
  >(() => {
    const server = this.serverErrors().parts;
    const own = this.submitted() ? this.errors().parts : {};
    const found = /^parts\[(\d+)\]\.(character_id|text)$/.exec(this.serverField());
    if (server && Object.keys(own).length === 0 && found) {
      // The refusal stands on the part it names: its owner or its text.
      return { [Number(found[1])]: found[2] === 'text' ? { text: server } : { owner: server } };
    }
    return own;
  });
  /** A refusal about the parts as a whole ("at most 8"), under the list. */
  protected readonly partsError = computed(() =>
    /^parts\[\d+\]/.test(this.serverField()) ? '' : (this.serverErrors().parts ?? ''),
  );
  protected readonly wrongField = computed(() =>
    this.serverErrors().wrong
      ? wrongTargetOf(this.serverField())
      : this.submitted() && this.errors().wrong
        ? wrongTarget(this.draft())
        : '',
  );
  protected readonly judged = computed(() => JUDGED_KINDS.includes(this.draft().kind));
  /** The message and the key are good enough to cipher: the form's own checks pass. */
  protected readonly cipherable = computed(
    () => !this.errors().cipherMessage && !this.errors().cipherKey,
  );
  protected readonly kindTitle = computed(() => kindName(protoKindOf(this.draft().kind)));
  /** A generated start has to be on screen before the puzzle can be saved: what is saved is what the master saw. */
  protected readonly startReady = computed(
    () => !['lights', 'pillars'].includes(this.draft().kind) || this.preview().status === 'ready',
  );
  protected readonly nameField = viewChild<ElementRef<HTMLInputElement>>('nameField');

  /** What the preview depends on: a change of it asks the server for a new start. */
  private readonly previewKey = computed(() => {
    const d = this.draft();
    const request = startRequestOf(d);
    return request
      ? JSON.stringify([d.kind, request.config, request.solution], (_, v) =>
          typeof v === 'bigint' ? String(v) : v,
        )
      : '';
  });
  private lastKey = '';
  private timer: ReturnType<typeof setTimeout> | undefined;
  private ticket = 0;

  constructor() {
    inject(DestroyRef).onDestroy(() => clearTimeout(this.timer));
    // A change of what the start depends on (not the first one of an edit, which has its own start) asks for a new start.
    effect(() => {
      const key = this.previewKey();
      // Read outside `untracked`: the first start is asked for when the page turns ready, not before.
      const ready = this.load().status === 'ready';
      untracked(() => {
        if (key === '') {
          // A kind with no generated start: coming back to one with the same configuration asks for the start again.
          this.lastKey = '';
          this.ticket++;
          clearTimeout(this.timer);
          return;
        }
        if (key === this.lastKey || !ready) {
          return;
        }
        this.lastKey = key;
        // A preview still on its way is for the old configuration: it is never shown or saved.
        this.ticket++;
        clearTimeout(this.timer);
        this.preview.update((p) => ({ ...p, status: 'loading', message: '' }));
        this.timer = setTimeout(() => void this.runPreview(), PREVIEW_PAUSE_MS);
      });
    });
    void this.start();
  }

  private async start(): Promise<void> {
    const access = await this.accessCheck.check(this.campaignId);
    this.access.set(access);
    if (access.status !== 'master') {
      return;
    }
    if (!this.editing) {
      this.load.set({ status: 'ready' });
      return;
    }
    try {
      const puzzle = await this.api.get(this.campaignId, this.puzzleId);
      const draft = draftOf(puzzle);
      if (draft === null) {
        this.load.set({
          status: 'blocked',
          message: 'Este tipo de quebra-cabeça se edita em outra tela.',
        });
        return;
      }
      if (puzzle.shown) {
        this.load.set({
          status: 'blocked',
          message:
            'Este quebra-cabeça já foi mostrado numa sessão e não pode mais ser editado. Faça outro, ou arquive este.',
        });
        return;
      }
      // The puzzle's own start is the preview: nothing is asked of the server until something it depends on changes.
      this.draft.set(draft);
      this.lastKey = this.previewKey();
      if (draft.kind !== 'lock') {
        this.preview.set({
          status: 'ready',
          start: puzzle.start,
          moves: puzzle.minimum?.moves ?? 0,
          solvable: puzzle.minimum?.solvable ?? true,
          message: '',
        });
      }
      this.load.set({ status: 'ready' });
    } catch (err) {
      this.load.set({
        status: 'blocked',
        message: puzzleErrorMessage(err, 'abrir o quebra-cabeça'),
      });
    }
  }

  protected retry(): void {
    this.access.set({ status: 'loading' });
    this.load.set({ status: 'loading' });
    void this.start();
  }

  protected patch(partial: Partial<Draft>): void {
    this.draft.update((d) => ({ ...d, ...partial }));
    this.nameServerError.set('');
    this.serverErrors.set({});
    this.serverField.set('');
  }

  /** Another kind starts that kind's form over; what is common (name, clue, hints, "Ao resolver") stays. */
  protected setKind(kind: FormKind): void {
    const d = this.draft();
    // A trap and an attempt need a move that is judged: the other kinds keep "Nada acontece" or the limits.
    const keepWrong =
      JUDGED_KINDS.includes(kind) || d.wrong.option === 'none' || d.wrong.option === 'limits';
    this.draft.set({
      ...newDraft(kind),
      name: d.name,
      clue: d.clue,
      hints: d.hints,
      solve: d.solve,
      hintCheck: d.hintCheck,
      parts: d.parts,
      wrong: keepWrong ? d.wrong : NO_WRONG,
    });
    this.preview.set(NO_PREVIEW);
    this.serverErrors.set({});
  }

  /** "Gerar outro começo": a start the server draws now, with a new seed. */
  protected again(): void {
    clearTimeout(this.timer);
    this.preview.update((p) => ({ ...p, status: 'loading', message: '' }));
    void this.runPreview();
  }

  private async runPreview(): Promise<void> {
    const request = startRequestOf(this.draft());
    if (!request) {
      return;
    }
    const ticket = ++this.ticket;
    try {
      const answer = await this.api.previewStart(
        this.campaignId,
        request.config,
        request.solution,
        0n,
      );
      if (ticket !== this.ticket) {
        return;
      }
      this.preview.set({
        status: 'ready',
        start: answer.start,
        moves: answer.minimum?.moves ?? 0,
        solvable: answer.minimum?.solvable ?? true,
        message: '',
      });
      this.patch({ seed: answer.seed });
    } catch (err) {
      if (ticket === this.ticket) {
        this.preview.set({
          ...NO_PREVIEW,
          status: 'error',
          message: puzzleErrorMessage(err, 'sortear um começo'),
        });
      }
    }
  }

  protected async save(): Promise<void> {
    this.submitted.set(true);
    this.notice.set('');
    if (!isValid(this.errors()) || !this.startReady()) {
      this.focusFirstProblem();
      return;
    }
    this.saving.set(true);
    try {
      const init = toInit(this.draft());
      if (this.editing) {
        await this.api.update(this.campaignId, this.puzzleId, init);
      } else {
        // A retry of the same form (a lost answer, a second tap) sends the same key and makes one puzzle.
        await this.api.create(this.campaignId, init, this.createKey.keyFor(init));
      }
      await this.router.navigate(['/campaigns', this.campaignId]);
    } catch (err) {
      const invalid = puzzleInvalid(err);
      const section = invalid ? invalidSection(invalid) : '';
      if (section === 'name') {
        this.nameServerError.set(puzzleErrorMessage(err));
        this.focusName();
      } else if (section !== '' && section !== 'hints') {
        // The refusal stands under the field it belongs to, in words, and the focus goes there.
        this.serverErrors.set({ [section]: puzzleErrorMessage(err) });
        this.serverField.set(invalid?.field ?? '');
        afterNextRender(() => this.focusFirstProblem(), { injector: this.injector });
      } else {
        this.notice.set(puzzleErrorMessage(err, 'salvar o quebra-cabeça'));
      }
      if (ConnectError.from(err, Code.Unavailable).code === Code.FailedPrecondition) {
        // Shown meanwhile, or archived: the form cannot save it any more.
        this.notice.set(puzzleErrorMessage(err, 'salvar o quebra-cabeça'));
      }
    } finally {
      this.saving.set(false);
    }
  }

  private focusFirstProblem(): void {
    if (this.errors().name) {
      this.focusName();
      return;
    }
    const el = this.host.nativeElement.querySelector<HTMLElement>(
      '[aria-invalid="true"], .field-bad input, .field-bad textarea, .field-bad select, .field-error',
    );
    // `focusWithRing` does not scroll (it must not jump a question under the bar); here the jump is the point, to the field's own margin.
    el?.scrollIntoView?.({ block: 'nearest' });
    focusWithRing(el);
  }

  private focusName(): void {
    this.nameField()?.nativeElement.focus();
  }

  protected length(text: string): number {
    return textLength(text.trim());
  }

  protected submit(event: Event): void {
    event.preventDefault();
    void this.save();
  }
}
