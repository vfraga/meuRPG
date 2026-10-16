import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { MapPointKind } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import type { SearchForTrapsResponse } from '../../../../../gen/meurpg/play/v1/traps_pb';
import {
  ResourceClient,
  type InspirationRoll,
  type LuckyRoll,
} from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import { InspirationOffer } from '../../inspiration/inspiration-offer';
import { LuckyOfferCard } from '../../inspiration/lucky-offer';
import { luckyText } from '../../../../core/combat/lucky';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import type {
  DiceRoll,
  LuckyOffer,
  OutsideInspirationOffer,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  parseFace,
  treatedFormula,
  treatedPreview,
  typedTotal,
} from '../../../../core/combat/combat-dice';
import { RollAnimator, showOfDice } from '../../../../shared/roll-overlay/roll-animator';
import { ActionKey } from '../../../../core/connect/idempotency';
import type { MapState } from '../../../../core/maps/map-state';
import { hasModeInfo, needsTwoD20, searchFaces } from '../../../../core/play/check-roll';
import { ExtraDiceState } from '../../../../core/effects/extra-dice-state';
import { needsTwoDice, trapErrorMessage } from '../../../../core/traps/trap-errors';
import {
  type OtherSkill,
  SEARCH_STEPS,
  SPECIFIC_SKILL_NOTE,
  type SearchSkills,
  otherTitle,
  resultMessage,
  searchStep,
  skillName,
  skillOptions,
} from '../../../../core/traps/trap-search';
import { type SearchDie, type SearchSkill, TrapsClient } from '../../../../core/traps/traps-client';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';
import { CheckMode } from '../../scene/check-mode/check-mode';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';

/** What the page hands "Procurar armadilhas". */
export interface TrapSearchData {
  readonly campaignId: string;
  /** The character's bonuses in the two skills (the derived sheet); `null` while unknown. */
  readonly skills: SearchSkills | null;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** The map the player sees: read again after the search, so the trap found shows by name. */
  readonly state: MapState;
  /** The search is the Search action of the character's turn: it spends the action. */
  readonly inCombat: boolean;
}

/** "Procurar armadilhas": a bottom sheet on a phone, a dialog from a tablet up. It answers `true` when a search was made. */
export function openTrapSearch(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: TrapSearchData,
): Observable<boolean | undefined> {
  return openSheet<TrapSearchSheet, TrapSearchData, boolean>(dialog, bottomSheet, TrapSearchSheet, {
    data,
    ariaLabel: 'Procurar armadilhas',
    labelledBy: 'trap-search-t',
    width: '560px',
    focus: 'input[type=radio]',
    restoreFocus: false, // the opener takes it back with the focus ring
  });
}

/**
 * "Procurar armadilhas" (E9-08 A to C and H, MR-035, question 71, RN-18): the player picks **Percepção**,
 * **Investigação** or "Outra perícia…" (any other skill of the sheet, with its bonus) with the character's own bonus, then rolls (in the app, or types the face of the real d20, as
 * the campaign's dice setting allows) and reads the answer: "Você achou uma armadilha: Fosso escondido." or
 * "Você não encontrou nada." The same words for a roll that fell short and for no trap at all: the
 * helper line says only that "algumas armadilhas só se acham com Investigação", so the player learns
 * nothing about a particular trap. The three steps (Como, Rolar, Resultado) are drawn above; the roll
 * buttons are the combat's `RollPicker`. A Perception search with a typed die in dim light needs a second die
 * (`SEARCH_NEEDS_TWO_DICE`): the sheet asks for it with the reason, and the lower of the two counts at
 * the dim squares. In a combat it is the Search action and says so. Focus: the first radio; the result's
 * "Fechar".
 */
