# Antes do MVP: PM-01, PM-02 e PM-03 (PM-04 fica de fora, de propósito)

Os quadros são HTML estático (estilos em linha), no formato das rodadas anteriores. Cada arquivo tem um PNG ao lado.
`project/canvas.json` põe os oito quadros numa fila, 80 px entre eles. Os `build_*.py`, `lib.py`, `mapkit.py` e
`crop.js` são só as ferramentas que desenharam os quadros (rodam com `python3 build_pm01a.py`, por exemplo).

| Quadro | Largura × altura | O que tem |
| --- | --- | --- |
| `PM-01a` sessões anteriores: o painel | 1280 × 3526 | painel da campanha com três, uma e nenhuma sessão; carregando, erro, lista longa; jogador no celular (390, 320); foco visível |
| `PM-01b` sessões anteriores: o resumo | 1280 × 5369 | a página `/campaigns/<id>/sessions/<n>`: com combates, sem combate, jogador, carregando e não encontrada |
| `PM-01c` sessões anteriores: outros estados | 1280 × 4361 | ainda aberta, erro, mestre × jogador, jogador sem personagem na sessão, mestre no celular, foco no pager |
| `PM-02a` magia de área: regras, colocar, quem está dentro | 1280 × 4434 | as 8 regras com o SRD ao lado, as 5 formas, colocar a Bola de Fogo na caverna (390, 320), quem está dentro |
| `PM-02b` magia de área: ninguém, parede, alcance, formas, teclado | 1280 × 4074 | confirmação "ninguém na área", ponto atrás da parede, fora do alcance, cone, linha e cubo, teclado e "Centrar em…" |
| `PM-02c` magia de área: escondidas e o turno parado | 1280 × 5910 | regra "Criaturas escondidas atingidas por uma área", pedido ao mestre que trava o turno, duas perguntas, depois de recarregar, resultado do jogador, o turno parado |
| `PM-02d` magia de área: seletor do mestre, teatro da mente, servidor | 1280 × 3504 | seletor do mestre com o interruptor, registros, teatro da mente, contrato do servidor |
| `PM-03a` Escudo Arcano e Ajuda | 1280 × 4594 | CA e PV do jogador, Ajuda +10, fim do Escudo, ficha, ordem do mestre, encerrar a Ajuda, Ajuda a 0 PV |
| `PM-03b` Crítico Brutal e Talento Confiável | 1280 × 4378 | dano do crítico (app e dados físicos), a conta nos níveis 9/13/17, registro, resultado do teste |
| `PM-03c` salto, ataque de oportunidade e armadilhas | 1280 × 7540 | aviso do salto (sem e com armadilha revelada), painel e registros do mestre, "Também acham com", busca do jogador, a conta do servidor |

Os números dos estados continuam entre os arquivos de PM-02: 1 a 3 em PM-02a, 4 a 7 em PM-02b, 8 a 10b (com 9b e 9c) em PM-02c, 11 a 14 em PM-02d. PM-01b e PM-01c seguem a mesma numeração (1 a 4 e 5 a 9).

## Decisões

**PM-01**

1. Painel vazio: o **mestre** vê o painel com "Nenhuma sessão encerrada ainda..." (caixa tracejada, sem botão); o **jogador não vê o painel**. É o que o painel "Sessão" já faz.
2. A linha traz também a **duração** ("3 h 42 min"), calculada dos dois horários; o servidor não manda mais nada. A data usa o formato do pedido ("qui., 8 de out., 19h05 às 22h47"), que é um formatador novo; hoje o app escreve "30/09 às 20:05". Sessão que cruza a meia-noite traz o dia da hora final; de outro ano, o ano.
3. Lista longa: as 5 mais novas e "Mostrar as outras N" (só no navegador, a lista já vem inteira).
4. O painel usa a **mesma resposta** de `ListGameSessions` que a página da campanha já lê (sem segunda chamada).
5. Cabeçalho da página: h1 "Sessão 2", embaixo "Mirathel · data e hora"; o título da aba é "Sessão 2 · Mirathel". Usei o nome da campanha do resto dos quadros (Mirathel), não "Cripta do Corvo".
6. O cartão verde "Sessão encerrada" (aviso do instante do fim) **sai** da página de arquivo; os quatro números viram o painel "Em números". O resto do resumo é o de hoje, e a página ganha o que a resposta já traz e a tela não mostrava: **"Números de cada jogador"** (só mestre). A tela do fim da sessão fica como é; as duas usam um componente.
7. Sessões vizinhas: dois links no alto ("Sessão 1", "Sessão 3"); sem vizinha, o link some. Se a seguinte está aberta, o link diz "Sessão 4 em andamento".
8. "Não houve combate nesta sessão." é um texto novo, só do navegador.
9. Não encontrada: o servidor devolve `not_found` para "número que não existe" e para "não é membro". O app separa pelo que sabe: membro (lista respondeu) vê "Não há Sessão 9" com a última sessão; quem não é membro vê uma página genérica sem o nome da campanha.

