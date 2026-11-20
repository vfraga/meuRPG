import { computed } from '@angular/core';

import type { CombatantState } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { castRows, castSubtitle, effectSentence } from '../../../../core/combat/cast-flow';
import { shapeText } from '../../../../core/combat/area-text';
import { poolRollText } from '../../../../core/combat/hp-effects';
import { releaseSubtitle, releaseTitle } from '../../../../core/combat/release-text';
import type { CastArea } from './cast-area';
import type { CastChoices } from './cast-choices';
import type { CastDamage } from './cast-damage';
import type { SlotAfter } from './cast-result';
import {
  castNotes,
  castTitle,
  concentrationEnd,
  doneSubtitle,
  isPlainCast,
  nextRollOf,
  resumedCastOf,
  shieldLostText,
  slotAfter,
} from './cast-sheet-view';
import type { CastState } from './cast-state';

/** What the sheet says about the cast: its title and subtitle, the result as told, the damage still to roll. */
export class CastTexts {
  /** "Constrição encerra a concentração em Conjurar Animais.", and what goes with it (`concentrationEnd`). */
  readonly endsConcentration = computed(() =>
    concentrationEnd(this.s.data.state.encounter(), {
      casterId: this.s.data.casterId,
      concentration: this.s.data.concentration,
      key: this.s.data.spellKey,
      name: this.s.data.name,
    }),
  );
  /** "Sono conjurado", "Palavra de Poder Atordoar conjurada": the sheet of a spell that
   * reads hit points says what was done, then who it touched. */
  private readonly hpDone = computed(
    () => this.s.done() && (this.choices.kind() === 'pool' || this.choices.kind() === 'hp'),
  );
  readonly title = computed(() =>
    this.releasing()
      ? releaseTitle(
          this.s.data.name,
          !!this.s.data.master,
          this.s.data.release?.reactorLabel ?? '',
        )
      : castTitle({
          name: this.s.data.name,
          resume: !!this.s.data.resume,
          done: this.s.done(),
          hpDone: this.hpDone(),
          placedArea: !!this.area.flow,
          typing: this.s.typing(),
        }),
  );
  /** The release's own words: before the cast, while the held spell is placed. */
  private readonly releasing = computed(() => !!this.s.data.release && !this.s.done());
  readonly subtitle = computed(() => {
    const { data } = this.s;
    if (data.release && !this.s.done()) {
      return releaseSubtitle(data.release.reactorLabel, !!data.master);
    }
    if (this.s.done()) {
      return doneSubtitle({
        cast: this.s.cast(),
        level: data.level,
        kind: this.choices.kind(),
        placedArea: !!this.area.flow,
        details: this.s.details(),
      });
    }
    if (this.s.typing()) {
      return `${data.name} · Rodada ${data.round}`;
    }
    return castSubtitle(
      data.economy,
      this.choices.kind(),
      this.s.details(),
      this.s.slotLevel(),
      this.choices.dartsTotal(),
      data.cantripDice,
      this.choices.damageType(),
      this.area.flow ? shapeText(this.area.shape) : '',
    );
  });
  readonly labels = computed(() => {
    const out = new Map<string, { label: string; state: CombatantState }>();
    for (const c of this.s.data.state.encounter()?.combatants ?? []) {
      out.set(c.id, { label: c.label, state: c.state });
    }
    return out;
  });
  readonly resultRows = computed(() => {
    const pendings = this.damage.pendings();
    return castRows(this.s.cast() ?? resumedCastOf(pendings.values()), pendings, this.labels());
  });
  /** The live sentence of a spell that reads hit points ("O Goblin 1 adormeceu. ..."), and the caster's
   * own roll of the pool: the server sends it only to the master and to the caster's player. */
  readonly sentence = computed(() => {
    const cast = this.s.cast();
    return cast ? effectSentence(cast, this.labels(), (id) => this.choices.npcs().has(id)) : '';
  });
  readonly poolText = computed(() => {
    const roll = this.s.cast()?.poolRoll;
    return roll ? poolRollText(roll) : '';
  });
  readonly plain = computed(() => isPlainCast(this.s.cast(), this.choices.kind()));
  readonly after = computed<SlotAfter | null>(() => slotAfter(this.s.cast(), this.choices.rows()));
  readonly shieldLost = computed(() =>
    shieldLostText(this.s.cast(), this.s.data.shieldFree, this.s.data.shieldName),
  );
  readonly notes = computed(() =>
    castNotes(this.s.cast(), {
      name: this.s.data.name,
      economy: this.s.data.economy,
      metaOptions: this.choices.metaOptions,
      sorceryPoints: this.choices.sorceryPoints,
    }),
  );
  /** The roll of the first damage group: its dice, the sum a typed roll may be. */
  readonly nextRoll = computed(() => nextRollOf(this.damage.groups(), this.labels()));

  constructor(
    private readonly s: CastState,
    private readonly choices: CastChoices,
    private readonly area: CastArea,
    private readonly damage: CastDamage,
  ) {}
}
