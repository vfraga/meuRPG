import { Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { RulesDraft, TableRulesVm } from '../../core/campaigns/table-rules';

/** The label every SRD 5.2.1 method carries, next to the number it gives (CC BY 4.0, `NOTICE`). */
export const SRD_521_LABEL = 'SRD 5.2.1 (regras de 2024)';

const LIST = new Intl.ListFormat('pt-BR', { type: 'conjunction' });

type MethodKey = 'standardArray' | 'pointBuy' | 'rolled4d6' | 'typed';

/**
 * The master's "Habilidades de uma ficha nova" (RN-24): four checkbox cards, one per way of making ability scores, at least
 * one on. The numbers in the words (the array, the budget, the score range, the typed range) are the server's
 * (`GetTableRules`); the three SRD 5.2.1 ways carry "SRD 5.2.1 (regras de 2024)" under them, and "Digitar" (ours) does not.
 */
@Component({
  selector: 'app-method-cards',
  imports: [MatIconModule],
  templateUrl: './method-cards.html',
  styleUrl: './method-cards.scss',
})
export class MethodCards {
  readonly rules = input.required<RulesDraft>();
  readonly numbers = input.required<TableRulesVm>();
  readonly toggled = output<Partial<RulesDraft>>();

  protected readonly cards = computed(() => {
    const n = this.numbers();
    const top = n.pointBuyMinScore + n.pointBuyCosts.length - 1;
    return [
      {
        key: 'standardArray' as MethodKey,
        title: 'Conjunto padrão',
        text: `${LIST.format(n.standardArray.map(String))}, um para cada habilidade.`,
        srd: true,
      },
      {
        key: 'pointBuy' as MethodKey,
        title: 'Compra por pontos',
        text: `${n.pointBuyBudget} pontos; cada valor vai de ${n.pointBuyMinScore} a ${top}.`,
        srd: true,
      },
      {
        key: 'rolled4d6' as MethodKey,
        title: '4d6, descartando o menor',
        text: 'Rolados uma vez só: rolar de novo mostra os mesmos.',
        srd: true,
      },
      {
        key: 'typed' as MethodKey,
        title: 'Digitar os valores',
        text: `O jogador escreve os seis valores, de ${n.typedMin} a ${n.typedMax}, antes do bônus da raça.`,
        srd: false,
      },
    ];
  });

  protected readonly label = SRD_521_LABEL;

  protected toggle(key: MethodKey): void {
    this.toggled.emit({ [key]: !this.rules()[key] });
  }
}