**PM-02**

10. **Quem conjura é Pensantus, Mago 5** (a Bola de Fogo pede espaço de 3º nível; nos quadros da Etapa 10 ele é Mago 4). Mapa: a caverna. Toren é o aliado dentro da área.
11. Fluxo em dois passos: 1 "Onde ela explode" (ponto ou direção; "Confirmar local") e 2 "Quem está na área" (lista com cobertura, "Mudar o local", "Conjurar"). Com o dedo, o molde segue **44 px acima** do dedo. "Ajustar o ponto" move um quadrado por toque.
12. **Cone, linha e cubo em 8 direções** (a cada 45°), não em ângulo livre; o cubo encosta por uma face (lados retos) ou uma quina (diagonais).
13. Um quadrado entra na área quando o **centro** dele está dentro (esfera: a até 4 quadrados do ponto, medida exata, não arredondada para baixo como a distância de hoje, para a bola não crescer 1,5 m).
14. **A Bola de Fogo "se espalha pelos cantos"** (texto da magia): a área é a parte da esfera ligada ao ponto, não só o que a linha reta alcança; as outras formas seguem a regra geral (só cobertura total bloqueia). Quem é atingido só pelo canto **não ganha bônus automático** no teste (o SRD não dá nenhum); o mestre marca a cobertura à mão (`SetCombatantCover`), como hoje.
15. Servidor (tudo "novo no servidor", tabela em PM-02d estado 14): `PreviewSpellArea` (só leitura), `CastSpellRequest.area` (ponto ou direção; para área em mapa em grade, `targets` é ignorado), `CastSpellRequest.reveal_hidden` (só mestre), `CastSpellResponse.area/hidden_hits/pending_reveal_id`, `ResolveHiddenReveal`, o motivo de espera `HIDDEN_REVEAL_PENDING`, `TableRules.hidden_area_hits` (`REVEAL` padrão, `KEEP_HIDDEN`, `ASK`) e o evento `hidden_hit_pending` (só mestre).
16. "Ninguém na área": a confirmação fica no lugar, com "Conjurar mesmo assim" e "Mudar o local"; o texto diz "que você vê" porque pode haver escondida lá dentro.
17. Escondidas: o efeito sempre vale. Antes do Revelar, o jogador não recebe a lista, a contagem, o resultado nem o registro (nada que a nomeie). Com "Perguntar a cada vez", o pedido ao mestre **trava o turno** (decisão do Vinicius, 08/10): o servidor recusa mover, atacar, conjurar, agir, `EndTurn` e o `NextTurn` do mestre com o motivo novo `HIDDEN_REVEAL_PENDING`, até o mestre tocar "Revelar" ou "Manter escondidas". O jogador lê só "Esperando o mestre" (a mesma frase do ataque de oportunidade), então a espera não revela nada. O mestre continua podendo agir; "Encerrar combate" descarta a pergunta. Para o mestre que conjura, o interruptor "Revelar as escondidas atingidas" só aparece se há escondida na área e começa na regra da mesa; com "Perguntar a cada vez" começa ligado.
18. A regra nova entra em `TableRules` e é gravada pelo "Salvar regras" que a página já tem; o servidor lê a regra na hora de cada magia (muda no meio da sessão).
19. Teatro da mente: a lista de hoje continua; a confirmação "Ninguém está marcado" é a mesma do passo 2; a cobertura continua sendo a marca do mestre.
20. Teclado: setas movem o ponto (Shift: 5 quadrados), Enter coloca/confirma, Escape cancela, C abre "Centrar em…", uma lista com as criaturas que quem conjura vê. Para cone, linha e cubo o botão é "Apontar para…".
21. Tokens: os do app de hoje (personagem: disco escuro com inicial clara; NPC: quadrado branco). A área da magia é um tom grená com contorno; "Fora do alcance" escurece o quadrado; na caverna os 45 m cobrem o mapa todo, então o alcance aparece num mapa de exemplo de 60 × 40.

