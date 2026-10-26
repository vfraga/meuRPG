# Contagem dos espaços de magia na linha da vez (`fix/slot-count-fits`)

A linha de números do cartão da vez do jogador ("PV 23 de 23 · CA 12 · 1º … 2º …") volta a dizer quantos espaços estão livres, e continua sendo **uma linha** em 390 e em 320.

- **Antes** (`antes-*`): sem a contagem; em 320 (e 360) a linha quebra em duas.
- **Depois** (`depois-*`): a partir de 440 px de largura do cartão cada círculo tem os anéis e "3 de 4" (`depois-768-claro`); abaixo disso a contagem curta "3/4" ocupa o lugar dos anéis; abaixo de 320 px de largura do cartão (viewport de ~350 ou menos) os ícones somem. O nome acessível de cada círculo ("3 livres de 4") não mudou.

## Medidas (Playwright, mago de nível 3: PV 23 de 23, CA 12, 1º 4 espaços (1 usado), 2º 2 espaços)

Altura da linha / linhas visuais, claro e escuro (28 de 28 medições do "depois" têm uma linha, sem rolagem horizontal):

| Largura | Antes | Depois |
| --- | --- | --- |
| 320 | 42 px, 2 linhas | 19 px, 1 linha |
| 340 | 42 px, 2 linhas | 19 px, 1 linha |
| 360 | 42 px, 2 linhas | 19 px, 1 linha |
| 375 | 19 px, 1 linha | 19 px, 1 linha |
| 390 | 19 px, 1 linha | 19 px, 1 linha |
| 430 | 19 px, 1 linha | 19 px, 1 linha |
| 768 | 19 px, 1 linha | 19 px, 1 linha |

## Método

Harness descartável que monta o `OwnNumbers` real num cartão com a mesma moldura do cartão da vez (gutter de 16 px da página, borda de 2 px, padding de 12 px), construído com `ng build` do `origin/main` ("antes") e da árvore da correção ("depois"), medido e fotografado com Playwright. Não montei o cartão da vez inteiro: a medida é da linha de números, não da posição da primeira linha de ações.

Limitação: a fonte de ícones não carrega offline; nas imagens e nas medidas os ícones são trocados por um disco de 16 px (o tamanho real) para a largura ser fiel.
