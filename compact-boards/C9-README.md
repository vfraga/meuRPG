# C9: quem move os tokens fora do combate, o lado do jogador (B2)

O lado do mestre (a aba "Tokens" sob o mapa, dos quadros B2-01 e B2-02) não está aqui. A série mostra só o que o jogador vê, nas telas compactas da cena do jogador:

- no celular, a aba "Mapa" da cena em abas "Ações · Mapa · Imagens" (`C7-07`);
- a partir de 1024 px, o mapa embaixo da cena, com o trilho "Conversa · Ficha" (`C6-08`).

O exemplo é Brisa, com Pensantus e Toren, no mapa "Estrada do Vale", que tem árvores na camada de parede e uma carroça quebrada marcada "Não dá para passar". Só aparece o que o servidor faz: `MoveMapTokens` move o token do próprio personagem quando o mestre deixou (`SetMapPlayersMove`, `players_move_tokens` no mapa) e para antes de uma parede, de uma porta trancada ou de um quadrado "Não dá para passar".

| Quadro | O que decide |
| --- | --- |
| `C9-01`, 1280 e 390 | A chave está desligada, o padrão do mapa. Embaixo do mapa há uma linha só: "O mestre move os tokens agora." Não há controle de movimento: nem "Mover Brisa", nem arrastar. O token de Brisa tem o anel de "você". |
| `C9-02`, 1280 e 390 | A chave está ligada. Embaixo do mapa aparece "Arraste Brisa no mapa ou use “Mover Brisa”. Os outros ficam com o mestre." e o botão "Mover Brisa", só para o próprio personagem. Ele abre o mesmo painel do mestre: a seta escolhe a direção, "Quadrados por clique" (no celular, "Quadrados por toque", como no B2-02) escolhe a distância, o plano diz para onde Brisa vai e "Mover Brisa" envia o movimento. Depois, um aviso: "Brisa andou 2 (3 m)." quando chegou, ou "O movimento parou antes." com o motivo e o quanto andou. No notebook o painel abre dentro do cartão do mapa; no celular, numa folha. |
| `C9-03`, 390 | O mestre liga ou desliga a chave enquanto Brisa olha o mapa. A linha embaixo do mapa vira um aviso de uma linha, numa região viva educada: "O mestre liberou: agora você move Brisa." ou "O mestre voltou a mover os tokens." Quando a chave é desligada, a folha de "Mover Brisa" fecha sem mover. Depois de alguns segundos o aviso volta a ser a linha de sempre, e o foco não muda de lugar. |

## Decisões tomadas aqui

- **O botão "Mover Brisa" do painel muda de forma com o tamanho da tela.** No notebook ele é contornado, porque o botão cheio da tela é o "Enviar" da Conversa. Na folha do celular, que cobre a tela, ele é o botão cheio.
- **O motivo da parada pode ficar de fora.** Num mapa com névoa, se Brisa parou por algo que ela não viu (`MAP_MOVE_STOP_BLOCKED`), o aviso diz só "O movimento parou antes.", sem o motivo.
- **"Estrada do Vale" foi desenhada para estes quadros**, como estrada ao ar livre e sem névoa. Os quadros B2 usam a caverna.
- **Um movimento enviado quando o mestre trava a chave segue a resposta do servidor.** Se o servidor recusar, o token fica onde estava e a folha diz "O mestre voltou a mover os tokens."

## Em aberto

- O plano mostrado antes de mover deveria avisar que o caminho cruza um quadrado "Não dá para passar" que Brisa vê? Hoje só o resultado avisa.
- Quanto tempo o aviso do `C9-03` fica antes de voltar a ser a linha de sempre?