**PM-03**

22. Exemplos com os dados do app: Pensantus tem CA 13, então "CA 18" e "CA 13 + 5" (o pedido dizia "CA 20" e "CA 15 + 5"). A Ajuda é em Sálvia (38 + 5 = 43, como no pedido), conjurada em espaço de 2º nível por um NPC. Crítico Brutal: **Ragna, Bárbara 9** (d12, Força 16); Talento Confiável: Brisa, Ladina 11, Acrobacia +9.
23. Ajuda: `hit_points_max_bonus` novo em `CharacterVitals` e `Combatant`; `hit_points_max` passa a ser o máximo efetivo. Não conta horas: acaba quando o mestre encerra (`EndCombatEffect`, novo) ou num descanso longo. Ao acabar, os PV atuais **só perdem o que passa do novo máximo** e nunca zeram por si. A Ajuda **deixa de dar PV temporários**.
24. Crítico Brutal: `PendingDamage.extra_dice_count` e `extra_dice_name_pt`; com dado físico o jogador continua digitando a **soma** (`typed_sum`), mas a tela diz quantos dados rolar (3d12) e a faixa (3 a 36). Com a regra "máximo mais uma rolagem", o máximo entra sozinho e o Crítico Brutal continua um dado rolado.
25. Talento Confiável: `DiceRoll.treated_as` e `treated_as_source`; a tela mostra "d20: 6 → 10 (Talento Confiável) + 9 = 19" só quando a regra mudou o número; vale onde o servidor rola teste de perícia (cena, busca de armadilhas), nunca em ataque, resistência, iniciativa nem teste de habilidade sem perícia.
26. Salto: o servidor **já provoca** no salto longo (`MoveCombatant`); só faltava o aviso. Novo: `GetMoveOptionsRequest.jump` e `jump_running_start` para a prévia trazer `provokes_reactor_ids`; `OpportunityOffer.jump` só para o painel dizer "saltou". Sem aviso em salto de altura.
27. Armadilhas: `TrapSpec.also_find_skill_keys` (nenhuma por padrão; usam a CD para achar), `TrapSearchSkill.OTHER` + `SearchForTrapsRequest.other_skill_key`. O menu do jogador é igual para toda armadilha e a busca com uma perícia que nenhuma armadilha por perto aceita é **igual** a uma busca que falhou (mesma resposta, mesma ação gasta, mesmo texto; a pílula é "Achou" ou "Nada", não "Falhou"). A penalidade de luz continua só da Percepção.

## Regras verificadas (SRD 5.1, no commit `a8abc93b…` do 5e-database; capítulos de regras e magias)

