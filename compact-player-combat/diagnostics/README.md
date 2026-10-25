# Diagnosticos (setima rodada, a021d41dc)

Capturas de falha dos specs e2e reais em que a escolha do alvo pela lista nao avanca: a folha fica no passo 1 "Alvo" depois do clique no rotulo "Goblin 1", o radio fica sem marca e nao ha evento `change`. Causa: o `(click)` do radio em `attack-sheet.html` (linha 40) termina em `$event.detail > 0 && advance()`; o Angular trata um manipulador que devolve `false` como `preventDefault()`, e o clique que o rotulo reenvia ao radio tem `detail` 0, entao o clique e cancelado.
