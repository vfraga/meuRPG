import { Component, computed, input } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { DiceOption } from '../../../core/campaigns/dice-labels';
import { formatModifier } from '../../../core/characters/character-labels';
import { DiceChoice } from '../../../shared/dice-choice/dice-choice';
import { RollPicker } from '../../../shared/roll-picker/roll-picker';
import { LevelUpSession } from '../level-up-session';

/** The two cards' values: `DiceChoice` keys its radios by number. */
const AVERAGE = 0;
const ROLL = 1;

/**
 * Step "Vida" of the guided level-up (MR-040, E8-15): "Média: 4" (preselected) or "Rolar 1d6",
 * the die the campaign's dice setting allows (RN-18): rolled on the server and kept
 * (`RollLevelUpHitPoints`), or typed from a physical die, with the same picker as the
 * combat and the scene. The Constitution case (an ability increase that raises its modifier)
 * shows what it does to the hit points; every number comes from `PreviewLevelUp`.
 * Under it, "O que o nível N dá": what is chosen, what is still to choose, what is automatic.
 */
@Component({
  selector: 'app-hp-step',
  imports: [DiceChoice, MatButtonModule, MatIconModule, RollPicker],
  templateUrl: './hp-step.html',
  styleUrl: './hp-step.scss',
})
export class HpStep {
  readonly s = input.required<LevelUpSession>();

  protected readonly card = computed(() => (this.s().draft.hpCard() === 'roll' ? ROLL : AVERAGE));
  protected readonly rolled = computed(() =>
    this.s().draft.hpCard() === 'roll' ? this.s().draft.rolled() : null,
  );
  /** The die card cannot be chosen when this level's die was rolled for another class, and says why inside the card. */
  protected readonly rollBlocked = computed<readonly number[]>(() =>
    this.s().rollOtherClass() ? [ROLL] : [],
  );
  protected readonly rollReasons = computed<Partial<Record<number, string>>>(() =>
    this.s().rollOtherClass() ? { [ROLL]: this.s().rollOtherClass() } : {},
  );
  protected readonly die = computed(() => this.s().die);

  protected readonly options = computed<readonly DiceOption<number>[]>(() => {
    const s = this.s();
    const avg = s.average();
    const rolled = this.rolled();
    const after = s.after().hitPointsMax;
    let rollNote = s.canApp
      ? `${s.withCon(`1d${s.die}`)} · o resultado fica no registro`
      : `Digite o número do seu dado · o resultado fica no registro`;
    if (rolled) {
      rollNote = `Saiu ${s.withCon(rolled.value)} · de ${avg.from} para ${after}`;
    }
    return [
      {
        value: AVERAGE,
        title: `Média: ${s.options.hitPointAverage}`,
        description: `${s.withCon(s.options.hitPointAverage)} · de ${avg.from} para ${avg.to}`,
      },
      { value: ROLL, title: `Rolar 1d${s.die}`, description: rollNote },
    ];
  });

  protected readonly title = computed(() => {
    const o = this.s().options;
    return o.isNewClass ? `Pontos de vida de ${o.classNamePt} 1` : 'Pontos de vida';
  });

  protected readonly intro = computed(() => {
    const s = this.s();
    if (s.options.isNewClass) {
      return `O nível 1 de ${s.options.classNamePt} dá um d${s.die}, e o personagem vai ao nível total ${s.options.totalToLevel}. O dado cheio só vale no nível 1 do personagem.`;
    }
    const raised = s.draft.abilityIncrease().con !== undefined;
    const mod = formatModifier(s.conModifier());
    return `O ${s.options.classNamePt} ganha 1d${s.die} por nível, mais o modificador de Constituição (${mod}${raised ? `, com o aumento do passo ${s.stepNumber('abilities')}` : ''}).`;
  });

  protected readonly lockLine = computed(
    () => `Só o que ${this.s().levelWords} dá fica aberto. O resto da ficha continua travado.`,
  );

  protected pick(value: number): void {
    if (value === ROLL) {
      this.s().chooseRoll();
    } else {
      this.s().chooseAverage();
    }
  }

  /** "Trocar o resultado": a typed die can be typed again (a roll in the app is kept, so it cannot). */
  protected readonly canRetype = computed(() => this.rolled()?.kind === 'physical');
  protected retype(): void {
    this.s().draft.rolled.set(null);
  }

  protected readonly rollLabel = computed(
    () => `Role 1d${this.s().die} para os pontos de vida do nível`,
  );
  protected readonly rollHint = computed(
    () => `Role o seu d${this.s().die} e digite o número que saiu (1 a ${this.s().die}).`,
  );
}