| Regra | Onde | O que diz (resumo) |
| --- | --- | --- |
| Ponto de origem da esfera e do cilindro | "Areas of Effect" (Sphere, Cylinder); Bola de Fogo | esfera: o ponto é escolhido, o raio sai dele e ele entra na área; Bola de Fogo: "a point you choose within range" |
| Cone e linha saem de quem conjura | "Areas of Effect" (Cone, Line); "Spell Range"; Mãos Flamejantes, Relâmpago | alcance Pessoal para cones e linhas "que saem de você"; o ponto de origem só entra na área se o conjurador decidir |
| Cubo | "Areas of Effect" (Cube); Onda Trovejante | "originating from you"; o ponto de origem fica numa face |
| Linha de efeito | "Areas of Effect"; Bola de Fogo | só cobertura total bloqueia a linha; a Bola de Fogo "spreads around corners" |
| Cobertura contra o teste | "Cover" | meia = +2, três quartos = +5 em CA e testes de Destreza; só vale se o efeito "originates on the opposite side"; criatura (amiga ou inimiga) dá meia cobertura |
| Aliados e o conjurador entram | Bola de Fogo ("Each creature…"); "Damage Rolls" | dano rolado uma vez para todos |
| Ponto que o conjurador não vê | "Targets" | o ponto de origem "comes into being on the near side of that obstruction" |
| Escondida atingida por área | "Unseen Attackers and Targets" | só trata de ataques; o SRD não diz que um golpe de área revela: decisão do Vinicius (08/10) |
| Escudo Arcano | Magia Escudo | reação; +5 de CA até o começo do próximo turno, inclusive contra o ataque que a provocou |
| Ajuda | Magia Ajuda | até 3 criaturas; máximo e PV atuais +5 por 8 horas; +5 por nível acima do 2º |
| PV não passam do máximo | "Healing" | usado para o que acontece ao fim da Ajuda |
| Acordar a 0 PV | "Dropping to 0 Hit Points" | "This unconsciousness ends if you regain any hit points"; testes contra a morte zeram |
| PV temporários não acordam | "Temporary Hit Points" | por isso a Ajuda não pode ser PV temporários |
| Efeitos iguais não somam | "Combining Magical Effects" | vale o mais potente (Ajuda repetida) |
| Descanso longo | "Long Rest" | pelo menos 8 horas (a Ajuda dura 8 horas) |
| Crítico Brutal | Característica do Bárbaro, nível 9 | +1 dado de arma no crítico corpo a corpo; 2 no 13; 3 no 17 |
| Dados do crítico | "Damage Rolls" (Critical Hits) | rolar todos os dados de dano duas vezes |
| Talento Confiável | Característica do Ladino, nível 11 | em teste de habilidade que soma proficiência, d20 de 9 ou menos conta 10 |
| Ataque de oportunidade | "Melee Attacks" (Opportunity Attacks); "Moving Around Other Creatures"; Desengajar | provoca quem sai do alcance de um hostil que o vê; Desengajar evita; o ataque vem "right before" a saída |
| Salto longo | "Special Types of Movement" (Jumping) | cada pé saltado custa um pé de movimento; precisa de 3 m a pé antes |
| Busca de armadilhas | "Traps in Play"; "Search" | Percepção ou Investigação; **qualquer personagem pode usar Arcanismo contra uma armadilha mágica** (o app não decide isso: o mestre acrescenta) |

## Revisão (adversarial, aplicada)