@Component({
  selector: 'app-trap-search-sheet',
  imports: [
    CheckMode,
    ExtraDice,
    InspirationOffer,
    LuckyOfferCard,
    MatButtonModule,
    MatIconModule,
    SheetFrame,
  ],
  templateUrl: './trap-search-sheet.html',
  styleUrl: './trap-search-sheet.scss',
})
export class TrapSearchSheet {
  private readonly api = inject(TrapsClient);
  /** Read when the player answers about the die: a sheet that never asks does not build the client. */
  private readonly lazyInjector = inject(Injector);
  private get resources(): ResourceClient {
    return this.lazyInjector.get(ResourceClient);
  }
  private readonly animator = inject(RollAnimator);
  private readonly injector = inject(Injector);
  private readonly sheet = injectSheet<TrapSearchData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly steps = SEARCH_STEPS;
  protected readonly options = skillOptions(this.data.skills);
  protected readonly note = SPECIFIC_SKILL_NOTE;
  protected readonly skill = signal<SearchSkill>('perception');
  /** "Outra perícia…": the other skills of the sheet, always the same list, and the one picked (none until the player picks). */
  protected readonly others: readonly OtherSkill[] = this.data.skills?.others ?? [];
  protected readonly otherKey = signal<string | null>(null);
  protected readonly otherPicked = computed(
    () => this.others.find((o) => o.key === this.otherKey()) ?? null,
  );
  /** The cursor of the list (its index): the arrows move it, Enter picks. */
  protected readonly otherCursor = signal(0);
  protected readonly otherTitle = otherTitle;
  protected readonly chosen = computed(() => {
    const picked = this.otherPicked();
    if (this.skill() === 'other') {
      return {
        skill: 'other' as const,
        title: picked ? otherTitle(picked) : 'Outra perícia…',
        detail: '',
        bonus: picked?.bonus ?? 0,
      };
    }
    return this.options.find((o) => o.skill === this.skill()) ?? this.options[0];
  });
  /** Under "Outra perícia…" nothing is rolled until a skill is picked. */
  protected readonly ready = computed(
    () => this.skill() !== 'other' || this.otherPicked() !== null,
  );
  protected readonly typing = signal(false);
  /** The server asked for a second die: the first face typed waits here. */
  protected readonly firstFace = signal<number | null>(null);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  /** The d4 an effect adds (Orientação, Bênção) with physical dice: asked when the server says the roll takes them. */
  protected readonly extra = new ExtraDiceState();
  /** The d20 was rolled and kept: the character holds a Bardic Inspiration die, and the player decides before the result. */
  protected readonly offer = signal<OutsideInspirationOffer | null>(null);
  /** The d20 was rolled and kept because a halfling's d20 shows a 1 ("Tirou 1: rolar de novo?"): asked before the die's. */
  protected readonly luckyOffer = signal<LuckyOffer | null>(null);
  private readonly dieKey = new ActionKey();
  /** One key per Lucky answer (roll again this way, or keep the 1): the same answer again is a retry. */
  private readonly luckyKey = new ActionKey();
  protected readonly result = signal<{
    res: SearchForTrapsResponse;
    names: readonly string[];
    skill: SearchSkill;
    /** The skill's own name, for "(Arcanismo)" after the bonus. */
    skillLabel: string;
  } | null>(null);
  /** The server said a trap was found, whether or not its name could be read. */
  protected readonly found = computed(() => (this.result()?.res.foundPointIds.length ?? 0) > 0);
  protected readonly step = computed(() => searchStep(this.result() !== null, this.typing()));

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;

  protected readonly message = computed(() =>
    resultMessage(this.result()?.names ?? [], this.result()?.res.foundPointIds.length ?? 0),
  );
  /** What the server rolled, written as it counts: "1d20 (13) + 4 (Investigação) = 17". */
  protected readonly rolls = computed(() => {
    const r = this.result();
    return r
      ? [r.res.roll, r.res.secondRoll]
          .filter((x) => x !== undefined)
          .map((x) => this.formula(x, r.skillLabel))
      : [];
  });
  /** "Sortudo: 1 → 14": the held d20 the halfling's Lucky rolled again; the formula is already the new one. */
  protected readonly lucky = computed(() => luckyText(this.result()?.res.roll));
  /** Both dice of a search with advantage or disadvantage, the one that counts marked. */
  protected readonly faces = computed(() => {
    const res = this.result()?.res;
    return res ? searchFaces(res.mode, res.roll, res.secondRoll) : [];
  });
  protected readonly modeInfo = computed(() => {
    const res = this.result()?.res;
    return !!res && hasModeInfo(res.mode, res.sources, this.faces());
  });
  protected readonly total = computed(() => {
    const r = this.result()?.res;
    return [r?.roll, r?.secondRoll]
      .filter((x) => x !== undefined)
      .map((x) => x.total)
      .join(' · ');
  });
  protected readonly skillWord = computed(
    () => this.result()?.skillLabel ?? skillName(this.skill(), this.otherPicked()?.name),
  );
  protected readonly label = computed(() =>
    this.firstFace() !== null
      ? 'Digite o segundo dado'
      : `Role 1d20 para ${skillName(this.skill(), this.otherPicked()?.name)}${this.data.skills ? ` (${this.chosen().title.replace(/^.*\s(?=[+−])/, '')})` : ''}`,
  );
  protected readonly hint = computed(() =>
    this.firstFace() !== null
      ? 'Esta procura leva dois d20: há penumbra por perto, ou o seu personagem tem vantagem ou desvantagem. Role de novo e digite o segundo dado (1 a 20); o app diz qual vale.'
      : 'Role o seu dado e digite o número que saiu (1 a 20).',
  );
  /** What is typed in the field, and the face it makes (1 to 20) or `null`. */
  protected readonly text = signal('');
  protected readonly face = computed(() => parseFace(this.text()));
  protected readonly invalid = computed(() => this.text().trim() !== '' && this.face() === null);
  protected readonly typedLine = computed(() => {
    const f = this.face();
    if (f === null) {
      return '';
    }
    // Talento Confiável: the server owns the rule; the sheet knows the feature and the skill, so it shows the account (6 → 10).
    const picked = this.skill() === 'other' ? this.otherKey() : `skill:${this.skill()}`;
    const raised =
      picked && this.data.skills?.reliableTalent?.includes(picked)
        ? treatedPreview(f, this.chosen().bonus, 'feature:reliable-talent')
        : null;
    return raised?.text ?? typedTotal(f, this.chosen().bonus);
  });
  protected readonly confirmLabel = computed(() =>
    this.face() === null ? 'Confirmar' : `Confirmar ${this.face()}`,
  );

