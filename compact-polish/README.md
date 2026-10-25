# compact-polish: a lista de polimento das revisões de 24/10 (feat/compact-polish)

**Capturas do harness, não da pilha real** (a VM não tem Docker nem CockroachDB): os componentes reais montados com um cliente de combate e um mapa falsos. Nome: `<tela>-<antes|depois>-<estado>-<largura>-<tema>.png`; "antes" é o `main` (f28b9c12), "depois" é o ramo.

| Tela | O que mudou (compare antes e depois) |
| --- | --- |
| `combate-jogador-*-01-turno`, `*-02-magias` | "Conjurar", "Atacar", "Encerrar turno" e "Mover" com o texto em tinta (não em grená); os espaços de magia dizem "3 de 4" ao lado das bolinhas; o ícone de "Ordem" é a lista de conferência (1280); no celular o mapa fica logo sob o cabeçalho (12 px, não 24) |
| `combate-jogador-*-03-alvo-no-mapa` | A dica "Adaga · toque num inimigo com anel." em tinta, 14 px |
| `combate-jogador-*-04-320x568-turno` | A 320 px a barra "Ainda tem" perde a palavra (os símbolos bastam; o grupo mantém o nome para o leitor de tela): o primeiro cartão sobe uma linha |
| `combate-mestre-*-01-resultado-do-ataque` | O bloco do resultado no trilho ganha a cabeça "Cimitarra em Brisa" (o quadro da `Main`) |
| `mapa-da-sessao-*-01-mestre-mapa`, `*-02-jogador-aba-mapa` | A legenda do mapa do mestre sem a linha "Arraste um token…"; a legenda da aba "Mapa" do jogador no celular diz "Mapa · Cripta do Corvo" |

## O que não tem captura, e por quê

- **"Grupo" no celular do mestre, em combate:** passou a ser o painel do grupo dentro da sessão (o combate espera atrás); é uma tela da sessão inteira, que o harness não monta. Está coberta por um teste da página (`live-session.spec`, "gives the master's phone Grupo as a panel...").
- **O aviso "Você abriu a porta.":** um toast de 8 s depois do movimento; o harness não move. Coberto por `door-notice.spec` e pelo e2e de portas.
- **"?" das magias:** o harness não traz a ficha das magias; mudou o nome (`title`) e os 44 px. 
- **A legenda do mapa com névoa** (o master): o mapa de névoa deixa a linha de fichas à página; coberto por `fog-map.spec` (`tokenMarks`).

## O que falta uma decisão de aparência (não construí)

- A lista de encontros guardados na janela "Iniciar combate" (uma seção nova na janela).
- A "margem preta" da prévia do mapa na cena do mestre: no harness ela vem dos quadrados "não visto" do `GetMapVision` da fixture; não reproduz com a visão inteira do mestre.
