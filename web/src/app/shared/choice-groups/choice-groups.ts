import { NgTemplateOutlet } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';

import {
  type Choice,
  ChoiceKind,
  type ChoiceOption,
  ChoiceOrigin,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { ChoiceGroup } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  type ChoiceSection,
  isBlocked,
  isRadio,
  isTraitChoice,
  missingSpell,
  originLine,
  sectionsOf,
} from './choice-picks';

/** A choice with more options than this offers the "only what I can take" filter. */
const LONG_LIST = 12;
/** A spell list longer than this offers a text filter. */
const FILTER_FROM = 8;

/** What the user did on a choice: its whole selection is now `optionKeys`. */
export interface ChoiceSelection {
  readonly choice: Choice;
  readonly optionKeys: readonly string[];
}

/** One of the free texts of a choice (the two humanoid races of a favored enemy), 1-based. */
export interface ChoiceTextEdit {
  readonly choice: Choice;
  readonly n: number;
  readonly text: string;
}

/**
 * The choices a class or a race asks (PM-05), drawn the same way on the editor's "Escolhas" step, on
 * the locked sheet's "Completar escolhas pendentes" page and at the level-up. It shows what the
 * server's `PreviewChoices` says: the groups, the options with the rule in one line, the ones that
 * cannot be taken now, dotted, with the reason (they never disappear), and what a pick gives. It
 * decides nothing: a pick is reported (`selected`), the page keeps the sheet's keys and asks the
 * server again.
 *
 * A choice of one pick is a radio group (arrows choose, Tab leaves the group); several picks are
 * check boxes in a group, the last one blocked once the choice is full ("Já escolheu 3"). A blocked
 * option is `aria-disabled` and still focusable, and its reason is the option's description. The
 * favored enemy's type and language are two lists, the type's humanoid option takes two texts.
 */