  private readonly key = new ActionKey();
  private readonly frame = viewChild(SheetFrame);
  private readonly done = viewChild('done', { read: ElementRef<HTMLButtonElement> });
  private readonly field = viewChild('field', { read: ElementRef<HTMLInputElement> });
  private readonly list = viewChild('list', { read: ElementRef<HTMLElement> });
  protected readonly title = 'Procurar armadilhas';

  constructor() {
    effect(() => this.done()?.nativeElement.focus());
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
  }

  /** "1d20 (13) + 4 (Investigação) = 17" for the server's roll; with Talento Confiável "d20: 6 → 10 (Talento Confiável) + 9 (Acrobacia) = 19". */
  private formula(roll: DiceRoll, skill: string): string {
    const treated = treatedFormula(roll, skill);
    if (treated) {
      return treated;
    }
    const mod = roll.modifier < 0 ? `− ${Math.abs(roll.modifier)}` : `+ ${roll.modifier}`;
    return `1d20 (${roll.faces.join(', ')}) ${mod} (${skill}) = ${roll.total}`;
  }

  protected startTyping(): void {
    this.typing.set(true);
    afterNextRender(() => this.field()?.nativeElement.focus({ preventScroll: false }), {
      injector: this.injector,
    });
  }

  protected backToApp(): void {
    this.typing.set(false);
    this.text.set('');
  }

  protected onType(event: Event): void {
    this.text.set((event.target as HTMLInputElement).value);
  }

  protected confirmTyped(): void {
    const f = this.face();
    if (f !== null) {
      void this.rollWith({ face: f }).then(() => this.text.set(''));
    }
  }

  protected pick(skill: SearchSkill): void {
    this.skill.set(skill);
    this.firstFace.set(null);
    if (skill === 'other') {
      // The focus goes to the list: Enter picks the skill.
      afterNextRender(() => this.list()?.nativeElement.focus(), { injector: this.injector });
    }
  }

  protected pickOther(key: string): void {
    this.otherKey.set(key);
    this.firstFace.set(null);
  }

  /** A tap on a row: the list answers for its rows. */
  protected onListClick(event: MouseEvent): void {
    const row = (event.target as HTMLElement).closest<HTMLElement>('[role="option"]');
    const at = Number(row?.dataset['index']);
    const skill = this.others[at];
    if (skill) {
      this.otherCursor.set(at);
      this.pickOther(skill.key);
    }
  }

  /** The list's keys: the arrows move, Enter (or Space) picks. */
  protected onListKey(event: KeyboardEvent): void {
    const last = this.others.length - 1;
    switch (event.key) {
      case 'ArrowDown':
        this.otherCursor.update((i) => Math.min(last, i + 1));
        break;
      case 'ArrowUp':
        this.otherCursor.update((i) => Math.max(0, i - 1));
        break;
      case 'Home':
        this.otherCursor.set(0);
        break;
      case 'End':
        this.otherCursor.set(Math.max(0, last));
        break;
      case 'Enter':
      case ' ': {
        const at = this.others[this.otherCursor()];
        if (at) {
          this.pickOther(at.key);
        }
        break;
      }
      default:
        return;
    }
    event.preventDefault();
  }

