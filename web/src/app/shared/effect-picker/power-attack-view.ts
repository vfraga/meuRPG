import { computed } from '@angular/core';

import { POWER_COPY } from '../../core/content/effect-field-copy';
import type { EffectDraft } from '../../core/content/effect-draft';
import { FORBIDDEN_EFFECT_TEXT } from '../../core/content/content-violations';
import type { EffectMenuField } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { SelectOption } from '../form-fields/select-field';

/** What the picker reads of the effect it draws, for the power attack. */
interface PowerHost {
  readonly effect: () => EffectDraft;
  readonly siblings: () => readonly EffectDraft[];
  readonly inFeat: () => boolean;
}

/**
 * What the picker says and does differently for a power attack: the block it draws, the type that cannot be taken twice in one
 * entry, the refusal of the second one in words, the resource and the names of what "Mais opções" holds while it is folded.
 */
export class PowerView {
  constructor(private readonly host: PowerHost) {}

  /** The effect is a power attack the block can show (one outside the block's ranges is kept as stored and shown read-only). */
  readonly active = computed(
    () => this.host.effect().type === 'power_attack' && this.host.effect().kept === undefined,
  );

  /** Another effect of the entry is a power attack already: the server allows one for each feature. */
  private readonly second = computed(() =>
    this.host.siblings().some((e) => e.type === 'power_attack' && e.id !== this.host.effect().id),
  );

  private reason(): string {
    return POWER_COPY.second(this.host.inFeat());
  }

  /** The option of a type in the "Efeito" menu: the power attack is dashed, with the reason, where the entry has one already. */
  option(t: { type: string; namePt: string }): SelectOption {
    const own = { value: t.type, label: t.namePt };
    return this.second() && t.type === 'power_attack'
      ? { ...own, disabled: true, reason: this.reason() }
      : own;
  }

  /** The refusals of the type: the server's "not in the menu" for a second power attack says so in words. */
  typeIssues(texts: readonly string[]): readonly string[] {
    return this.host.effect().type === 'power_attack'
      ? texts.map((t) => (t === FORBIDDEN_EFFECT_TEXT ? this.reason() : t))
      : texts;
  }

  /** The label of "no resource" in the list of the entry's resources, or `undefined` for the picker's own. */
  resourceNone(): string | undefined {
    return this.host.effect().type === 'power_attack' ? POWER_COPY.resourceNone : undefined;
  }

  /** The line beside the resource, or '' for another type. */
  resourceHint(): string {
    return this.host.effect().type === 'power_attack'
      ? POWER_COPY.resourceHint(this.host.inFeat())
      : '';
  }

  /** What "Mais opções" holds, after its name, while it is folded: the fields of `all` that `shown` leaves out, in the words of the block. */
  folded(
    all: readonly EffectMenuField[],
    shown: readonly EffectMenuField[],
    expanded: boolean,
  ): string {
    return !this.active() || expanded
      ? ''
      : all
          .filter((f) => !shown.includes(f))
          .map((f) => POWER_COPY.folded[f.name] ?? f.name)
          .join(', ');
  }
}