@Component({
  selector: 'app-choice-groups',
  imports: [NgTemplateOutlet, MatFormFieldModule, MatIconModule, MatInputModule, MatSelectModule],
  templateUrl: './choice-groups.html',
  styleUrl: './choice-groups.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ChoiceGroups {
  /** What `PreviewChoices` answered. */
  readonly groups = input.required<readonly ChoiceGroup[]>();
  /** "Escolhas feitas: done de total", from the same answer. */
  readonly done = input<number | null>(null);
  readonly total = input<number | null>(null);
  /** The line at the right of the counter: "Tharn · Guerreiro 1 · Draconato". */
  readonly subtitle = input('');
  /** Hides the choices already made (the page that completes the open ones never shows a made one as a field). */
  readonly onlyOpen = input(false);
  /** Whether "Escolher X agora" is offered on an option that asks for a spell the sheet lacks (a locked sheet cannot add one). */
  readonly offerCantrips = input(true);

  readonly selected = output<ChoiceSelection>();
  readonly textEdited = output<ChoiceTextEdit>();
  /** "Escolher Rajada Mística agora": the spell the user wants added to the sheet's cantrips. */
  readonly addCantrip = output<string>();

  protected readonly originLine = originLine;
  protected readonly isBlocked = isBlocked;
  protected readonly isRadio = isRadio;
  protected readonly missingSpell = missingSpell;
  protected readonly kind = ChoiceKind;

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  /** What the user has picked, by choice key: the server's answer until the user changes it. */
  private readonly local = signal<ReadonlyMap<string, readonly string[]>>(new Map());
  private readonly localTexts = signal<ReadonlyMap<string, readonly string[]>>(new Map());
  /** The filter of a long list: only what can be taken, or everything. */
  protected readonly onlyTakable = signal<ReadonlyMap<string, boolean>>(new Map());
  protected readonly spellFilter = signal<ReadonlyMap<string, string>>(new Map());

  protected readonly sections = computed<ChoiceSection[]>(() => {
    const groups = this.groups();
    const shown = this.onlyOpen()
      ? groups
          .map((g) => ({ ...g, choices: g.choices.filter((c) => c.missing > 0) }))
          .filter((g) => g.choices.length > 0)
      : groups;
    return sectionsOf(shown as readonly ChoiceGroup[]);
  });

  /** Every choice of the round with its group, in order: the choices an option may be shared with. */
  private readonly everyChoice = computed(() =>
    this.groups().flatMap((group) => group.choices.map((choice) => ({ choice, group }))),
  );

  /** "Escolhas feitas", as the page sees it now: the server's count less the repeats it has not caught up with yet. */
  protected readonly shownDone = computed(() => {
    const done = this.done();
    if (done === null) {
      return null;
    }
    const unseen = this.everyChoice().reduce(
      (n, { choice }) =>
        n +
        this.pickedKeys(choice).filter(
          (k) => this.earlierHolder(choice, k) && !choice.unmet.includes(k),
        ).length,
      0,
    );
    return Math.max(0, done - unseen);
  });

  constructor() {
    // A new answer from the server is the truth again.
    effect(() => {
      const picked = new Map<string, readonly string[]>();
      const texts = new Map<string, readonly string[]>();
      for (const g of this.groups()) {
        for (const c of g.choices) {
          picked.set(c.key, c.picked);
          texts.set(c.key, c.texts);
        }
      }
      this.local.set(picked);
      this.localTexts.set(texts);
    });
  }

  /**
   * Moves the focus to the title of the first choice that still has something to pick: one that stops the sheet
   * first, then one that only warns (the languages and tools).
   */
  focusFirstPending(): boolean {
    const el = this.host.nativeElement;
    const title =
      el.querySelector<HTMLElement>('[data-pending]:not([data-advisory]) .choice__title') ??
      el.querySelector<HTMLElement>('[data-pending] .choice__title');
    title?.focus();
    return title !== null;
  }

  protected pickedKeys(choice: Choice): readonly string[] {
    return this.local().get(choice.key) ?? choice.picked;
  }

  protected firstKey(choice: Choice): string {
    return this.pickedKeys(choice)[0] ?? '';
  }

  protected isPicked(choice: Choice, option: ChoiceOption): boolean {
    return this.pickedKeys(choice).includes(option.key);
  }

  /** The earlier choice of the round that offers the option `key` and holds it, if any: this pick is a repeat of that one. */
  private earlierHolder(choice: Choice, key: string): Choice | undefined {
    for (const { choice: other } of this.everyChoice()) {
      if (other.key === choice.key) {
        return undefined;
      }
      if (this.holds(other, key)) {
        return other;
      }
    }
    return undefined;
  }

  /** Whether the choice offers the option `key` and has it picked. */
  private holds(choice: Choice, key: string): boolean {
    return this.pickedKeys(choice).includes(key) && choice.options.some((o) => o.key === key);
  }

  /** Whether another choice of the round holds the option, so this one must not take it. */
  protected isTaken(choice: Choice, option: ChoiceOption): boolean {
    return (
      !this.isPicked(choice, option) &&
      this.everyChoice().some(
        ({ choice: other }) => other.key !== choice.key && this.holds(other, option.key),
      )
    );
  }

  /** The picks of the choice that repeat one an earlier choice holds, or that the server says cannot stand. */
  private repeats(choice: Choice): string[] {
    return this.pickedKeys(choice).filter(
      (k) => choice.unmet.includes(k) || this.earlierHolder(choice, k) !== undefined,
    );
  }

  /** "Abissal já foi escolhido em Idioma (raça).", for each pick that repeats an earlier choice's. */
  protected repeatLines(choice: Choice): string[] {
    const lines: string[] = [];
    for (const key of this.pickedKeys(choice)) {
      const earlier = this.earlierHolder(choice, key);
      const name = choice.options.find((o) => o.key === key)?.namePt;
      if (earlier && name) {
        lines.push(`${name} já foi escolhido em ${this.where(earlier)}.`);
      }
    }
    return lines;
  }

  private where(choice: Choice): string {
    const group = this.everyChoice().find((e) => e.choice.key === choice.key)?.group;
    switch (group?.origin) {
      case ChoiceOrigin.RACE:
        return `${choice.titlePt} (raça)`;
      case ChoiceOrigin.BACKGROUND:
        return `${choice.titlePt} (antecedente)`;
      case ChoiceOrigin.FEAT:
        return `${choice.titlePt} (talento)`;
      default:
        return choice.titlePt;
    }
  }

  /** Why an option cannot be taken, in one line. */
  protected reason(option: ChoiceOption): string {
    return isBlocked(option) ? option.reasonPt : 'Você já escolheu esta opção.';
  }

  protected count(choice: Choice): number {
    const kept = this.pickedKeys(choice).length - this.repeats(choice).length;
    const n = Math.min(kept, choice.picks);
    return choice.kind === ChoiceKind.ENEMY &&
      n > 0 &&
      this.needsTexts(choice) &&
      !this.textsComplete(choice)
      ? n - 1
      : n;
  }

  protected isDone(section: ChoiceSection): boolean {
    return section.choices.every((c) => this.count(c) >= c.picks);
  }

  protected progress(section: ChoiceSection): string {
    const picks = section.choices.reduce((n, c) => n + c.picks, 0);
    const done = section.choices.reduce((n, c) => n + this.count(c), 0);
    // A trait's choice shows its progress even with one pick.
    return picks > 1 || isTraitChoice(section.group, section.choices[0])
      ? `${done} de ${picks}`
      : '';
  }

  /** Whether an unpicked option cannot be taken because the choice already holds all its picks. */
  protected isFull(choice: Choice): boolean {
    return choice.picks > 1 && this.pickedKeys(choice).length >= choice.picks;
  }

  /** Whether an option cannot be clicked: one that is picked never is, so it can always be unticked. */
  protected isOff(choice: Choice, option: ChoiceOption): boolean {
    if (this.isPicked(choice, option)) {
      return false;
    }
    return (
      isBlocked(option) || this.isTaken(choice, option) || (this.isFull(choice) && !isRadio(choice))
    );
  }

  protected titleId(choice: Choice): string {
    return `choice-${this.slug(choice.key)}`;
  }

  protected optionId(choice: Choice, option: ChoiceOption): string {
    return `${this.titleId(choice)}-${this.slug(option.key)}`;
  }

  private slug(key: string): string {
    return key.replace(/[^a-z0-9]+/gi, '-');
  }

  protected sectionTitle(section: ChoiceSection): string {
    return section.choices[0].titlePt;
  }

  protected origin(section: ChoiceSection): string {
    return originLine(section.group, section.choices[0]);
  }

  /** The options in the order shown: for a long list with blocked options, the ones that can be taken first. */
  protected visibleOptions(choice: Choice): { takable: ChoiceOption[]; blocked: ChoiceOption[] } {
    const parked = (o: ChoiceOption) => isBlocked(o) && !this.isPicked(choice, o);
    const split = choice.options.length > LONG_LIST && choice.options.some(parked);
    const byName = (a: ChoiceOption, b: ChoiceOption) => a.namePt.localeCompare(b.namePt, 'pt-BR');
    if (!split) {
      return { takable: this.filtered(choice, [...choice.options]), blocked: [] };
    }
    return {
      takable: this.filtered(choice, choice.options.filter((o) => !parked(o)).sort(byName)),
      blocked: this.onlyTakable().get(choice.key) ? [] : choice.options.filter(parked).sort(byName),
    };
  }

  protected takableCount(choice: Choice): number {
    return choice.options.filter((o) => !isBlocked(o)).length;
  }

  protected hasFilter(choice: Choice): boolean {
    return choice.options.length > LONG_LIST && choice.options.some(isBlocked);
  }

  protected setTakable(choice: Choice, only: boolean): void {
    this.onlyTakable.update((m) => new Map(m).set(choice.key, only));
  }

  private filtered(choice: Choice, options: ChoiceOption[]): ChoiceOption[] {
    const q = (this.spellFilter().get(choice.key) ?? '').trim().toLowerCase();
    return q === '' ? options : options.filter((o) => o.namePt.toLowerCase().includes(q));
  }

  protected hasSpellFilter(choice: Choice): boolean {
    return choice.kind === ChoiceKind.SPELLS && choice.options.length > FILTER_FROM;
  }

  protected setSpellFilter(choice: Choice, value: string): void {
    this.spellFilter.update((m) => new Map(m).set(choice.key, value));
  }

  protected filterValue(choice: Choice): string {
    return this.spellFilter().get(choice.key) ?? '';
  }

  protected tabIndex(choice: Choice, option: ChoiceOption): number {
    if (!isRadio(choice)) {
      return 0;
    }
    const picked = this.pickedKeys(choice);
    const anchor = picked[0] ?? choice.options.find((o) => !isBlocked(o))?.key;
    return option.key === anchor ? 0 : -1;
  }

  protected toggle(choice: Choice, option: ChoiceOption): void {
    if (this.isOff(choice, option)) {
      return;
    }
    const now = this.pickedKeys(choice);
    let next: readonly string[];
    if (isRadio(choice)) {
      next = [option.key];
    } else if (now.includes(option.key)) {
      next = now.filter((k) => k !== option.key);
    } else {
      next = [...now, option.key];
    }
    this.commit(choice, next);
  }

  protected choose(choice: Choice, key: string): void {
    this.commit(choice, key === '' ? [] : [key]);
  }

  private commit(choice: Choice, next: readonly string[]): void {
    this.local.update((m) => new Map(m).set(choice.key, next));
    this.selected.emit({ choice, optionKeys: next });
  }

  /** Arrows move among a radio group's options and pick the one they land on, unless it is blocked. */
  protected onKey(event: KeyboardEvent, choice: Choice, option: ChoiceOption): void {
    if (event.key === ' ' || event.key === 'Enter') {
      event.preventDefault();
      this.toggle(choice, option);
      return;
    }
    if (!isRadio(choice)) {
      return;
    }
    const step =
      event.key === 'ArrowDown' || event.key === 'ArrowRight'
        ? 1
        : event.key === 'ArrowUp' || event.key === 'ArrowLeft'
          ? -1
          : 0;
    if (step === 0) {
      return;
    }
    event.preventDefault();
    const options = this.visibleOptions(choice);
    const all = [...options.takable, ...options.blocked];
    const at = all.findIndex((o) => o.key === option.key);
    const target = all[(at + step + all.length) % all.length];
    this.host.nativeElement
      .querySelector<HTMLElement>(`#${this.optionId(choice, target)}`)
      ?.focus();
    if (!isBlocked(target)) {
      this.commit(choice, [target.key]);
    }
  }

  protected selectedOption(choice: Choice): ChoiceOption | undefined {
    const key = this.pickedKeys(choice)[0];
    return choice.options.find((o) => o.key === key);
  }

  protected needsTexts(choice: Choice): boolean {
    return this.pickedKeys(choice).some((k) => choice.options.find((o) => o.key === k)?.needsText);
  }

  protected text(choice: Choice, n: number): string {
    return (this.localTexts().get(choice.key) ?? choice.texts)[n - 1] ?? '';
  }

  private textsComplete(choice: Choice): boolean {
    return [1, 2].every((n) => this.text(choice, n).trim() !== '');
  }

  protected editText(choice: Choice, n: number, text: string): void {
    this.localTexts.update((m) => {
      const next = [...(m.get(choice.key) ?? choice.texts)];
      while (next.length < 2) {
        next.push('');
      }
      next[n - 1] = text;
      return new Map(m).set(choice.key, next);
    });
    this.textEdited.emit({ choice, n, text });
  }

  protected circleSpellsOf(choice: Choice): { level: number; names: string; reached: boolean }[] {
    const option = this.selectedOption(choice);
    const rows = new Map<number, { names: string[]; reached: boolean }>();
    for (const s of option?.circleSpells ?? []) {
      const row = rows.get(s.level) ?? { names: [], reached: s.reached };
      row.names.push(s.namePt);
      rows.set(s.level, row);
    }
    return [...rows.entries()].map(([level, r]) => ({
      level,
      names: r.names.join(', '),
      reached: r.reached,
    }));
  }

  protected spellLine(option: ChoiceOption): string {
    const level = option.spellLevel === 0 ? 'Truque' : `${option.spellLevel}º nível`;
    return [level, option.classesPt.join(', ')].filter((p) => p !== '').join(' · ');
  }

  protected fullText(choice: Choice): string {
    return `Já escolheu ${choice.picks}. Desmarque uma para trocar.`;
  }
}
