# Bandeja de dados (`feat/dice-tray`)

Antes e depois da animação dos dados da sobreposição de rolagem (`shared/roll-overlay`), a 390, 768 e 1280 px, nos temas claro e escuro.
`before/` é o `main` (os dados em CSS, depois do pouso, a 1,8 s); `after/` é a bandeja de feltro com os dados em repouso: semente fixa (`crypto.getRandomValues` fixo em 20260101) e um toque, que chama `finish()`.
Nome do arquivo: `<largura>x<altura>-<tema>-<cena>.png`. Cada cena existe nos dois lados, nos três tamanhos e nos dois temas (18 pares).

| Cena | O que mostra |
| --- | --- |
| `ataque-vantagem` | Ataque com Machado grande com vantagem: dois d20, o 6 descartado (meia força e "descartado"), o 17 que vale; "17 + 5 = 22" e "Acertou" sob a bandeja |
| `dano-3d6` | Dano de 3d6 (4, 2 e 5): três dados de papel, a linha "4 + 2 + 5 + 3 = 14" e "cortante" |
| `critico` | Ataque com Espada longa, 20 natural: o d20 grená com brilho dourado, a etiqueta "Crítico!" e a palavra "Crítico!" no cartão |

O que muda entre os lados:

- A bandeja ocupa a largura do conteúdo da sobreposição (margens de 16 px) com altura de 0,55 da largura no celular, e no máximo 520 × 280 px do tablet para cima. O texto (rótulo, linha, desfecho) continua sob a bandeja.
- O d20 é grená e os outros dados são de papel, com números de tinta; as bordas dos dados e o feltro vêm dos tokens `--mr-tray-*` e `--mr-die-*`, nos dois temas.
- Quando os dados param, os números dos lados esmaecem e só o resultado fica forte.
- O botão "Som dos dados" (desligado, `aria-pressed="false"`, alvo de 44 px) fica sob o texto.
- A tela de fundo é a página inicial sem servidor, só para dar contexto; a sobreposição é a mesma que o app monta.
