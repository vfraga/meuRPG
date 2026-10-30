# W7-M (a): ações da criatura que não são ataque

Prints do cartão do NPC do mestre (Cão infernal e Carniçal do SRD, contra Vex, combate no teatro da mente), tirados de uma pilha real
(API, banco e web compilados da branch `feat/w7m-actions`). Larguras 390, 768 e 1280; temas claro e escuro. Nome do arquivo:
`NN-tela-wLARGURA-TEMA.png`. Só dados inventados ou do SRD.

- `01a-cartao-antes-sem-as-acoes`: o cartão do Cão infernal como na `main`, sem a área "Outras ações". Não há pilha da `main` aqui: o
  "antes" é o mesmo cartão com a área escondida por CSS (`app-creature-actions { display: none }`), o que é o comportamento da `main`.
- `01b-cartao-depois-com-as-acoes`: o mesmo cartão com "Outras ações": quem a ação atinge (o mestre escolhe, no teatro da mente), o
  Fire Breath com a salvaguarda, o dano e a área, e o botão "Usar".
- `02-resultado-da-salvaguarda`: depois de "Usar": a ação gasta e o resultado dentro do cartão (a salvaguarda de cada alvo e a rolagem
  contra a CD), com o dano a rolar logo abaixo.
- `03-oferta-do-efeito`: a oferta do efeito das garras do Carniçal depois do golpe (salvaguarda de Constituição, Paralisado), com
  "Aplicar" e "Dispensar".
- `04-registro-do-mestre`: o registro do combate para o mestre: as linhas das ações, com o texto do SRD marcado "Só o mestre vê".
- `05-registro-do-jogador`: o registro do jogador (a folha de Registro do combate): só o resultado ("o Vex resistiu"), sem o texto da
  ficha do monstro e sem a CD.
