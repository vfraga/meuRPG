# O mapa no combate do mestre (1280, 1492 e 1728 px)

Branch `fix/master-desktop-combat`, item 2. Telas de 812 px de altura, com um mapa de 40 × 30 quadrados, três jogadores, três
"Espada voadora" (1 a 3) lado a lado e uma "Armadura animada" (criatura Grande, 2 × 2) ao lado delas.

| Largura | Coluna do mapa antes → depois | Quadrado antes → depois | Ficha (token) antes → depois |
| --- | --- | --- | --- |
| 1280 | 480 → 528 px | 11,1 → 12,3 px | 9,5 → 10,5 px |
| 1492 | 480 → 740 px | 11,1 → 17,6 px | 9,5 → 15,1 px |
| 1728 | 480 → 976 px | 11,1 → 23,5 px | 9,5 → 20,2 px |

Ordem (288 px) e Turno (368 px) não mudam de largura.

## O que limitava o mapa

A página inteira (`main.app-content`) para em 1280 px de largura, com 48 px de margem de cada lado. Em qualquer tela de 1280 px
para cima sobravam 1184 px; tirando Ordem, Turno e os espaços entre as colunas, o mapa ficava com 480 px, e um quadrado de um mapa
de 40 colunas com cerca de 11 px, seja a tela de 1280 ou de 1728 px.

## O que mudou

- Do 1280 px para cima, no combate do mestre (três colunas), a página deixa de parar em 1280 px (limite de 2400 px) e a margem
  lateral é de 24 px em vez de 48 px. A coluna do mapa ocupa a largura que sobra.
- Uma ficha nunca é desenhada maior que o quadrado dela (86% do quadrado, no máximo 40 px, vezes os quadrados que ocupa), e agora
  a borda e os anéis (a vez, "você", a escolha do alvo) encolhem junto, para não passar sobre o quadrado do vizinho.
- Ao escolher um alvo no mapa, o toque vale para a criatura cujo quadrado está sob o dedo ou o ponteiro, não para o botão que
  está por cima: os botões têm 44 px para o toque e, em quadrados pequenos, passavam por cima dos vizinhos. Num quadrado vazio
  vale a criatura mais perto, até 22 px do centro. O teclado continua escolhendo a criatura do botão focado.

## Arquivos

`antes-LARGURA-claro|escuro.png` e `depois-LARGURA-claro|escuro.png`. Gerados com uma página de teste (harness) que monta o
combate do mestre com os dados de teste do código e a mesma moldura de `main.app-content`; as imagens vêm de um build de
desenvolvimento, sem a fonte de ícones (os nomes dos ícones aparecem como texto).