- **Cobertura só vale para teste de Destreza** (SRD 5.1, "Cover"). A Onda Trovejante (Constituição) não mostra bônus ("Sem bônus de cobertura: o teste é de Constituição"); `PreviewSpellAreaResponse.targets[].cover` e a linha genérica do passo 2 passam a depender da habilidade do teste. Nenhum texto de "+5 pelo canto" sobrou: a Bola de Fogo contornando cantos não dá bônus automático; o mestre marca a cobertura à mão.
- **Escudo Arcano:** a linha com "Vez" é a do Hobgoblin (o Escudo acaba no começo do turno do Pensantus); o instante em que acaba foi desenhado (PM-03a 1c), junto com a Ajuda de 3º nível (+10).
- **"Encerrar Ajuda"** é uma confirmação contornada em `danger-ink`, com o foco em "Cancelar".
- **RN-10 no salto:** a armadilha só aparece no mapa, na legenda, no cartão e no registro dos jogadores quando já foi revelada a quem lê; o caso principal (PM-03c) é o da armadilha não achada; o teste de vazamento do repositório precisa cobrir o salto sobre armadilha escondida. `ResolveHiddenReveal`: o jogador recebe o mesmo `permission_denied` para qualquer `pending_reveal_id`, checado antes de procurar o id.
- **Layout:** colunas iguais dentro de 48–1232 (PM-01a), caixas de números do resumo com a altura de dois rótulos e valor de 28 px, `font-family` nos dois botões, chips e "Acrescentar perícia" com 44 px, lista de perícias colada ao botão, linha do SRD sem sugerir o que já é etiqueta, rótulo do escudo a 13 px, rótulo e valor alinhados no cartão do salto, 8 px entre botões, contornos do mapa em `#8a1f33` com halo maior (acima de 3:1 sobre o pergaminho).
- **Dados de exemplo:** só uma sessão aberta (a 4); as sessões 1 a 3 em quintas anteriores (17/09, 24/09, 01/10) e o aviso da aberta no formato novo; nenhuma "Sessão 6"; "Machado grande", "Meio-elfo", "Ladino 4/11", "Bárbaro 9"; monstros só do SRD (Goblin, Hobgoblin); "Zuk" é um NPC do mestre (base: Goblin); "Pensantus (você), aqui".
- **Estados novos:** foco visível (PM-01a 5, PM-01c 9), jogador sem personagem na sessão (PM-01c 7), mestre no celular (PM-01c 8), duas perguntas ao mesmo tempo (PM-02c 9b; a espera segura o andamento do turno, não as reações), depois de recarregar (PM-02c 9c), Ajuda +10 e fim do Escudo (PM-03a 1c), a conta do Crítico Brutal nos níveis 13 e 17 (PM-03b 2b; o bônus de dano da Fúria é de outro bloco e a linha ganhará "+ 3 de Fúria" depois).
- **Não aplicado:** os seletores de cone, linha e cubo do mestre no desktop (a lista de "estados que faltam" os citava, mas a decisão não os incluiu); o seletor do mestre mostra só a Bola de Fogo. O mestre usa o mesmo passo do jogador, com a lista completa.

## Perguntas para o Vinicius

1. **Bola de Fogo contorna cantos** (texto da magia): a área é a parte da esfera ligada ao ponto, e quem é atingido só pelo canto não ganha bônus automático (o SRD não dá nenhum); **o mestre marca a cobertura à mão**, como hoje. Serve?
2. **Cone/linha em 8 direções.** Prefere ângulo livre? Fica mais fiel, mas o servidor teria que receber um ângulo e as regras de borda ficam menos previsíveis.
3. **O que é "dentro" do círculo:** centro do quadrado a até 4 quadrados do ponto (exato). A distância de hoje (RN-21) arredonda para baixo; usá-la aqui faria o raio de 6 m virar 7,5 m. Manter o exato?
4. **O turno parado pela pergunta** não tem tempo limite: espera o mestre ou o fim do combate. Quer um "Pular" depois de um tempo (por exemplo, se o mestre saiu)?
5. **Interruptor do mestre com "Perguntar a cada vez":** começa ligado ("Revelar"). Ou melhor sem valor inicial e obrigar a escolha?
6. **Ajuda:** o app não conta as 8 horas (acaba por descanso longo ou por "Encerrar Ajuda"). E o fim da Ajuda só corta o que passa do novo máximo (o SRD não diz). Confirma?
7. **Ajuda em quem está a 0 PV acorda** (o pedido já dizia; o SRD não diz se o aumento conta como "recuperar"). Ao zerar, os testes contra a morte também zeram?
8. **Segunda Ajuda sobre a primeira:** vale a mais forte, não soma (SRD, "Combining Magical Effects"). Aceita?
9. **Armadilha mágica:** o SRD diz que qualquer personagem pode usar Arcanismo contra armadilha mágica; pelo seu critério o app não presume isso. Quer que as predefinições mágicas (Estátua que cospe fogo) já venham com Arcanismo marcado, ou nenhuma?
10. **Busca com "Outra perícia…":** a penalidade de luz continua só da Percepção. Certo?
11. **Salto fora do alcance:** nenhum campo novo para o ataque de oportunidade (o servidor já resolve); só o aviso e a palavra "saltou". Serve para esta etapa?
12. **Data das sessões** ("qui., 1 de out., 19h05 às 22h47"): num formatador novo. Desenhei o aviso da sessão aberta também no formato novo ("desde qui., 8 de out., 19h05"). Trocar o "30/09 às 20:05" que existe hoje por ele em todo o app?
13. **PM-04** (reações) não foi desenhado, como pedido.