  protected async rollWith(die: SearchDie): Promise<void> {
    if (this.busy() || !this.ready()) {
      return;
    }
    const extra = this.extra.take(!('inApp' in die));
    if (extra === null) {
      this.error.set(this.extra.missingText());
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const first = this.firstFace();
      const sent: SearchDie =
        'face' in die && first !== null ? { face: first, face2: die.face } : die;
      const other = this.skill() === 'other' ? (this.otherKey() ?? '') : '';
      const res = await this.api.search(
        this.data.campaignId,
        this.skill(),
        sent,
        this.key.keyFor({ skill: this.skill(), other, die: sent, extra }),
        other,
        extra,
      );
      if (this.holdFor(res.luckyOffer, res.inspirationOffer)) {
        return;
      }
      await this.showResult(res);
    } catch (err) {
      if ((needsTwoDice(err) || needsTwoD20(err)) && 'face' in die && this.firstFace() === null) {
        this.firstFace.set(die.face);
        // Not an error: the step's own heading and text ask for the second die and say why.
        this.error.set('');
      } else {
        this.error.set(this.extra.fromRefusal(err) || trapErrorMessage(err, 'procurar armadilhas'));
      }
    } finally {
      this.busy.set(false);
    }
  }

  /** The d20 is rolled and kept for a question, the halfling's Lucky about a 1 (first) or the Bardic Inspiration die: the
   * d20 alone tumbles, and the result is told after the answer. `false` when the search asks nothing. */
  private holdFor(
    lucky: LuckyOffer | undefined,
    offer: OutsideInspirationOffer | undefined,
  ): boolean {
    if (!lucky && !offer) {
      return false;
    }
    this.luckyOffer.set(lucky ?? null);
    this.offer.set(lucky ? null : (offer ?? null));
    this.typing.set(false);
    const held = showOfDice('Procurar armadilhas', (lucky ?? offer)?.d20);
    if (held) {
      this.animator.play(held);
    }
    return true;
  }

  /** The search as the result card tells it. */
  private async showResult(res: SearchForTrapsResponse): Promise<void> {
    // Read the map again: what was found now shows on it, and its name is the map's to give.
    await this.data.state.refresh();
    const names = this.data.state
      .points()
      .filter((p) => p.kind === MapPointKind.TRAP && res.foundPointIds.includes(p.id))
      .map((p) => p.name);
    this.result.set({
      res,
      names,
      skill: this.skill(),
      skillLabel: skillName(this.skill(), this.otherPicked()?.name),
    });
    this.typing.set(false);
    this.animate(res);
  }

  /** The answer about the die: the search is made now, with the d20 it rolled and the die when it was used. */
  protected async answerDie(use: boolean, die: InspirationRoll | null): Promise<void> {
    const offer = this.offer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerOutsideInspiration(
        this.data.campaignId,
        offer.holdId,
        use,
        die,
        this.dieKey.keyFor({ hold: offer.holdId, use, die }),
      );
      if (res.result.case !== 'trapSearch') {
        throw new Error('AnswerOutsideInspiration answered without its trap search');
      }
      this.offer.set(null);
      await this.showResult(res.result.value);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder sobre a Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** "Rolar de novo" (rolled in the app, or the typed face) or "Ficar com o 1": the search is made now with the new d20, or,
   * when the character holds a Bardic Inspiration die, the die's question comes next, on the new roll. */
  protected async answerLucky(reroll: boolean, die: LuckyRoll | null): Promise<void> {
    const offer = this.luckyOffer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerOutsideLucky(
        this.data.campaignId,
        offer.holdId,
        reroll,
        die,
        this.luckyKey.keyFor({ hold: offer.holdId, reroll, die }),
      );
      if (res.result.case !== 'trapSearch') {
        throw new Error('AnswerOutsideLucky answered without its trap search');
      }
      this.luckyKey.renew();
      this.luckyOffer.set(null);
      const next = res.result.value.inspirationOffer;
      if (next) {
        this.holdFor(undefined, next);
        return;
      }
      await this.showResult(res.result.value);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The d20 of the search, with "Achou" or "Nada": exactly what the result card tells (the DC never comes). */
  private animate(res: SearchForTrapsResponse): void {
    const found = res.foundPointIds.length > 0;
    const show = showOfDice('Procurar armadilhas', res.roll, {
      withTotal: true,
      outcome: { word: found ? 'Achou' : 'Nada', good: found },
    });
    if (show) {
      this.animator.play(show);
    }
  }

  protected cancel(): void {
    this.sheet.close(this.result() !== null);
  }
}
