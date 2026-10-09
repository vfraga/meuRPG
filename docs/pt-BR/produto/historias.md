> A versão em inglês ([stories.md](../../product/stories.md)) é a canônica.

# Histórias e critérios de aceite

O MVP tem 37 histórias: 35 mais 2 pré-requisitos (o convite, MR-002, que leva à MR-003, e os NPCs, MR-005, que são os inimigos do combate). Outras dez histórias ficam para depois do MVP.

Uma história está pronta quando todos os critérios dela passam. Cada critério vira um teste automático: Playwright para o que aparece na tela, teste em Go para a regra no servidor. Não há testes de caracterização do app antigo; o sistema novo só precisa provar os próprios critérios de aceite.

Como ler uma história: a frase da história, a prioridade (MVP, MVP pré-requisito ou Depois), as regras (RN-xx, em [Regras de negócio](regras.md)) e os módulos que ela toca, os critérios de aceite (Dado / quando / então) e "No app", um resumo curto do que existe hoje. As histórias "Depois" são propostas; o modelo de dados já nasce preparado para a MR-021 e a MR-022 (ver [Modelo de dados](../../data.md)).

## Índice

| ID | Área | Prioridade |
| --- | --- | --- |
| [MR-001](#mr-001-criar-campanha) | Campanha | MVP |
| [MR-003](#mr-003-entrar-pelo-convite) | Personagem | MVP |
| [MR-004](#mr-004-ficha-no-formato-do-pdf) | Personagem | MVP |
| [MR-006](#mr-006-ficha-travada) | Personagem | MVP |
| [MR-008](#mr-008-pontos-de-interesse) | Mapa | MVP |
| [MR-009](#mr-009-mapa-sem-spoiler) | Mapa | MVP |
| [MR-011](#mr-011-iniciar-a-sessão) | Sessão | MVP |
| [MR-012](#mr-012-acompanhar-a-sessão) | Sessão | MVP |
| [MR-013](#mr-013-ordem-dos-turnos) | Combate | MVP |
| [MR-014](#mr-014-sua-vez) | Combate | MVP |
| [MR-015](#mr-015-ações-da-cena-de-rp) | RP | MVP |
| [MR-016](#mr-016-dar-xp) | Progressão | MVP |
| [MR-018](#mr-018-documento-de-campanha) | Apoio | MVP |
| [MR-019](#mr-019-galeria-de-imagens) | Apoio | MVP |
| [MR-024](#mr-024-aprovar-o-personagem-do-convite) | Personagem | MVP |
| [MR-028](#mr-028-mostrar-uma-imagem-aos-jogadores) | Sessão | MVP |
| [MR-025](#mr-025-cadastrar-conteúdo-da-mesa) | Regras | MVP |
| [MR-010](#mr-010-gerar-masmorras) | Masmorra | MVP |
| [MR-029](#mr-029-ganchos-e-pistas-da-cena) | RP | MVP |
| [MR-030](#mr-030-anotações-do-jogador) | RP | MVP |
| [MR-031](#mr-031-npcs-na-cena) | RP | MVP |
| [MR-032](#mr-032-destaques-do-combate) | Combate | MVP |
| [MR-033](#mr-033-imprimir-o-mapa-com-a-grade) | Mapa | MVP |
| [MR-034](#mr-034-movimentos-especiais) | Combate | MVP |
| [MR-035](#mr-035-armadilhas) | Mapa | MVP |
| [MR-036](#mr-036-névoa-de-guerra-pela-visão) | Mapa | MVP |
| [MR-037](#mr-037-criaturas-do-personagem) | Combate | MVP |
| [MR-038](#mr-038-quebra-cabeças) | Sessão | MVP |
| [MR-039](#mr-039-imagens-geradas-para-masmorras-e-cenas) | Apoio | MVP |
| [MR-040](#mr-040-subir-de-nível-pela-ficha) | Progressão | MVP |
| [MR-041](#mr-041-tesouros-e-xp-por-ouro) | Mapa | MVP |
| [MR-042](#mr-042-bestiário) | Combate | MVP |
| [MR-043](#mr-043-gerar-encontros) | Combate | MVP |
| [MR-044](#mr-044-gerar-tesouro) | Mapa | MVP |
| [MR-045](#mr-045-consultar-as-magias) | Regras | MVP |
| [MR-002](#mr-002-gerar-convite) | Campanha | MVP (pré-requisito) |
| [MR-005](#mr-005-criar-npcs) | Personagem | MVP (pré-requisito) |
| [MR-007](#mr-007-importar-ficha-em-pdf) | Personagem | Depois |
| [MR-017](#mr-017-subir-de-nível) | Progressão | Depois |
| [MR-020](#mr-020-consultar-o-livro-de-regras) | Apoio | Depois |
| [MR-021](#mr-021-copiar-personagem) | Personagem | Depois |
| [MR-022](#mr-022-reutilizar-npcs) | Personagem | Depois |
| [MR-023](#mr-023-passar-ou-dividir-a-campanha) | Campanha | Depois |
| [MR-026](#mr-026-propor-uma-raça-ou-classe-nova) | Regras | Depois |
| [MR-027](#mr-027-ler-as-regras-de-um-pdf) | Regras | Depois |
| [MR-046](#mr-046-o-estilo-da-mesa-recurso-por-recurso) | Regras | Depois |
| [MR-047](#mr-047-mais-quebra-cabeças) | Sessão | Depois |

## Prioridade: MVP

### MR-001: Criar campanha

**Como** mestre, **quero** criar uma campanha que reúna as sessões, os personagens e os mapas, **para** organizar cada mesa separadamente.

- Prioridade: MVP
- Regras: RN-05, RN-30
- Módulos: campaigns

#### Critérios de aceite
- **Dado** que estou logado, **quando** crio a campanha "Mirathel", **então** viro mestre dela **e** só os membros a veem na lista.
- **Dado** que já sou mestre do máximo de campanhas da conta (10, por padrão), **quando** crio outra, **então** nada é criado **e** a tela mostra, no formulário, uma frase com o máximo (RN-30).
- **Dado** que o servidor só deixa alguns e-mails criarem campanhas, **quando** uma conta de outro e-mail tenta criar, **então** é recusada com o motivo e a tela explica; quem entra por convite continua jogando (RN-30).

#### No app
- Módulo `campaigns` (ver [Arquitetura](../../architecture.md#campaigns-module-and-authorization)).
- Página `/campaigns` (`web/src/app/pages/campaigns/`): a lista mostra o papel em cada campanha (Mestre ou Jogador), uma linha por campanha, e o formulário "Criar campanha". A lista vazia explica como criar uma campanha ou entrar por convite.
- O teto por conta (RN-30) e a lista de quem cria são aplicados pelo servidor; a tela mostra a frase do teto (ou a de uma conta que a lista não permite) como um alerta no formulário de criação quando o servidor recusa; ela não conhece o teto antes disso.
- Testes: `TestMR001_CreatorBecomesMasterAndOnlyMembersSeeTheCampaign`; `TestRN30_TheCampaignCapPerAccount`, `TestRN30_TheCapHoldsUnderConcurrentCreations`, `TestRN30_TheCreatorsAllowList`; a frase do teto em `campaigns.spec.ts` (Vitest); Playwright `o mestre cria uma campanha pela tela e a vê como mestre na lista` (`@MR-001`, `e2e/tests/campaigns.spec.ts`).

### MR-003: Entrar pelo convite

**Como** jogador, **quero** criar meu personagem pelo link do convite, **para** ele já entrar vinculado à campanha.

- Prioridade: MVP
- Regras: RN-03
- Módulos: characters, campaigns

#### Critérios de aceite
- **Dado** um convite válido para "Mirathel", **quando** o jogador abre o link e faz login com o Google, **então** vira jogador da campanha e cria um personagem do tipo jogador, que o mestre já vê na campanha.
- **Dado** um convite expirado, **quando** alguém abre o link, **então** vê uma mensagem clara **e** nada é criado.

#### No app
- Com o usuário logado, o convite vira participação como jogador. Aceitar de novo não muda nada. Dois jogadores disputando o último uso não entram os dois.
- Quem não está logado entra num passo só: `POST /auth/login` com a intenção `campaign_invite` faz o login e aceita o convite, sem guardar nada no navegador, e cai em `/campaigns/<id>` ou em `/invite/error?reason=<código>`.
- Página `/invite` (`web/src/app/pages/invite/invite-accept.ts`): lê o token do fragmento da URL, o apaga da URL na hora (`history.replaceState`) e aceita o convite (logado) ou oferece "Entrar para aceitar o convite" (deslogado).
- O jogador cria o próprio personagem, do tipo jogador, com `CharacterService.CreateCharacter`. Ele nasce como rascunho. O mestre o vê na lista da campanha, com o nome de exibição do jogador, a classe e a raça (`ListCharacters`). O editor da criação é o editor em passos da [MR-004](#mr-004-ficha-no-formato-do-pdf).
- Testes: `TestMR003_ValidInviteMakesTheUserAPlayerTheMasterSees`, `TestMR003_ExpiredInviteGivesAClearErrorAndCreatesNothing`, `TestMR003_PlayerCreatesTheirCharacterAndTheMasterSeesIt`; com provedor falso e CockroachDB: `TestSignInWithAnInviteJoinsTheCampaign`, `TestSignInWithAnUnusableInvite`, `TestSignInWithAnInviteAsAMember`. Playwright (`@MR-003`; em `e2e/tests/invite.spec.ts`): `jogador já logado abre o link do convite, entra na campanha e o mestre o vê nos membros`, `visitante sem sessão entra pelo convite, faz login e é adicionado à campanha automaticamente` (que também confere que o token não aparece em nenhuma URL pedida pelo navegador) e, em `e2e/tests/characters.spec.ts`, "o jogador entra pelo convite, cria o personagem e o mestre o vê na campanha".

#### Relacionadas
- RN-03: o jogador só cria um personagem novo nesta campanha quando o atual morre; o personagem morto fica no sistema (ver [Regras de negócio](regras.md)).
- RN-17 trata do login do jogador sem Google (handle por mesa; ainda não construído): ele entra sem senha e, quando a primeira sessão de 30 dias vence, precisa definir uma senha ou vincular o Google (ADR-0009, opção 3).
- Quando o convite exige aprovação (RN-15), o jogador entra como membro pendente, vai direto criar o personagem, e o personagem nasce pendente até o mestre aprovar ou recusar. Ver [MR-024](#mr-024-aprovar-o-personagem-do-convite).

### MR-004: Ficha no formato do PDF

**Como** jogador, **quero** ver minha ficha num formato parecido com o PDF oficial, **para** achar tudo onde estou acostumado.

- Prioridade: MVP
- Regras: —
- Módulos: characters, rules

#### Critérios de aceite
- **Dado** um personagem completo, **quando** o jogador abre a ficha no celular, **então** vê as seções da ficha oficial (habilidades, perícias, combate, magias, equipamento) **e** os valores calculados, como modificadores e CD de magia, vêm prontos do servidor.

#### No app
- O "Equipamento" da ficha lista a armadura, o escudo, as armas e os itens, depois o equipamento do antecedente em texto ("Do antecedente: …": o de um antecedente da mesa, ou o que o jogador escreveu para um antecedente "Outro"), depois as moedas. Os "Ataques" mostram o dano com duas mãos de uma arma versátil sob o dano ("Com duas mãos: 1d8+1").
- `CharacterService.GetCharacter` devolve a ficha como o jogador a preencheu e, junto, os valores calculados pelo servidor (`DerivedSheet`): habilidades e modificadores, testes de resistência, perícias, passivas, CA, PV, deslocamento, sentidos, CD e ataque de magia, espaços (os espaços de Magia de Pacto do Bruxo numa lista própria, "Espaços do pacto", pois são todos do mesmo nível), magias, ataques, características e as pendências da ficha. Teste: `TestMR004_SheetComesWithServerCalculatedValues`, com o Pensantus (INT 18, +4; CD 14; ataque de magia +6; CA 13; PV 23). Playwright: "o jogador abre a ficha no celular e vê as seções da ficha oficial com os valores calculados pelo servidor".
- Perícias, etapa "Perícias": as perícias que a raça, a sub-raça e o antecedente já dão aparecem marcadas e travadas, com a origem de cada uma ("da raça", "do antecedente"), e não são escolha: a contagem das escolhas as deixa de fora, como faz o servidor (`Race.skill_keys`, `Subrace.skill_keys` e `Background.skill_keys` em `ContentService.ListContent`), e uma escolha que o jogador fez numa delas não é enviada. O SRD deixa trocar uma proficiência repetida por outra do mesmo tipo (Antecedentes), então o jogador escolhe outra perícia. Teste: `TestSkillsTheRaceAndBackgroundGiveAreNoPicks` (`rules`); Vitest `skill-picker.spec.ts` e `character-editor.spec.ts`.
- Pontos de experiência de um personagem novo: criado acima do nível 1 numa campanha em que se sobe de nível por XP de inimigos derrotados, o campo começa com o XP do nível dele (a tabela do SRD, `Content.level_xp`: 6.500 no nível 5) e acompanha o nível até o jogador digitar; o mestre edita como antes, e uma campanha de marcos mantém 0.
- Subclasse no editor: oferece "Nenhuma", diz em que nível a classe a escolhe ("O Bárbaro escolhe a subclasse no nível 3.") e troca a subclasse ao trocar de classe.
- Etapa "Magias": lista só as magias até o maior nível da magia do nível atual, em ordem de nível e depois de nome. Uma magia já escolhida acima do limite continua na lista, marcada "acima do nível", para poder ser desmarcada. Classes que começam a conjurar depois (Paladino, Patrulheiro) mostram "O Paladino conjura magias a partir do nível 2." no nível 1. "Truques" só aparece para a classe que tem truques na lista dela (o Paladino e o Patrulheiro não têm), ou quando a ficha já tem um truque, para poder desmarcar. O maior nível por nível vem do servidor (`ClassSpellcasting.max_spell_level_by_level`, em `ContentService.ListContent`). Cada lista conta contra os números da classe, da prévia que o servidor faz do rascunho (`PreviewCharacter`, `Spellcasting.cantrips_known`, `spells_known` e `prepared_max`: "3 de 4 truques escolhidos"); uma classe que prepara (clérigo, druida, paladino, mago) com menos magias preparadas do que pode vê "Prepare até N", e o "Prepare mais N" da subida de nível diminui com as escolhas e some quando não resta nenhuma.
- Etapa "Habilidades": o jogador que cria uma ficha recebe os métodos que a mesa permite (RN-24, [MR-025](#mr-025-cadastrar-conteúdo-da-mesa)), e o servidor rola e guarda os 4d6. O editor livre abaixo é o que o mestre vê nos NPCs e o que a edição de uma ficha sem método registrado mantém. Nele, "Como definir os valores" tem três cartões: "Digitar" (os seis campos de sempre), "Rolar 4d6" e "Conjunto padrão".
  - "Rolar 4d6" rola seis vezes 4d6 no navegador (`crypto.getRandomValues`, sem viés; só neste editor livre), risca o menor dado de cada uma, lista os resultados do maior para o menor e tem "Rolar de novo", que troca os seis e desfaz a colocação.
  - O jogador coloca cada resultado numa habilidade. Do tablet para cima, cada habilidade vira um seletor (escolher um resultado que está em outra habilidade troca os dois). No celular, toca-se no resultado ("Escolhido") e depois na habilidade, com as palavras "Livre", "Escolhido" e "em Força".
  - "Conjunto padrão" faz o mesmo com 15, 14, 13, 12, 10 e 8, sem dados.
  - Enquanto sobrar resultado sem habilidade, o editor não salva e diz "coloque cada resultado numa habilidade" (nunca vira seis 10 sem ninguém ver).
  - Nada é guardado neste editor livre: a ficha só recebe o número colocado. As rolagens dele rodam no navegador, não ficam registradas e não entram na RN-18 (os 4d6 do jogador entram: RN-24). A ficha continua editável até a primeira sessão, e o mestre revisa.
- "Pontos de vida": "Rolado" mostra uma linha por nível a partir do 2º ("Nível 2 (1d12)"), "Rolar", "Rolar os níveis que faltam" (só os vazios; dá para digitar o que saiu no dado de mesa), a fórmula "1d12 (8) + 3 (Constituição) = 11 PV" com a Constituição final (valor, raça, sub-raça e bônus manual) e uma caixa com o total até agora e a faixa final. No multiclasse, cada nível rola com o dado da classe que o dá. O editor não salva enquanto um nível está sem rolagem ou com uma rolagem que não cabe no dado do seu nível, e o aviso sob os passos diz o nível ("Dado de vida do nível 3"). As linhas são os dados e o modificador de Constituição; o que a raça, a classe e as características somam (a Dureza Anã dá ao Anão da Colina 1 PV por nível, e um efeito da própria mesa também conta) é o número que o servidor calcula para o rascunho (`PreviewCharacter`, pedido depois de uma pausa de 300 ms; só vale a resposta mais nova) e uma linha curta o nomeia ("+1 PV de raça, classe ou característica"). Com todas as rolagens digitadas, o total da caixa é o máximo que a ficha salva tem. Enquanto a resposta não chega, a caixa guarda o último número; se a chamada falhar, mostra só os dados e avisa que é uma prévia que não conta esses pontos de vida.
- "?" das magias: ao lado de cada magia, um "?" de 44 px abre a descrição (diálogo no desktop, folha de baixo no celular): nome, nível da magia e escola, tempo de conjuração, alcance (em metros: 5 pés = 1,5 m), componentes e duração em português, as etiquetas Ritual e Concentração e o texto do SRD em inglês, marcado `lang="en"` ("Texto do SRD 5.1 (em inglês)", mais "Em níveis superiores" quando há). Vem de `ContentService.GetSpellDetails`, buscado na hora e guardado só enquanto a página está aberta. O que o formatador não sabe traduzir aparece como o texto cru do SRD, com a nota "(texto do SRD)".
- Testes: `TestMaxSpellLevelFromSlots`, `TestCatalogMaxSpellLevelByLevel` (`rules`), `TestCatalogToProtoMaxSpellLevel` (`characters`); Vitest `dice.spec.ts`, `hit-points-preview.spec.ts`, `spell-details-format.spec.ts`, `ability-scores.spec.ts`, `hit-points-rolls.spec.ts`, `spell-details.spec.ts` e `character-editor.spec.ts`; Playwright `@MR-004` em `e2e/tests/character-editor.spec.ts` (`o editor da ficha mostra só as magias do nível e deixa tirar a subclasse`, "rolar 4d6 dá seis resultados…", "o conjunto padrão pode ser colocado e trocado…", "os pontos de vida rolados preenchem todos os níveis que faltam", "o \"?\" ao lado da magia abre a descrição…"); `@a11y` em `a11y.spec.ts` ("as rolagens e a descrição da magia passam no axe…").

#### Relacionadas
- As regras como dados, com as fórmulas no Expr, calculam a ficha. Ver [ADR-0008](../../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

### MR-006: Ficha travada

**Como** mestre, **quero** que a ficha do jogador fique só para visualização a partir da primeira sessão, **para** só eu e o sistema alterarmos.

- Prioridade: MVP
- Regras: RN-01
- Módulos: characters

#### Critérios de aceite
- **Dado** que a primeira sessão da campanha já começou, **quando** o jogador tenta editar as habilidades da própria ficha, **então** o servidor recusa **e** o mestre consegue editar a mesma ficha.
- **Dado** que nenhuma sessão começou, **quando** o jogador edita a ficha, **então** a alteração é salva.
- **Dado** um personagem criado depois da primeira sessão, **quando** o jogador edita a ficha antes da próxima sessão, **então** a alteração é salva **e**, quando a próxima sessão começa, a ficha trava.
- **Dado** que a ficha travou, **quando** o jogador tenta editar a história do personagem, **então** o servidor recusa; **depois que** o mestre libera a história desse personagem, o jogador edita e salva, **e** a liberação acaba quando a próxima sessão começa.

#### No app
- A sessão começa por `PlayService.StartGameSession`, que trava as fichas na mesma transação. O jogador recebe `failed_precondition` com o motivo `SHEET_LOCKED` ou `STORY_LOCKED`. O mestre libera a história com `SetStoryEditing`; começar uma sessão desliga a edição da história de novo.
- A tela da ficha é só de leitura quando ela está travada, com os botões do mestre.
- O endereço de edição de uma ficha travada (`/campaigns/:id/characters/:characterId/edit`, digitado ou salvo) também mostra a trava antes de qualquer formulário: o editor lê `Character.can_edit` ao abrir e, sem permissão, mostra "Ficha travada" (ou "Personagem morto") com o motivo e "Voltar para a ficha", em vez de deixar o jogador preencher tudo e só descobrir ao salvar.
- Testes, um por critério: `TestMR006_AfterTheFirstSessionOnlyTheMasterEditsTheSheet`, `TestMR006_BeforeAnySessionThePlayerEditsTheSheet`, `TestMR006_CharacterCreatedAfterTheFirstSessionLocksAtTheNextOne` (e, no `play`, com a sessão de verdade, `TestCharacterCreatedLaterLocksAtTheNextSession`), `TestMR006_AfterTheLockTheStoryNeedsTheMastersPermission` (e, no `play`, `TestStartingASessionTurnsStoryEditingOff`); Playwright `e2e/tests/sheet-lock.spec.ts`, que também confere o endereço de edição travado.

### MR-008: Pontos de interesse

**Como** mestre, **quero** criar pontos de interesse que abrem uma batalha, um submapa ou uma cena de RP.

- Prioridade: MVP
- Regras: —
- Módulos: maps

#### Critérios de aceite
- **Dado** um mapa da campanha, **quando** o mestre cria um ponto do tipo batalha, submapa ou cena de RP, **então** o ponto aparece no mapa **e** abrir o ponto leva ao encontro, ao submapa ou à cena.

#### No app
- O `MapService` cria, muda, move e apaga mapas e pontos dos três tipos (ver [Arquitetura](../../architecture.md#maps-module-maps-points-and-tokens)). O mapa nasce de uma imagem da galeria e nasce escondido; o ponto também. O ponto de submapa leva a outro mapa da mesma campanha, nunca ao próprio mapa. Teste: `TestMR008_MasterCreatesPointsOfEachKind`.
- Telas: "Novo mapa" (nome e uma imagem da galeria), o painel "Mapas" da campanha e o editor do mestre no computador. No editor, escolher o tipo e clicar no mapa põe o ponto (escondido); arrastar ou as setas movem; o painel do ponto salva tudo junto em "Salvar ponto"; "Adicionar token". No celular o mestre só anda, dá zoom e revela pelas listas. Ver [Design](../../design.md#maps-and-the-shown-image).
- Renomear e apagar um mapa: no cabeçalho do mapa, "Renomear" ao lado do nome e "Apagar mapa" na ponta direita. Apagar pergunta ali mesmo o que vai junto (os pontos e os tokens; a imagem fica na galeria) e avisa quando é o mapa atual da sessão aberta. Antes de qualquer clique, o botão já fica cinza (ainda clicável) com o motivo embaixo dele, na ordem do servidor (há um combate no mapa, um tesouro virou XP, um tesouro foi encontrado; só o primeiro é dito), e quando o servidor recusa mesmo assim a pergunta diz qual é o motivo, com as mesmas palavras, em vez de dizer que o servidor não responde. Apagado, o app volta para a campanha.
- Abrir o ponto: o de submapa mostra o nome e a descrição, com "Abrir <mapa>" (primeiro a ficha do ponto, depois o submapa). Os de batalha e de cena abrem o encontro com o combate e a cena de RP (ver [MR-013](#mr-013-ordem-dos-turnos) e [MR-015](#mr-015-ações-da-cena-de-rp)); fora de uma sessão, abri-los só mostra a ficha do ponto.
- Testes: `maps.spec.ts` (`@MR-008`: o mestre cria o mapa e os três pontos pela tela, o jogador abre o submapa pela ficha do ponto, o mestre renomeia o mapa e o apaga depois de confirmar) e `a11y.spec.ts`.

### MR-009: Mapa sem spoiler

**Como** jogador, **quero** ver no mapa só os pontos que meu grupo já conhece, **para** não receber spoiler.

- Prioridade: MVP
- Regras: RN-10
- Módulos: maps

#### Critérios de aceite
- **Dado** um mapa com um ponto revelado e outro escondido, **quando** o jogador abre o mapa, **então** só o revelado aparece **e** a resposta do servidor não contém o escondido.

#### No app
- O `MapService` decide no servidor o que cada um vê (RN-10). O jogador recebe só os pontos revelados, só os tokens visíveis, e só os mapas revelados ou o mapa atual da sessão. Um mapa escondido é `not_found` para ele, igual a um mapa que não existe.
- A tela do jogador (`/campaigns/<id>/maps/<mapa>`) desenha só o que chegou, com a trilha do submapa, "Mapas revelados" e "Pontos deste mapa". Um mapa escondido mostra "Mapa não encontrado".
- Testes: `TestMR009_PlayersNeverReceiveHiddenPoints` lê a resposta do jogador como o JSON que o app recebe e confere que não há o ID, o nome nem a descrição do ponto escondido, e que uma mudança só em coisas escondidas chega pelo stream só ao mestre. `TestRN10_PlayersCannotOpenHiddenMaps` cobre os mapas e os submapas. `maps.spec.ts` (`@MR-009`) abre o mapa com o jogador, lê a resposta de `GetMap` que a própria página recebeu e confere que ela não traz o ID nem o nome do ponto escondido.

### MR-011: Iniciar a sessão

**Como** mestre, **quero** iniciar a sessão, que os jogadores recebam uma notificação no app e ter um link da sessão para mandar a eles, **para** todos entrarem juntos.

- Prioridade: MVP
- Regras: RN-06, RN-07
- Módulos: play, campaigns, characters

#### Critérios de aceite
- **Dado** uma campanha com três jogadores, **quando** o mestre inicia a sessão, **então** quem está com o app aberto vê a notificação **e** o mestre pode copiar o link da sessão.
- **Dado** o link da sessão, **quando** alguém que não é membro abre o link, **então** vê "peça um convite ao mestre" **e** não entra.
- **Dado** que é a primeira sessão da campanha, **quando** o mestre inicia a sessão, **então** as fichas dos jogadores travam.

#### No app
- Antes de o mestre iniciar a sessão, o cartão "Sessão" lista os personagens de jogador cuja ficha ainda tem escolhas em aberto ("Ilaria: faltam 1 perícia"; perícias, truques, magias conhecidas e preparadas, de `CharacterSummary.open_choices`, que só o mestre recebe) e pergunta "Iniciar mesmo assim?": as fichas travam do jeito que estão, e o mestre decide. Sem nenhuma ficha nesse estado, a sessão começa de uma vez. Testes: `TestListCharactersTellsTheMasterWhichSheetsHaveChoicesOpen` (`characters`), `game-session-card.spec.ts`.
- `PlayService.StartGameSession` abre a sessão e trava, na mesma transação, as fichas dos jogadores que ainda são rascunho. Teste: `TestRN01_StartingTheFirstSessionLocksPlayerSheetsOnly` (no `play`).
- Notificação: `PlayService.ListOpenGameSessions` diz ao app, a cada 30 segundos com a aba visível, quais sessões estão abertas nas campanhas da pessoa (`TestListOpenGameSessions`). O aviso "A sessão 4 de Mirathel começou." fica embaixo da barra do app, com "Entrar na sessão" (para quem joga na campanha, fora da página dela e das páginas de sessão; fechar vale só para a aba). Na página da campanha quem avisa é o cartão "Sessão", e o cartão do jogador acompanha a mesma consulta. RN-06: a notificação em tela basta; não há notificação push do navegador.
- Outras entradas: o link "Ao vivo" na barra, a etiqueta "Sessão ao vivo" em "Minhas campanhas" e o painel "Sessão" da campanha, com "Entrar na sessão" e "Copiar link da sessão" (copiar o link é só da tela).
- Página da sessão `/campaigns/<id>/session`: lê `GetLiveSession` e abre o stream `WatchGameSession`. Os dois respondem `not_found` a quem não é membro e ao membro pendente (a tela diz "Peça um convite ao mestre", sem o nome da campanha) e `failed_precondition` com `NO_OPEN_SESSION` sem sessão aberta (`TestWatchGameSessionRefuses`, `TestAuthorizationMatrix`). Um membro sem sessão aberta vê "Nenhuma sessão em andamento", e a página abre a sessão sozinha quando o mestre inicia.
- Testes: `live-session.spec.ts` (`@MR-011`), com o jogador com o app aberto enquanto o mestre inicia a sessão pela tela.

#### Relacionadas
- RN-07: o convite tem padrão de 1 uso e 7 dias; o mestre escolhe de 1 a 20 usos e de 5 minutos a 30 dias, e pode revogar (ver [MR-002](#mr-002-gerar-convite)).

### MR-012: Acompanhar a sessão

**Como** jogador, **quero** acompanhar minha ficha e o mapa atual durante a sessão.

- Prioridade: MVP
- Regras: RN-02, RN-10, RN-11, RN-20
- Módulos: play, maps

#### Critérios de aceite
- **Dado** uma sessão ativa, **quando** o mestre move um token ou o sistema aplica dano ao personagem, **então** o celular do jogador mostra a mudança sem recarregar a página **e** as notas do mestre nunca aparecem.

#### No app
- Metade da ficha, servidor: o mestre corrige PV, PV temporários, espaços de magia e dados de vida durante a sessão (`AdjustCharacterVitals`, RN-02). A mudança chega na hora, pelo stream `WatchGameSession`, ao mestre e ao dono do personagem, nunca a outro jogador. Nada da sessão ao vivo carrega as notas do mestre (RN-11). Testes: `TestRN02_MasterAdjustsVitalsDuringSession`, `TestPlayersSeeOnlyTheirOwnVitals`, `TestLiveStreamIsNotBuffered`, `TestRN11_LiveSessionNeverCarriesMasterNotes`.
- Metade da ficha, telas: o jogador vê os PV, os PV temporários, a CA, os dados de vida e os espaços de magia do próprio personagem, e o número muda na tela quando o mestre corrige, sem recarregar. O mestre vê o grupo e corrige em "Ajustar" (uma folha no celular, um diálogo no desktop), que não passa do máximo da ficha e, depois do fim da sessão, diz "A sessão acabou". Sem conexão, a página diz "Reconectando…" com a hora da última atualização, e os números continuam na tela. Testes: `live-session.spec.ts` (`@MR-012`, `@RN-02`) e `a11y.spec.ts`.
- Metade do mapa, servidor: o mestre escolhe o mapa atual da sessão (`PlayService.SetCurrentMap`, que também o revela) e move os tokens (`MapService.PlaceMapToken`). O stream leva `current_map_changed`, `token_moved` e `map_changed`, e o jogador só ouve falar do que ele vê (RN-10). Testes: `TestMR012_TokenMovesReachPlayersLive` (o token visível chega ao jogador; o token escondido de um NPC, só ao mestre) e `TestSetCurrentMap`.
- Metade do mapa, telas: o mapa atual aparece no lugar do aviso "O mestre ainda não escolheu um mapa.", para o jogador (uma prévia que abre o mapa inteiro) e para o mestre (o seletor "Mapa atual", tokens que se arrastam, "Pontos do mapa" e "Tokens no mapa" com "Revelar aos jogadores" e "Esconder"; a criatura de um personagem tem a sua própria linha, sempre visível e sem botão, e arrastar o token de uma criatura move a criatura, não o dono). A página lê o mapa de novo em `map_changed` (se o servidor responde `not_found`, o jogador perdeu a vista do mapa e volta ao aviso), troca o mapa em `current_map_changed` e move o token em `token_moved` sem ler nada. Teste: `maps.spec.ts` (`@MR-012`: o mestre escolhe o mapa e move um token pelo teclado; a página aberta do jogador mostra o mapa e a nova posição sem recarregar).
- No combate, o jogador vê os inimigos por uma palavra (Ileso, Ferido, Muito ferido, Derrotado), não pelo PV (RN-20).
- "O sistema aplica dano": um ataque que acerta abre um dano pendente. No NPC o dano é aplicado na hora (PV temporários primeiro; a 0 PV ele fica derrotado e sai da ordem). No personagem do jogador ele espera o mestre, que o aplica ("Aplicar 5 de dano") ou o descarta. Ao aplicar, a mudança chega ao dono e ao mestre ao vivo (`vitals_changed`). O dano de uma magia segue o caminho do dano de um ataque. Uma cura é aplicada na hora (e levanta quem estava a 0 PV). O mestre aplica uma quantia diferente da rolada (`TestMasterAppliesADifferentAmount`). O dano em quem está a 0 PV conta uma falha no teste contra a morte (RN-03). O mestre também mexe nos PV de um NPC ("Dano/Cura"), desfaz a última ação e lê o registro do combate, que o jogador recebe só com o que vê (RN-20). Testes: `TestMR012_DamageToAnNPCIsAppliedAndDefeatsIt`, `TestRN02_DamageToAPlayerWaitsForTheMaster`, `TestCombatUndoRestoresTheLastAction`, `TestTimelineRound1And2Log`, `TestRN20_PlayersNeverReceiveCAOrHiddenLogEntries`, `TestCombatLogChangedPerAudience`. Ver [Arquitetura](../../architecture.md#combat).

#### Relacionadas
- RN-02: sim, o mestre pode corrigir PV e espaços de magia na mão durante a sessão; o mestre tem a palavra final (ver [Regras de negócio](regras.md)).

### MR-013: Ordem dos turnos

**Como** jogador, **quero** ver a ordem dos turnos, onde cada um está e quanto posso me mover, **para** planejar minha ação.

- Prioridade: MVP
- Regras: RN-19, RN-20, RN-21
- Módulos: play, rules

#### Critérios de aceite
- **Dado** um combate com a iniciativa definida, **quando** o jogador abre a tela de combate, **então** vê a ordem dos turnos, onde está cada combatente visível e quanto ainda pode se mover neste turno.
- **Dado** um combate com a grade, **quando** abro a economia do turno ou o mapa, **então** o movimento aparece também em quadrados ("7,5 m · 5 quadrados").
- **Dado** combatentes lado a lado na ordem com o mesmo total de iniciativa, jogadores inclusive, **quando** a ordem dos turnos é mostrada, **então** eles formam um turno conjunto, numa caixa com a palavra "Turno conjunto" e o total. O mestre vê todos os grupos; o jogador só vê um grupo que tem um personagem de jogador (um grupo só de NPCs é do mestre, RN-20).
- **Dado** um turno conjunto, **quando** a vez chega ao grupo, **então** a vez de todos os membros começa de uma vez, cada um com a sua ação, ação bônus, reação, movimento e teste contra a morte; o jogador de cada membro encerra a própria parte, o mestre encerra a de qualquer um, e o turno só passa quando o último encerra (não existe "encerrar o turno do grupo", nem reabrir uma parte).
- **Dado** que o jogador encerra a própria parte, **quando** toca em "Encerrar a minha parte", **então** o app pergunta antes ("Encerrar a sua parte?", com o que ainda sobra e "Voltar" primeiro), porque a parte não reabre.

#### No app
- Servidor: o `CombatService` cria o combate no mapa com grade (o mapa tem `grid_columns`, `MapService.SetMapGrid`), põe o grupo e as cópias dos NPCs, rola a iniciativa de cada NPC, recebe a do jogador (no app ou o d20 físico, RN-18), deixa o mestre ordenar os empates, começa o combate, passa a vez (a rodada sobe depois do último; o movimento, a ação e a reação voltam no começo da vez de cada um) e deixa andar na grade com o limite do movimento que sobra. O jogador nunca recebe um combatente escondido nem o número de um NPC (RN-20), e a vez de um escondido aparece como "Vez do mestre". O ponto de batalha do mapa pode apontar o mapa do combate. Ver [Arquitetura](../../architecture.md#combat).
- Telas do mestre, na página da sessão: "Combate" com "Iniciar combate" (nome, o mapa com a grade, o grupo e como cada jogador rola, os NPCs com quantas cópias e "Escondido no início"). Um mapa sem grade manda para "Grade do mapa" (`/campaigns/:id/maps/:mapId/grid`, de 4 a 200 quadrados na largura, a mesma faixa do editor de mapas, as linhas pela proporção da imagem). Iniciativa: totais com a conta `1d20 (15) + 4 = 19`, o empate uma vez só com as setas dentro do grupo, "Esperando …" com "Digitar pelo jogador", "Começar o combate" bloqueado com o motivo, "Posições iniciais". Com o combate andando: a barra ("Vez do …", "Rodada 2", "Próximo turno", "Encerrar combate", que pergunta na própria barra), o mapa com a grade e todos os tokens, e a ordem com PV, "Dano/Cura" dos jogadores, revelar e esconder, remover e "Adicionar combatente".
- Telas do jogador: a iniciativa por "Rolar no app" ou "Digitar o resultado" (conforme RN-18), o total em 92 px e "Esperando o mestre começar o combate". Com o combate andando: "Vez do …" (ou "Vez do mestre" quando é um escondido), "Você é o próximo", a ordem em fichas só com os visíveis e as palavras de estado, "Sua vez" com o movimento, "Mover" e "Encerrar turno", e a página "Mover" (alcance; "Mover 3 m. 2 quadrados para a direita e 1 quadrado para baixo. Depois restam 4,5 m."; "Longe demais: faltam 1,5 m"; "Ocupado"; no computador, arrastar o token dentro do alcance). No fim, "Combate encerrado" para os dois. A tela segue a sessão ao vivo: `encounter_changed` lê o combate de novo, `turn_changed` e `combatant_moved` entram no lugar.
- Distâncias em quadrados: toda distância sai de um só lugar, `core/units.ts` (1 quadrado = 1,5 m = 5 pés). O quadro "Movimento" diz "7,5 m de 7,5 m" e "5 quadrados livres"; a barra do turno, "Mover 7,5 m · 5 quadrados" (numa grade de duas colunas para o número não quebrar); o cartão do NPC do mestre, "Deslocamento 9 m · 6 quadrados"; a ficha, "7,5 m · 5 quadrados (25 pés)" numa linha; a página "Mover" e os grupos de ações usam as mesmas funções.
- Turno conjunto: o grupo é calculado da ordem e dos totais quando a vez começa (`nextTurnGroup`, `combat_turn.go`) e fica guardado em `combatants.turn_state` (`idle`, `acting`, `ended`, migration `00089`), então reordenar um empate, trazer reforço ou esconder um membro não muda uma vez que já começou.
  - `EndTurn` encerra a parte de um membro (`expected_combatant_id` é o membro; o `aborted` garante que um toque nunca encerra duas partes nem duas vezes) e o turno passa quando ninguém mais age. Cada parte encerrada é um evento `turn_part_ended` e uma linha do registro ("Brisa encerrou a parte dela"; a de um membro escondido não chega ao jogador).
  - Toda checagem de "é a vez dele" (`MoveCombatant`, ataques, ações, magias, testes contra a morte, reações, `GetTurnOptions`) é "está no turno e a parte não terminou".
  - O servidor manda ao jogador `Encounter.turn_group_ids` (só os membros que ele vê), `Combatant.turn_part_ended` (só de grupo com personagem de jogador), `npc_only_groups` (só para nomear "os Goblins") e, para os jogadores do mesmo grupo, a economia uns dos outros.
  - Telas: a caixa "Turno conjunto" na ordem do mestre e na do jogador, o cartão do mestre com um bloco por membro e "Encerrar a parte da Brisa", a pílula "Turno conjunto com Brisa", "O que a Brisa ainda tem", "Encerrar a minha parte" com a pergunta no lugar, "Você encerrou a sua parte", "Vez de Brisa e Toren" e "Vez dos Goblins".
- Testes de servidor: `TestMR013_TurnOrderAndMovementLeft`, `TestRN19_EachNPCRollsItsOwnInitiative`, `TestRN20_PlayersNeverReceiveHiddenCombatantsOrNPCNumbers`, `TestRN21_PlayerMovementIsLimitedTheMasterIsNot`, `TestEndTurnIsIdempotent`, `TestStartEncounterNeedsAGrid`, `TestMR013_CombatAuthorizationMatrix`, `TestMR013_CombatEventsPerAudience`, `TestMR013_CombatantsStartOnTheirTokensAndEndWhereTheyStand`. Turno conjunto: `TestMR013_PlayersWithTheSameInitiativeShareATurn`, `TestMR013_TheTurnPassesWhenTheLastMemberEnds`, `TestMR013_EachMemberHasItsOwnEconomy`, `TestMR013_TheMasterEndsAPartForAnAbsentPlayer`, `TestMR013_APlayerCannotEndAnotherMembersPart`, `TestMR013_ADoubleTapEndsOnePartOnly`, `TestMR013_AnUndoStaysClosedAcrossAPartEnding`, `TestMR013_AMemberWhoLeavesDoesNotBlockTheTurn`, `TestMR013_AReinforcementWithTheSameTotalActsFromTheNextTurn`, `TestMR013_EachMemberOwesItsOwnDeathSave`, `TestMR013_OrderingATieKeepsTheGroup`, `TestMR013_EndTurnPartAuthorization`, `TestRN20_NPCOnlyGroupsStayTheMasters`, `TestRN20_AHiddenMemberIsNeverNamed`.
- Testes de tela: `combat.spec.ts` (`@MR-013`: a grade, o início com o grupo e três goblins escondidos, a iniciativa no app, o empate, "Próximo turno", "Vez do mestre", mover dentro do alcance e a recusa além dele, o fim, "a distância sai em metros e em quadrados"), `joint-turns.spec.ts` (`@MR-013`, `@RN-20`), `a11y.spec.ts` ("o combate passa no axe…", `scanJointTurnScreens`); Vitest `combat-grid.spec.ts`, `combat-view.spec.ts`, `combat-state.spec.ts`, `combat-errors.spec.ts`, `initiative-setup.spec.ts`, `turn-panel.spec.ts`, `order-list.spec.ts`, `units.spec.ts` (5, 25, 30 e 35 pés, 0 e pés ímpares), `joint-turn.spec.ts`. A suíte e2e só tem duas pessoas logadas, então o grupo do e2e é Pensantus e a Brisa como NPC aliada; o grupo só de jogadores é coberto pelos testes do servidor e do Vitest.

#### Relacionadas
- Cada NPC rola a própria iniciativa (RN-19); o jogador vê o estado dos inimigos por uma palavra, nunca o PV nem a CA (RN-20); cada quadrado da grade vale 1,5 m, inclusive na diagonal, e o app não deixa o jogador passar do movimento do turno (RN-21). Ver [Regras de negócio](regras.md).
- O deslocamento disponível vem do motor de regras (regras como dados). Ver [ADR-0008](../../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).
- O grupo de turno agrupa NPCs e jogadores, e os jogadores também podem atacar em conjunto (descrito acima).

### MR-014: Sua vez

**Como** jogador, na minha vez, **quero** ver minhas ações, ações bônus e ataques possíveis.

- Prioridade: MVP
- Regras: RN-02, RN-03, RN-18, RN-20, RN-22
- Módulos: play, rules

#### Critérios de aceite
- **Dado** um combate, **quando** chega a vez do Pensantus, **então** o jogador vê ação, ação bônus, reação e movimento disponíveis **e** as magias sem espaço de magia aparecem desabilitadas.
- **Dado** que o Pensantus conjura Mísseis Mágicos (Magic Missile) com um espaço de 1º nível, **quando** a ação é confirmada, **então** o sistema marca o espaço como usado.
- **Dado** um personagem com magias, **quando** abre a lista de magias na vez dele, **então** as que ele pode conjurar agora vêm primeiro, depois as outras, cada grupo por nível.
- **Dado** uma magia na lista durante a sessão, **quando** o jogador toca no "?", **então** vê a descrição completa, como no editor.
- **Dado** uma magia que lê pontos de vida (Sono, Leque Cromático, Palavra de Poder Atordoar e Matar, Estabilizar, Cura Completa), **quando** ela é conjurada, **então** o servidor a resolve com o PV real dos alvos **e** o jogador continua vendo só "Ileso", "Ferido" ou "Muito ferido" (RN-20): quem conjurou vê a própria rolagem e quem foi afetado, nunca o PV de um alvo.

#### No app
- Motor de regras: `rules/combat.Options` calcula a economia (ação, ação bônus, reação, movimento com a Disparada), os ataques, as magias com os níveis possíveis e as ações padrão e das features, e marca cada opção desabilitada com um código de motivo (`NO_SLOT`, `ACTION_USED`, `NO_USES`, `NOT_YOUR_TURN`…). Ver [Arquitetura](../../architecture.md#combat-engine-and-spell-details). Cada magia tem detalhes estruturados (`ContentService.GetSpellDetails`).
- `CombatService.GetTurnOptions` devolve a economia (com o movimento em pés), os ataques com os alvos à vista e a distância (RN-21), as magias e as ações padrão, e, fora da vez, tudo desabilitado com o motivo `NOT_YOUR_TURN`. Cada alvo de magia vem com a distância e os dardos por nível (`spell_targets`).
- O ataque tem dois passos: `RollAttack` (gasta a ação; o d20 vai ao servidor, que compara com a CA e devolve só Acertou, Errou ou Crítico) e `RollDamage`. `TakeAction` faz as ações padrão que só gastam a economia (a Disparada dobra o movimento que sobra).
- `CastSpell` conjura e **gasta o espaço e a ação na hora** (critério 2). Resolve pelo que a magia é (ataque de magia; resistência rolada pelo servidor com uma rolagem de dano para a conjuração inteira; os dardos dos Mísseis Mágicos; cura; ou só registro) e põe a concentração. A retentativa não gasta de novo; o `NO_SLOT` traz o nível mínimo.
- Reações: o Escudo vira um aviso quando um golpe acerta o personagem (`reaction_prompts`, `UseReaction`, `DeclineReaction`); o ataque de oportunidade é o `RollAttack` com `as_reaction`; o Ataque Extra deixa mais de um ataque por ação; as ações das features (Retomar o Fôlego, Surto de Ação…) gastam o uso do recurso.
- Também no servidor: o "Caído", os testes contra a morte e a confirmação do mestre (RN-03), as condições e o lembrete de concentração (RN-22).
- Ordem das magias: a lista de `GetTurnOptions` já vem na ordem do critério 3 (`rules/combat.Options`: as que dá para conjurar agora primeiro, depois as outras; em cada grupo, os truques primeiro, depois por nível e pelo nome em português). O Escudo Arcano, que na vez do personagem nunca dá para conjurar, fica entre as outras, com o motivo `REACTION_ONLY_WHEN_HIT`, que a tela mostra como "Só fora da sua vez".
- As magias que leem PV (Sono, Leque Cromático, Palavra de Poder Atordoar e Matar, Estabilizar, Cura Completa) são conteúdo escrito à mão (`effects/spells.json`, quatro tipos fechados: `hp_pool`, `hp_threshold`, `zero_hp_target` e `flat_heal`). `CastSpell` as resolve com o PV real, o do NPC do combate e o do personagem de jogador da ficha viva, com a rolagem do total no app ou digitada (`pool_sum`, RN-18) e a condição pelo mesmo código das condições; "Desfazer última ação" devolve tudo. Quem conjurou vê a própria rolagem e quem foi afetado; o mestre vê o total, o PV de cada um e a ordem; os outros jogadores veem só quem foi afetado (RN-20). Dobre pelos Mortos (Toll the Dead) não está no SRD 5.1 e não entra: o repositório é público e só tem SRD.
- Tela "Sua vez": "O que você pode fazer" agrupa o que o servidor calcula em Ação (Ataques, Magias, Ações padrão), Ação bônus, Reação e Movimento, cada grupo com a palavra de estado ("Disponível", "Usada") e cada opção desabilitada com o motivo em palavras ("Ação já usada", "Sem espaço de 2º nível ou maior"), sem tirar a opção do lugar nem do teclado. As ações padrão são uma grade de duas colunas com uma só linha de motivo. A partir de 1024 px a vez é uma faixa compacta com "Encerrar turno" à direita, e os quadros de economia ficam no painel; de 1280 px a ordem é uma coluna à esquerda.
- Folha do ataque: "Atacar com Raio de Fogo" abre uma folha no celular (um diálogo no desktop; ela rola por dentro, com os botões grudados embaixo) em três passos, Alvo, Rolar e Dano. O alvo é um grupo de rádio com o estado e a distância ("Longe demais: alcance de 36 m", desabilitado). O d20 e o dano rolam no app ou se digitam ("Digite o resultado do dado", de 1 a 20; o dano, de N a N × faces) conforme RN-18. O resultado mostra `1d20 (13) + 6 = 19`, "Acertou", "Crítico" ou "Errou" e o alvo derrotado, nunca a CA.
- "Encerrar turno" é contornado enquanto a ação ou a ação bônus está livre e vira o botão cheio quando as duas acabam; com a ação livre pergunta no lugar do botão.
- Folha de conjurar: "Conjurar" abre a folha do celular (diálogo no desktop) com o espaço de magia em rádios ("1 livre de 4"; o nível sem espaço é tracejado, "Sem espaço livre"), os alvos de `spell_targets` (distância, "Longe demais", o limite de alvos) e os dardos dos Mísseis Mágicos em passos de 44 px com o contador "3 de 3 dardos distribuídos". O aviso "É o seu último espaço de 1º nível" (e o do Escudo Arcano, quando é o último que ele teria) vem antes do botão cheio "Conjurar X". Uma magia de ataque troca o botão pelo d20 em dois jeitos (RN-18), um por alvo. O resultado lista cada alvo (d20, "Falhou" / "Resistiu: metade", "Dardo 1: 1d4 (3) + 1 = 4"), o dano que falta rolar logo abaixo (uma rolagem para a conjuração toda quando é área, uma para cada alvo nos dardos), "Espaços de 1º nível: 0 livres de 4", "Escudo Arcano indisponível", a concentração e "Sua ação foi usada". Uma magia sem efeito conhecido diz "A magia foi conjurada: o mestre resolve o efeito." Um truque de resistência (Chama Sagrada) é conjurado, não atacado.
- As habilidades de classe têm "Usar" (Retomar o Fôlego rola o d10 numa folha e mostra a cura; o Surto de Ação diz "Você tem outra ação"; `NO_USES` vira "Sem usos: volta num descanso curto"). O Ataque Extra mostra "1 ataque restante" na Ação com os ataques ainda habilitados ("Ataques desta ação já usados" depois).
- O aviso do Escudo: o golpe que o Escudo Arcano pode parar abre sozinho o `alertdialog` "Você foi atingido", com quem atacou e com quê quando o jogador vê o atacante (foco em "Não usar", sem saída sem resposta); o cartão do mestre troca ao vivo quando o jogador responde. O ataque de oportunidade é a ação de texto sob "Sua reação" (folha só com ataques corpo a corpo, gasta a reação).
- Quem está a 0 PV vê "Brisa está caída", as marcas e "Rolar teste contra a morte" (RN-03); o mestre confirma a morte no lugar, no topo do cartão ("Confirmar a morte" / "Ainda não"). As condições (15 do SRD; "Derrubado" é o `prone`) e a concentração estão em "Condições…" no ⋮ da ordem, como etiquetas sob o nome, na faixa do jogador e como ponto no token; o jogador encerra a própria concentração. O mestre aplica **outro valor** ("Aplicar outro valor") e, quando o alvo concentra, lê "Teste de Constituição, CD 10". O registro tem as frases de magia, reação, teste contra a morte, morte confirmada e condições, cada uma como o servidor a manda a cada plateia.
- Vez do mestre: o cartão "Ações do Capitão Goblin" (PV, CA, deslocamento, o ataque, o campo "Alvo", "Rolar ataque" no app ou digitado, "Acertou contra CA 18 do Toren", o dano e "Aplicar 5 de dano" / "Não aplicar", que pergunta "Descartar o dano de 5?"). No celular e no tablet o cartão é a vez inteira e termina em "Próximo turno", com "Encerrar combate" e "Desfazer última ação" no fim da página. Enquanto há dano sem aplicar, "Próximo turno" pergunta "Há dano sem aplicar. Passar o turno mesmo assim?". Na ordem, "CA n" (só o mestre) e "Dano/Cura" também nos NPCs (`AdjustCombatantHitPoints`). Dois campos só do mestre: `Combatant.armor_class` e `AttackRoll.target_armor_class`.
- Registro do combate: `ListCombatLog` (lido de novo a cada `combat_log_changed` e a cada reconexão) mostra as rodadas, a mais nova primeiro, com o ícone de magia nos ataques de magia. O mestre desfaz a última ação nomeando-a ("Desfazer o ataque do Capitão Goblin ao Toren (5 de dano)?"). A linha do cabeçalho diz "Em andamento desde 20:05" para todos.
- Tela da lista de magias: as magias de todas as economias formam uma lista só, na ordem que o servidor manda (não se ordena de novo), com os espaços de cada nível acima ("1º nível ○ ✕ ✕ ✕ 1 livre de 4") e o motivo repetido só "Sem espaço". Cada linha tem o "?" de 44 px, que nunca apaga, e a folha de conjurar o tem no cabeçalho. Ele abre os detalhes da magia (tempo, alcance, componentes, duração e o texto do SRD em inglês) numa folha no celular e num diálogo do tablet para cima, por cima da folha de conjurar sem perder a escolha. O nível é uma etiqueta em linha própria ("Reação" ao lado, no Escudo Arcano); o Escudo na sua vez não tem botão e diz "Só fora da sua vez". `SpellDetails` fica em `shared/spell-details/`.
- Magias que leem PV na tela: na folha de conjurar, Sono pede o total dos dados (no app ou digitado) e quem está na área, só pelo nome. O resultado do jogador diz "O Goblin 1 adormeceu. O Capitão Goblin não foi afetado.", a rolagem dele (`5d8 (2, 4, 1, 5, 3) = 15`) e nenhum PV. No registro, o mestre lê um cartão com o total, cada criatura do menor PV ao maior, a conta que sobra, a palavra com o ícone e "Mudar as condições"; o jogador só a frase.
- Testes do motor (`rules/combat`): `TestOptionsPensantus` (com 1 espaço de 1º nível livre de 4 e 0 de 2 livres de 2, Teia e Passo Nebuloso ficam `NO_SLOT` com mínimo 2 e Mísseis Mágicos só aceita o 1º; Raio de Fogo +6 1d10), `TestOptionsToren` (Machado de batalha +5 1d8+3; Retomar o Fôlego é ação bônus, 1 uso por descanso curto), `TestOptionsWarlockPactMagic`, `TestOptionsMovement`, `TestSpendSlot`, `TestSpendResource`, `TestMR014_SpellsSortByAvailabilityThenCircle`, `TestResolvePool`, `TestResolveThreshold`, `TestResolveZeroHP`, `TestResolveFlatHeal`; em `rules`: `TestResourcesAndActions`, `TestAttackDice`, `TestGetSpellDetails`, `TestSpellDetailsExamples`.
- Testes de servidor: `TestMR014_TurnOptionsFollowTheEconomy`, `TestRN18_PhysicalRollsAreTypedSums`, `TestMR014_CastingSpendsTheSlot`, `TestTimelineRound3MagicMissile`, `TestSaveSpellRollsOnceForTheCast`, `TestHealingSpellRevivesAndResetsDeathSaves`, `TestShieldTurnsAHitIntoAMiss`, `TestOpportunityAttackSpendsTheReaction`, `TestExtraAttackAllowsTwoAttacks`, `TestSecondWindAndActionSurge`, `TestRN03_DeathSavesAndTheMasterConfirms`, `TestRN22_ConditionsAndTheConcentrationReminder`, `TestMR014_SleepUsesTheRealHitPoints` (o total de 15 põe o Goblin de 7 PV para dormir e deixa o Capitão de 27 PV acordado, e o jogador nunca recebe um PV), `TestMR014_SleepSkipsTheUnconsciousAndTheOneAtZero`, `TestMR014_ColorSprayBlindsByThePool`, `TestMR014_PowerWordStunAndKillOnNPCs`, `TestMR014_PowerWordKillOnAPlayerCharacterWaitsForTheMaster`, `TestMR014_SpareTheDyingWorksOnlyAtZero`, `TestMR014_CompleteHealHealsAndEndsBlindnessAndDeafness`, `TestRN18_PoolSpellsFollowTheDiceMode`.
- Testes de tela: `combat.spec.ts` (`@MR-012`, `@MR-014`, `@RN-02`, `@RN-03`, `@RN-22`, `@RN-20`: o ataque com dados físicos, o ataque no app, o fim do turno com a Disparada, o ataque do mestre com aplicar, descartar e desfazer, "Dano/Cura", o registro sem o goblin escondido, Mísseis Mágicos com os dardos e o último espaço, a resistência em dois goblins, a cura, os testes contra a morte e a confirmação, o Escudo nas duas telas, as condições e a concentração, outro valor com o lembrete, o guerreiro com Ataque Extra, Retomar o Fôlego e Surto de Ação, o ataque de oportunidade, "as magias vêm na ordem do servidor", "Sono em dois goblins e no Capitão"); `a11y.spec.ts` ("agir no combate", "conjurar e cair"); specs Vitest de `core/combat` (`combat-dice`, `combat-options`, `combat-log`, `combat-grid` com `tight`, `attack-flow`, `turn-options-state`, `cast-flow`, `death-saves`, `combat-errors`, `cast-result`, `hp-effects`, `hp-spells-log`), de componentes (`roll-picker`, `end-turn`, `next-turn`, `action-row`, `order-column`, `action-groups`, `open-spell-details`, `pool-card`), `session-time.spec.ts` e `TestRN20_PlayersNeverReceiveCAOrHiddenLogEntries`.

#### Relacionadas
- RN-02: o mestre pode corrigir PV e espaços de magia na mão (ver [Regras de negócio](regras.md)).
- Dados (RN-18): o mestre escolhe como a campanha rola (cada jogador escolhe, todos no app ou todos com os próprios dados) e cada jogador guarda a sua preferência, na página da campanha (`TestRN18_DiceSettings`, `TestEffectiveDiceMode`, `dice.spec.ts` `@RN-18`). Com o dado físico, o jogador digita a soma dos dados e o app soma o modificador (`dice.Physical`, `TestPhysical`). As rolagens do combate seguem essa regra: o d20 do ataque e o dano, o d20 do ataque de magia, a cura, o d10 do Retomar o Fôlego e o teste contra a morte.
- O jogador vê se acertou ou errou e o dano, não a CA nem a rolagem do NPC (RN-20). Condições e concentração só são marcadas e lembradas, e o mestre decide os efeitos (RN-22). Na terceira falha no teste contra a morte, o personagem só morre quando o mestre confirma (RN-03).
- Quais ações, ações bônus, reações e recursos o sistema conhece vem do motor de regras (regras como dados). Ver [ADR-0008](../../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

### MR-015: Ações da cena de RP

**Como** jogador, **quero** ver numa lista simples as ações que o mestre escolheu para a cena,
**para** saber o que posso rolar e usar fora de combate.

- Prioridade: MVP
- Regras: RN-10, RN-18, RN-20
- Módulos: maps, play, rules

#### Critérios de aceite
- **Dado** uma cena de RP com as ações que o mestre escolheu para ela, **quando** o jogador abre a cena, **então** vê a lista de ações escolhidas pelo mestre, cada rolagem com o bônus do próprio personagem já calculado (ex.: Investigação) **e** as habilidades que só valem em combate não aparecem.
- **Dado** um combate (MR-013, MR-014), **quando** o jogador vê as ações possíveis, **então** quem decide essa lista é o sistema, pelas regras de D&D — nunca o mestre. A cena de RP é o único lugar em que o mestre escolhe a lista.
- **Dado** uma cena, **quando** o mestre liga "Mostrar a CD aos jogadores" (por cena; o padrão é desligado), **então** o jogador vê a CD das ações que têm uma e se passou nas próprias rolagens; desligado, ele vê só o total (RN-20). Uma ação sem CD não mostra nada a mais, e o mestre vê a CD sempre.
- **Dado** uma ação da cena, **quando** o mestre define as tentativas por jogador (1 por padrão, outro número ou sem limite) e pode dar mais uma tentativa a um jogador, **então** o jogador rola até o limite dele e vê quantas tentativas lhe restam ("Restam 2 de 3 tentativas", "Sem mais tentativas"). Fechar e abrir a cena de novo zera as contas; baixar o limite abaixo do que alguém já gastou o deixa sem tentativas, sem apagar nada.

#### No app
- O mestre escolhe as ações no ponto de cena do mapa, uma por vez (`AddSceneAction`, `UpdateSceneAction`, `MoveSceneAction`, `RemoveSceneAction`): perícia, teste de habilidade ou teste de resistência, com nome opcional (até 60 caracteres) e CD opcional (1 a 30), no máximo 20, e nada de combate (a chave é conferida no catálogo das regras).
- O mestre abre a cena na sessão (`OpenScene`: qualquer ponto de cena abre, mesmo sem ações, e pode estar escondido, e então continua escondido no mapa) e fecha sem perguntar (`CloseScene`); todos recebem `scene_changed`.
- O jogador lê a cena (`GetOpenScene`) com o nome e a descrição do ponto, as ações com o bônus do próprio personagem (a ficha, `rules.SceneOptions`) e a passiva de Percepção, Investigação e Intuição. A CD só vem quando o mestre liga "Mostrar a CD aos jogadores".
- Rolar (`RollSceneCheck`): o d20 do app ou o digitado (RN-18) mais o bônus, até o limite de tentativas da ação. O mestre recebe `scene_check_rolled` e vê todas as rolagens, com o total e o "passou" quando há CD (o registro da cena). O jogador vê só as próprias, com o "passou" só quando a cena mostra a CD.
- Opções da cena no servidor: o ponto de cena tem `show_dc` (`UpdateMapPoint` / `CreateMapPoint`) e cada ação, `max_attempts` (1 a 5, 0 sem limite; `AddSceneAction` / `UpdateSceneAction`). `OpenSceneInfo.show_dc`, `SceneActionView.max_attempts` / `attempts_left` e `SceneRoll.attempts_left` (só do mestre) dão à tela o que ela precisa. `GrantSceneAttempt` é o "Dar mais uma tentativa".
- Editor: o painel de um ponto de cena tem "Ações da cena" (a lista com ↑ ↓, o lápis "Editar" e remover; o formulário no lugar com o erro da CD, o mesmo formulário preenchido para editar a verificação, o nome e a CD de uma ação, com "Salvar ação" apagado até um campo mudar; o estado vazio e o limite de 20 ações). No alto de "Ações da cena" fica o interruptor "Mostrar a CD aos jogadores" (por cena, desligado de início, salvo na hora; ligado, mostra como o jogador vê a CD). Cada ação tem "Tentativas por jogador" (uma lista de 44 px: 1 a 5 ou "Sem limite", salva na hora).
- Na sessão, o mestre tem "Cena de RP" com "Abrir cena" (o seletor, com a cena escondida e a sem ações), a cena aberta no topo da coluna do mapa com as ações, as rolagens ao vivo com Passou / Não passou e "Trocar cena" / "Fechar cena". Também dá para abrir a cena pelo ponto, em "Pontos do mapa" ("Abrir cena" ou "Trocar para esta cena"). A cena aberta do mestre diz, no alto de "Ações", "Os jogadores veem a CD" ou "Só você vê a CD", o limite de cada ação e, em cada rolagem, "Tentativa 1 de 3" e "Dar mais uma tentativa". Esse botão só aparece na última rolagem do personagem naquela ação, pergunta no lugar com "Voltar" primeiro e, com a CD à mostra, só é oferecido se a rolagem falhou — ou quando a ação não tem CD.
- O jogador tem o bloco "Cena" com o próprio bônus e "Rolar", a folha de rolar (no app ou digitando o dado físico) e a linha "Rolada". Lê "CD 12", "Passou · CD 12" / "Não passou · CD 10" (também na folha do resultado) e as tentativas que restam: "1 tentativa", "Restam 2 de 3 tentativas", "Restam N tentativas" (acima do limite), "Sem mais tentativas" ou nada, se a ação é sem limite. A região viva diz "O mestre deu mais uma tentativa em …". "Rolar" fica no mesmo lugar em toda linha (na última linha do cartão no celular). Uma ação que o jogador esgotou mostra o último resultado e "Rolada às 21:12" (sem a etiqueta da CD).
- **Com um combate na tela, os blocos da cena não são desenhados** (nem para o mestre, nem para o jogador): a cena continua aberta no servidor e volta à tela quando o combate termina.
- Testes de servidor: `TestMR015_PlayerSeesTheMastersActionsWithTheirBonus`, `TestMR015_NothingCombatOnlyInAScene`, `TestRN20_APlayerNeverGetsADCOrAnotherPlayersRoll`, `TestMR015_OneRollPerActionWhileTheSceneIsOpen` (a conta com o limite padrão de 1), `TestRN18_SceneRollsFollowTheDiceMode`, `TestMR015_AHiddenPointCanBeOpenedAndStaysHidden`, `TestMR015_OpeningAndClosingAScene`, `TestMR015_SceneActionRules`, `TestMR015_RollingNeedsALivingCharacter`, `TestSceneAuthorizationMatrix`, `TestMR015_ShowTheDCToPlayers`, `TestMR015_AttemptsPerAction`, `TestMR015_OneMoreAttempt`. Ver [Arquitetura](../../architecture.md#rp-scenes).
- Testes de tela (`e2e/tests/scenes.spec.ts`, `@MR-015`, `@RN-20`): "o mestre escolhe as ações no ponto de cena, abre a cena na sessão, o jogador rola uma no app e uma com o dado físico e o mestre vê as duas com Passou e Não passou", "o jogador rola cada ação uma vez; o mestre fecha a cena sem pergunta e, ao abrir de novo, as ações voltam a poder ser roladas", "o mestre abre uma cena escondida: os jogadores veem a cena e o ponto continua escondido no mapa deles", "o mestre abre a cena pelo ponto na lista da sessão", "o mestre liga a CD da cena e dá 3 tentativas…", "com a CD desligada o jogador não vê CD nem Passou…"; `a11y.spec.ts` ("as cenas de RP passam no axe…"). O Vitest cobre o editor das ações, o seletor, as linhas do jogador, a folha de rolar, a linha da rolagem do mestre, o texto de cada motivo de `SceneBlocked` e as mensagens da região viva (`scene-roll-line.spec.ts`, `scene-player.spec.ts`, `scene-actions.spec.ts`, `scene-view.spec.ts`).

#### Relacionadas
- Na cena de RP, o mestre escolhe as ações possíveis da cena, e o jogador vê o que pode fazer com o próprio bônus; no combate, quem decide e mostra as ações é o sistema, pelas regras de D&D. Ver [Regras de negócio](regras.md) e [ADR-0008](../../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).
- As cenas têm testes de perícia, de habilidade e testes de resistência (magias e habilidades ficam para depois do MVP); o mestre abre a cena, e um ponto revelado também a abre; o registro da cena existe.
- Quais habilidades aparecem na lista, e o bônus de cada uma, vêm do motor de regras (regras como dados). Ver [ADR-0008](../../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

### MR-016: Dar XP

**Como** mestre, **quero** dar XP ao grupo por inimigos derrotados, por ouro ou por marcos, conforme a campanha, ou quando eu quiser.

- Prioridade: MVP
- Regras: RN-09, RN-12
- Módulos: progression

#### Critérios de aceite
- **Dado** uma campanha no modo por inimigos e dois goblins derrotados (50 XP cada), **quando** o encontro termina, **então** os 100 XP são divididos entre os quatro personagens do grupo, 25 para cada.
- **Dado** uma campanha no modo por marcos, **quando** o mestre registra um marco, **então** os personagens que o mestre escolhe (todos os vivos do grupo vêm marcados) ficam marcados para subir de nível **e** nenhum XP é contado.
- **Dado** os modos por inimigos ou por ouro, **quando** o mestre dá XP ao grupo por conta própria, **então** o XP entra nas fichas **e** o histórico da campanha mostra quem deu, quando e por quê.
- **Dado** uma campanha por inimigos, **quando** o combate termina, **então** o mestre usa o ND do NPC ou digita o XP (os dois jeitos valem por igual) **e** escolhe quem recebe a divisão.
- **Dado** uma campanha por marcos com os marcos planejados antes pelo mestre, **quando** o grupo chega a um deles e o mestre o marca, **então** os personagens escolhidos "Podem subir de nível".
- **Dado** o modo por ouro, **quando** o mestre quer dar XP, **então** digita as PO; com os tesouros do mapa, "Voltar à cidade" converte em XP o que o grupo achou ([MR-041](#mr-041-tesouros-e-xp-por-ouro)), e digitar as PO continua valendo.

#### No app
- Módulo `progression` (ver [Arquitetura](../../architecture.md#progression-module-xp-and-milestones)). RPCs: `AwardXP` (por inimigos, por ouro ou avulso), `MarkMilestone`, `UndoLastXPAward`, `ListXPAwards`, `GetCampaignExperience` e, para os marcos planejados, `ListMilestones`, `AddMilestone`, `UpdateMilestone`, `MoveMilestone`, `RemoveMilestone`, `MarkMilestoneReached` e `GiveMilestoneTo`; `ListTreasuresToConvert` atende a [MR-041](#mr-041-tesouros-e-xp-por-ouro). O XP de cada NPC fica na ficha (`challenge_rating`, `xp_value`) e no combatente (só o mestre vê). "Pode subir de nível" (`can_level_up`) vem em `GetCharacter` para o mestre e o dono (RN-12). O resto de uma divisão é arredondado para baixo, perdido e avisado ("2 XP se perdem na divisão"). No modo por ouro, 1 XP por 1 PO (RN-09).
- Editor do NPC, "Ao ser derrotado": o ND preenche o XP, e "Usar 50 XP" volta ao da tabela.
- Fim do combate: "Experiência do combate" com "Dar 116 XP a cada um" (e "Agora não", que deixa a linha "XP do combate ainda não dado").
- "Dar XP" a qualquer hora, por inimigos, por ouro ou avulso, e "Registrar marco". "Experiência" na página da campanha, com o histórico para todos e o "Desfazer" do último prêmio, perguntado no lugar. A ficha mostra o XP como bloco de leitura e "Pode subir de nível" (também na lista do grupo), que se atualiza sozinha enquanto há sessão.
- Marcos planejados: o mestre escreve os marcos antes (até 120 caracteres cada, no máximo 100, na ordem que quiser: subir, descer, editar no lugar, remover perguntando no lugar; um marco que já foi alcançado e depois desfeito não pode ser removido, porque os prêmios dele ficam no histórico: o app diz isso e deixa de oferecer "Remover" para ele) e só ele vê a lista. Marcar um como alcançado e escolher quem sobe de nível deixa os escolhidos em "Pode subir de nível"; o marco passa para "Marcos alcançados" com o dia, a hora e quem, e a ordem não é cobrada. "Dar a mais alguém" dá o mesmo marco a um personagem que ficou de fora, num novo prêmio do histórico. Desfazer o último prêmio devolve o marco a planejado (se era a única marca) ou o mantém alcançado para os outros (se era de "Dar a mais alguém"). O jogador vê só os marcos alcançados e o próprio personagem (a linha só aparece depois do primeiro marco, e o estado vazio nunca sugere que há planejados). "Registrar um marco fora da lista" continua, como ação de texto.
- Limite conhecido: a página da campanha, vista pelo jogador, não escuta o stream da sessão, então o painel do jogador só se atualiza ao abrir a página ou quando a aba volta ao primeiro plano (`visibilitychange`); a página do mestre escuta o stream enquanto há sessão aberta (avisos de subida de nível, MR-040). A atualização ao vivo ali para o jogador fica para depois.
- Testes de servidor: `TestMR016_EnemiesAwardSplitsTheDefeated` (dois goblins de 50 XP, quatro personagens, 25 para cada), `TestMR016_MilestoneMarksWithoutXP` (marca todos, nenhum XP, a marca some quando o nível sobe), `TestMR016_ManualAwardIsInTheHistory` (quem, quando, por quê, quanto), `TestGoldAwardGivesOneXPPerGoldPiece`, `TestRemainderIsLostAndReported`, `TestUndoTakesBackOnlyTheLastAward`, `TestSecondEnemiesAwardForTheSameEncounterIsRefused`, `TestModeMustFitTheCampaign`, `TestOnlyLivingPlayerCharactersGetXP`, `TestPlayersNeverWrite`, `TestAuthorizationMatrix`, `TestRN20_PlayersNeverGetAnNPCsXP`, `TestMR016_PlannedMilestones`, `TestMR016_ReachingAPlannedMilestoneLetsTheChosenLevelUp`, `TestMR016_GiveAReachedMilestoneToSomeoneElse`, `TestMR016_UndoOfAPlannedMilestone`, `TestMR016_AMilestoneMarkedOffTheListIsReachedToo`, `TestRN20_PlayersSeeOnlyReachedMilestones`, `TestMR016_PlannedMilestonesNeedAMilestonesCampaign`.
- Testes de tela (Playwright `e2e/tests/xp.spec.ts`, `@MR-016`): "o XP de um combate: o mestre dá pelo resumo, a ficha do jogador sobe ao vivo e o histórico guarda" (também `@RN-12`), "\"Agora não\" deixa o XP do combate para depois, e a linha abre \"Dar XP\" com o motivo e o total", "um prêmio avulso, a qualquer hora: os erros aparecem ao sair do campo e o histórico mostra o prêmio", "por ouro: o mestre digita as peças de ouro e cada um recebe a sua parte", "por marcos: o marco marca quem pode subir de nível, sem nenhum número de XP, e a marca some quando o mestre sobe o nível" (também `@RN-12`), "desfazer o último prêmio: a pergunta fica no lugar, o foco vai para \"Voltar\", e o histórico guarda o desfazer", "o XP que o NPC dá: o ND preenche o XP, o valor digitado fica e \"Usar\" volta ao da tabela" (`@RN-20`); `e2e/tests/milestones.spec.ts` (`@MR-016`, `@RN-12`, `@RN-20`); `a11y.spec.ts` ("as telas de XP passam no axe e nas conferências de layout", `scanMilestoneScreens`). A conta da divisão na tela ("116 XP para cada", "2 XP se perdem na divisão") é provada no Vitest, já que a mesa do e2e tem um jogador só.

#### Relacionadas
- Como o XP é dado: o ND ou o XP digitado, o mestre decide quem recebe, arredondar para baixo, o aviso de subir de nível para o mestre e o jogador, o histórico para todos. Ver [RN-09](regras.md) e [RN-12](regras.md).
- O que cada personagem ganha ao subir de nível vem do motor de regras (regras como dados). Ver [ADR-0008](../../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

### MR-018: Documento de campanha

**Como** mestre, **quero** um documento de campanha com texto, imagens, links para mapas e fichas que abrem num modal, **para** manter as anotações da campanha num lugar só.

- Prioridade: MVP
- Regras: —
- Módulos: campaigns (o documento); maps e characters (as imagens, os mapas e as fichas dos links)

Existia no app antigo ([app antigo](../../legacy-app.md)).

#### Critérios de aceite
No MVP, só o mestre vê o documento.

- **Dado** o documento da campanha, **quando** o mestre escreve texto, põe uma imagem da galeria e um link para um mapa e para uma ficha, **então** o documento mostra a imagem **e** o link abre o mapa ou a ficha numa janela, sem sair do documento.
- **Dado** um jogador, **quando** pede o documento, **então** o servidor recusa (só o mestre vê o documento).

#### No app
- Telas: `/campaigns/:id/document` e o painel "Documento da campanha" na página da campanha, só para o mestre (ver [Design](../../design.md#campaign-document), [Arquitetura](../../architecture.md#campaign-document) e [Modelo de dados](../../data.md#tables-by-module)).
- Contrato: `CampaignDocumentService`, com `GetCampaignDocument` e `UpdateCampaignDocument` (`campaign_document.proto`). Um documento por campanha, em Markdown, até 200 KiB. Salvar confere a revisão lida (`aborted` se alguém salvou antes). Tabela: `campaign_documents`.
- Os links do próprio app: `[texto](map:<id>)`, `[texto](character:<id>)` e `![legenda](image:<id>)`. O servidor guarda o texto como veio e não abre os links; o app os resolve pelas chamadas de sempre, com a autorização de sempre.
- A janela do mapa mostra o mapa com todos os pontos (os escondidos marcados "Escondido"), a legenda, "Mapa atual da Sessão N" quando a sessão aberta está nele e "Abrir no editor de mapas".
- O jogador vê só "Só o mestre vê o documento da campanha", e a página da campanha não mostra o painel. O servidor responde `permission_denied` ao jogador e `not_found` a quem não é membro e ao membro pendente.
- Testes: `TestMR018_MasterWritesTheCampaignDocument`, `TestMR018_PlayersCannotReadTheDocument`, `TestCampaignDocumentAuthorizationMatrix`, `TestUpdateCampaignDocumentRefusesAStaleRevision`, `TestSavingTheSameDocumentTwiceIsNotAConflict`, `TestUpdateCampaignDocumentFirstSavesRace`, `TestUpdateCampaignDocumentValidates`, `TestCampaignDocumentGoesWithTheCampaign`; Playwright `campaign-document.spec.ts` (`@MR-018`: barra de ferramentas, salvar, recarregar, as janelas do mapa e da ficha, o conflito entre duas abas, o aviso ao sair sem salvar, os alvos apagados).

### MR-019: Galeria de imagens

**Como** mestre, **quero** uma galeria de imagens **para** usar nos documentos e nos mapas.

- Prioridade: MVP
- Regras: RN-10
- Módulos: maps

Existia no app antigo.

#### Critérios de aceite
Limites: JPEG, PNG ou WebP, até 10 MB por imagem, 300 imagens e 500 MB por campanha.

- **Dado** o mestre na galeria, **quando** envia uma imagem JPEG, PNG ou WebP de até 10 MB, **então** ela aparece na galeria **e** o arquivo guardado não tem os metadados (EXIF) do original.
- **Dado** um jogador, **quando** pede a galeria da campanha, **então** o servidor recusa.
- **Dado** uma imagem usada num mapa, **quando** o mestre tenta apagá-la, **então** o app diz em qual mapa ela está.

#### No app
- Servidor: o envio (`POST /uploads/images`), as imagens e as miniaturas (`GET /images/{id}` e `/images/{id}/thumb`) e o `GalleryService` (listar, renomear, apagar). Aceita só JPEG, PNG e WebP, recusa imagem com pixels demais e grava a imagem codificada de novo, sem nenhum metadado (ver [Arquitetura](../../architecture.md#maps-module-gallery-and-images)).
- `/campaigns/:id/gallery` (`web/src/app/pages/gallery/`), só do mestre: a cota; a área de envio (o botão "Enviar imagem" ou arrastar arquivos para a página, vários de uma vez, enviados um depois do outro, cada um com o próprio progresso e "Cancelar envio"); o lembrete de privacidade; a grade da mais nova para a mais antiga; renomear no lugar; apagar com confirmação no lugar; a janela da imagem, com anterior e próxima. O app confere o tipo e o tamanho antes de enviar, e cada recusa, do app ou do servidor, vira um aviso em português com o nome do arquivo. O jogador vê "Só o mestre vê a galeria da campanha."; quem não é membro, "Campanha não encontrada". Ver [Design](../../design.md#gallery-and-images).
- O painel "Galeria" da página da campanha, só para o mestre: as 5 imagens mais novas, a cota e "Abrir galeria".
- O seletor de imagem da galeria (`web/src/app/shared/gallery-picker/`), usado no formulário "Novo mapa", no editor do documento e em "Mostrar imagem": escolhe uma imagem ou envia uma nova e já a escolhe.
- Cada imagem diz os mapas que a usam (`GalleryImage.used_in_maps`); o cartão mostra "Usada em Mirathel e arredores". Apagar uma imagem que um mapa usa dá `failed_precondition` com o detalhe `ImageInUse`; o cartão diz "Essa imagem está num mapa. Troque a imagem do mapa antes de apagá-la."
- Testes: `TestMR019_MasterUploadsAnImageWithoutItsMetadata` (um JPEG com EXIF e GPS), `TestMR019_PlayersCannotListTheGallery`, `TestMR019_AnImageAMapUsesCannotBeDeleted`, `TestDeletingAnImageAMapUses`, `TestListGalleryImagesShowsWhereEachIsUsed`; Playwright `e2e/tests/gallery.spec.ts` (`@MR-019`: o arquivo baixado não tem o bloco EXIF; um texto com nome `.png` e um GIF mostram o erro em português; o jogador vê só o aviso; renomear, apagar, as setas da janela, o foco voltando ao cartão) e as varreduras do axe em `a11y.spec.ts`.

### MR-024: Aprovar o personagem do convite

**Como** mestre, **quero** aprovar ou recusar o personagem que um jogador criou pelo convite, **para** manter na campanha só os personagens que fazem sentido para a mesa.

- Prioridade: MVP
- Regras: RN-15
- Módulos: campaigns, characters

#### Critérios de aceite
O convite escolhe se exige aprovação, e a recusa apaga o personagem e a participação.

- **Dado** que sou mestre de "Mirathel", **quando** gero um convite, **então** posso marcar "Exigir aprovação do mestre" **e**, sem marcar, o convite funciona como antes: quem aceita entra direto.
- **Dado** um convite que exige aprovação, **quando** o jogador o aceita (já logado, ou fazendo login pelo convite), **então** vai direto criar o personagem, que nasce "Pendente de aprovação" **e**, enquanto espera, ele só vê o nome da campanha e o próprio personagem, que continua editando.
- **Dado** um personagem pendente, **quando** o mestre abre a campanha, **então** o vê em "Esperando aprovação", abre a ficha e, **quando** aprova, o personagem vira rascunho **e** o jogador passa a ser jogador da campanha.
- **Dado** um personagem pendente, **quando** o mestre o recusa, **então** o personagem é apagado, o jogador não entra na campanha **e** precisa de um convite novo para tentar de novo.
- **Dado** que sou jogador, ou jogador pendente, **quando** tento aprovar ou recusar um personagem, **então** o servidor recusa.
- **Dado** um jogador pendente que ainda não criou o personagem, **quando** o mestre abre a campanha, **então** vê quem está pendente sem personagem, com um botão para remover; **e**, passados 30 dias sem personagem, a participação pendente é apagada sozinha.
- **Dado** um jogador pendente de "Mirathel", **quando** ele aceita um convite comum (sem aprovação) da mesma campanha, **então** vira jogador na hora, porque o convite comum conta como a aprovação do mestre; se ele já tinha um personagem esperando, o personagem também é aprovado, e o convite gasta um uso. Um convite com aprovação, ou que não vale mais, não muda nada.

#### No app
- Contratos: `CreateInviteRequest.requires_approval` e `Invite.requires_approval`; `Campaign.awaiting_approval`; `CharacterService.ApproveCharacter` e `RejectCharacter`; `Character.can_approve`; os motivos `NOT_PENDING` e `AWAITING_APPROVAL` de `CharacterBlocked`. Tabelas: `campaign_invites.requires_approval` e `campaign_members.status` (`active` ou `pending`). Ver [RN-15](regras.md), [Arquitetura](../../architecture.md#pending-member) e [Modelo de dados](../../data.md#tables-by-module).
- O membro pendente só passa pelas chamadas da lista `pendingMayCall` do `authz`; em todas as outras, recebe `not_found`, e parece um desconhecido.
- Pendentes sem personagem: `ListPendingMembers`, `RemovePendingMember` e o prazo `campaign_members.pending_expires_at` (TTL por linha, 30 dias; criar o personagem o limpa).
- Telas: a caixa "Exigir aprovação do mestre" no formulário de convite; o convite leva o membro pendente direto a "Criar personagem"; o aviso "Esperando a aprovação do mestre" na campanha e na ficha; a lista "Esperando aprovação" do mestre, na seção "Personagens"; os botões "Aprovar personagem" e "Recusar personagem" (com "Confirmar recusa") na ficha. Na página da campanha, em "Membros", cada pessoa sem personagem aparece com a etiqueta "Sem personagem", a data de entrada e a data em que sai sozinha; "Remover" abre a confirmação ali mesmo, com o foco em "Cancelar". Se a pessoa criou o personagem nesse meio-tempo, a lista é atualizada e a mensagem manda aprovar ou recusar em Esperando aprovação. Só o mestre vê.
- Testes, por critério:
  - Primeiro: `TestMR024_MasterChoosesWhetherAnInviteRequiresApproval`, `TestRN15_InviteWithoutApprovalMakesAPlayerAtOnce`.
  - Segundo: `TestRN15_AcceptingAnInviteWithApprovalMakesAPendingMember`, `TestSignInWithAnApprovalInviteGoesToCreateTheCharacter`, `TestMR024_PendingPlayerCreatesTheirCharacterAndWaits`.
  - Terceiro e quarto: `TestMR024_MasterApprovesAndThePlayerJoins`, `TestMR024_MasterRejectsAndThePlayerStaysOut`.
  - Quinto: `TestMR024_OnlyTheMasterApprovesOrRejects` e as colunas do membro pendente em `TestAuthorizationMatrix`.
  - Sexto: `TestQ24_MasterSeesAndRemovesPendingMemberWithoutCharacter`, `TestQ24_OnlyTheCampaignsMasterManagesPendingMembers`, `TestQ24_PendingMembershipExpiresAfter30Days`, `TestQ24_CreatingTheCharacterClearsTheDeadline`.
  - Sétimo: `TestQ25_PlainInvitePromotesPendingMember` (em `campaigns` e `characters`), `TestQ25_OnlyAWorkingPlainInvitePromotes`.
  - Da regra: `TestRN15_PendingCharacterIsNotPartOfTheCampaignYet`, `TestRN15_ApproveAndRejectRace`, e no `authz`, `TestRN15_PendingMemberOnlyGetsThroughTheAllowedCalls`, `TestRN15_PendingMemberLooksLikeAStranger`, `TestPendingMayCallIsTheAgreedList`.
  - Playwright `e2e/tests/character-approval.spec.ts` (`@MR-024`: o personagem criado por convite com aprovação fica pendente até o mestre aprovar; o mestre recusa e o jogador não entra; o mestre vê quem entrou e ainda não criou o personagem, e o remove) e o axe.

#### Relacionadas
- Estende [MR-003](#mr-003-entrar-pelo-convite): o personagem nasce pendente de aprovação (ver [Ciclo de vida da ficha](regras.md#ciclo-de-vida-da-ficha), RN-01).
- Estende [MR-002](#mr-002-gerar-convite): o mestre escolhe, em cada convite, se ele exige aprovação.

### MR-028: Mostrar uma imagem aos jogadores

**Como** mestre, **quero** mostrar aos jogadores uma imagem da galeria durante a sessão, com uma ação própria, separada do mapa atual, **para** apresentar um retrato, uma carta ou uma cena.

- Prioridade: MVP
- Regras: RN-10
- Módulos: play, maps

#### Critérios de aceite
- **Dado** uma sessão aberta, **quando** o mestre mostra uma imagem da galeria, **então** quem está na sessão a vê na hora, sem recarregar, **e** o mapa atual continua lá; **quando** o mestre para de mostrar, a imagem some da tela dos jogadores.
- **Dado** que a sessão acabou ou o mestre parou de mostrar, **então** os jogadores não recebem mais o ID da imagem.
- **Dado** uma imagem que o mestre quer que os jogadores continuem vendo, **quando** ele liga "Deixar com os jogadores" e depois para de mostrar ou troca a imagem, **então** ela continua com os jogadores, numa lista "Imagens que o mestre deixou" na página da sessão deles, até o mestre tirá-la ("Tirar"); a lista é da campanha e continua depois que a sessão acaba.

#### No app
- `PlayService.SetShownImage` (só o mestre, só com a sessão aberta, só uma imagem da galeria da campanha) guarda a imagem em `game_sessions.shown_image_id`. `GetLiveSession` a devolve (`shown_image`, com o nome como legenda) e o stream leva `shown_image_changed` a todos. Uma imagem por vez, independente do mapa atual. Mostrar o fundo de um mapa com névoa mostra uma cópia dele ("… (névoa)"), cujo ID é o devolvido; com a galeria cheia a chamada dá `resource_exhausted`. Apagar a imagem da galeria para de mostrá-la. Uma sessão nova começa sem imagem. Ver [Arquitetura](../../architecture.md#what-the-session-shows).
- O jogador baixa a imagem (`GET /images/{id}`) só enquanto ela é mostrada, enquanto está deixada com os jogadores ou enquanto é o fundo de um mapa que ele vê; fora disso, `404`, mesmo com o ID guardado, e o navegador dele pergunta de novo a cada uso (`Cache-Control: private, no-cache`). Ver [RN-10](regras.md) e [Servir as imagens](../../architecture.md#serving-images).
- "Deixar com os jogadores": `SetShownImage` tem `keep` (guardado em `game_sessions.shown_image_keep`; desligado ao mostrar uma imagem nova). Ligado, parar de mostrar, trocar a imagem ou encerrar a sessão passa a imagem para a lista da campanha (`campaign_left_images`), na mesma transação. `ListLeftImages` (qualquer membro ativo, com ou sem sessão) lê a lista, `TakeBackLeftImage` (só o mestre) tira uma imagem dela, e o stream manda `left_images_changed`, uma dica sem conteúdo, a todos. Apagar a imagem da galeria a tira da lista.
- Tela do mestre: o painel "Imagem para os jogadores" ("Mostrar imagem", o seletor da galeria com "Mostrar aos jogadores", "Trocar imagem", "Parar de mostrar"), o interruptor "Deixar com os jogadores" ("Ligado"/"Desligado") e a lista "Deixadas com os jogadores", com "Tirar".
- Tela do jogador: o bloco "O mestre está mostrando", com a imagem, o nome como legenda e "Ver em tela cheia", que aparece e some ao vivo e é anunciado; e a lista "Imagens que o mestre deixou", com "Ver em tela cheia", escondida enquanto vazia e sem aviso de leitor de tela quando muda. A lista aparece só na página da sessão, não na da campanha. Ver [Design](../../design.md#maps-and-the-shown-image).
- O nome da imagem vem do nome do arquivo enviado e aparece para os jogadores como legenda; o mestre pode renomear antes de mostrar ("covil-secreto-do-lich" entregaria um segredo).
- Fora do escopo: guardar automaticamente todas as imagens já mostradas num "baú" de handouts por jogador seria uma história nova; a lista só tem o que o mestre escolheu deixar, e é a mesma para todos os jogadores da campanha.
- Testes: `TestMR028_MasterShowsAnImageToThePlayers` (mostrar, parar, apagar, e as recusas: jogador, imagem de outra campanha, sem sessão aberta), `TestMR028_MasterLeavesAnImageWithThePlayers`, `TestRN10_PlayersOnlyFetchImagesTheyCanSee`, e as linhas de `SetShownImage`, `ListLeftImages` e `TakeBackLeftImage` nas matrizes de autorização do `play`; Playwright `shown-image.spec.ts` (`@MR-028`).

### MR-025: Cadastrar conteúdo da mesa

**Como** mestre, **quero** cadastrar raças, classes, subclasses, antecedentes, magias e regras que não vêm no SRD 5.1, e definir as regras da minha mesa, **para** a campanha usar o material que a mesa joga e eu ter total controle do produto.

- Prioridade: MVP
- Regras: RN-23, RN-24, RN-25
- Módulos: rules, characters, campaigns, play, maps

A história cobre classes **e** subclasses próprias, raças e sub-raças, antecedentes, magias, regras da casa e de dados e a grade da campanha. A proposta do jogador (MR-026) e a leitura de um PDF (MR-027) são histórias à parte, para depois.

#### Critérios de aceite
- **Dado** que sou mestre de "Mirathel", **quando** cadastro uma classe nova com o dado de vida, as perícias, a tabela dos 20 níveis e as características dela, **então** a classe aparece no editor de personagem e na subida de nível só em "Mirathel" **e** a ficha calcula os números com ela (RN-23).
- **Dado** que sou mestre de "Mirathel", **quando** cadastro uma subclasse de uma classe do SRD (um colégio do Bardo, uma tradição do Mago), **então** ela aparece na escolha de subclasse daquela classe só em "Mirathel", com as magias sempre preparadas dela, se tiver.
- **Dado** um conteúdo cadastrado em "Mirathel", **quando** abro outra campanha minha, **então** ele não aparece lá: o conteúdo vale por campanha.
- **Dado** que sou mestre de "Mirathel", **quando** cadastro uma magia própria, **então** ela aparece para os personagens de "Mirathel" que podem aprendê-la **e** não aparece em outra campanha; se ela tem ataque ou teste de resistência e dano, o combate a resolve como uma magia do SRD.
- **Dado** que cadastro uma magia de área (um cone de 4,5 m, uma esfera de 6 m de raio), **quando** a salvo, **então** a área aparece na descrição dela, **e** no combate quem a conjura escolhe as criaturas que ela pega, como numa magia de área do SRD.
- **Dado** um personagem novo que entra no nível do grupo, **quando** o jogador o cria com mais de uma classe (Rafa cria a Corvina, Maga 3 e Clériga 1, no nível 4), **então** o editor tem um bloco por classe, com o nível e a subclasse de cada uma (a da mesa inclusa), **e** o servidor confere os pré-requisitos de multiclasse, como na subida de nível.
- **Dado** uma classe da mesa que uma ficha usa, **quando** o mestre muda um número dela, **então** a ficha mostra o número novo, também travada, **e** o que ficou fora das regras aparece nela como aviso ("A classe mudou"), sem impedir outra edição (RN-23).
- **Dado** um conteúdo da mesa que uma ficha usa, **quando** o mestre o arquiva, **então** a ficha continua com ele **e** ele não aparece mais como escolha nova; nada do conteúdo da mesa se apaga.
- **Dado** uma característica que o mestre cadastra, **quando** ele escolhe o efeito, **então** escolhe de um menu fechado (modificador, proficiência, recurso, sentido, modo de rolagem, ação, ataque extra, escolha, nota, magia concedida), ou deixa só o texto: o conteúdo da mesa nunca roda código.
- **Dado** que sou mestre de "Mirathel", **quando** escolho na página "Regras da mesa" como se ganham PV, como se fazem as habilidades, o crítico e quem vê os testes contra a morte, **então** o servidor segue a escolha em cada ficha e em cada combate (RN-24); o conjunto padrão, a compra por pontos e os 4d6 aparecem com o rótulo "SRD 5.2.1 (regras de 2024)".
- **Dado** uma campanha que permite 4d6 descartando o menor, **quando** o jogador rola as habilidades de uma ficha nova, **então** o servidor rola e guarda, e rolar de novo devolve o mesmo.
- **Dado** um mapa desenhado com quadrados de 3 m, **quando** o mestre diz "cada quadrado deste desenho vale 3 m", **então** o app conta quatro quadrados de 1,5 m em cada um, e o movimento, o alcance e a névoa seguem as regras de sempre (RN-25).
- **Dado** um combate que o mestre começa sem mapa ("teatro da mente"), **quando** o jogador se move, **então** ele gasta o movimento por número ("Restam 6 m"), sem posição na grade, **e** o mestre julga o alcance (RN-25, ADR-0017).
- **Dado** a tela "Opções para os jogadores", **quando** o mestre desliga uma classe, subclasse, raça, sub-raça, antecedente ou magia (do SRD ou da mesa), **então** os jogadores não a veem nem a escolhem mais; o que está ligado eles veem por inteiro, com os números e os efeitos.
- **Dado** uma entrada da mesa arquivada, **quando** o mestre a desarquiva, **então** ela volta a aparecer como escolha nova.
- **Dado** um personagem com antecedente de texto livre (o "Outro" do editor), **quando** o jogador o preenche, **então** escolhe, como na regra do SRD 5.1 "Personalizar um antecedente", duas perícias, duas ferramentas ou idiomas no total, uma característica (com o texto dele) e o equipamento, **e** a ficha calcula com isso.

#### No app
**O motor.** `rules.Content.With(Overlay)` soma ao SRD as classes (com a tabela dos 20 níveis e os quatro tipos de conjuração), as subclasses (inclusive o conjurador de um terço e as magias sempre preparadas), as raças e sub-raças, os antecedentes e as magias (com o alvo e a área) da mesa, sem mudar o SRD, e a ficha calcula com elas. O conteúdo é guardado por campanha e servido pelo `TableContentService` (ver [Arquitetura](../../architecture.md#live-table-content)).
- Testes (`rules`): `TestWithAddsTheTableContent`, `TestWithDoesNotChangeTheBase`, `TestWithConcurrently`, `TestWithRefusals` (uma linha por regra: chave, `handler`, escolha fora do SRD, referência solta, limites, fórmula, tabela sem 20 linhas), `TestLevelUpSweepTable`, `TestLevelUpSweepTableMulticlass` (as classes da mesa de 1 a 20), `TestDeriveTableCharacterGolden`, `TestSpellDetailsOfTableSpells`, `TestThirdCasterSlots`, `TestAlwaysPreparedDoNotCount`, `TestArchivedEntriesStillResolve`, `TestMissingTableKeysNeverPanic`.

**Guardar e servir.** O mestre cria, edita, arquiva e desarquiva as entradas dos seis tipos.
- O servidor faz a chave do nome e as chaves das características (que não mudam enquanto a característica existe). Cada escrita confere a campanha inteira com o mesmo código do SRD e devolve uma violação por campo, no **caminho exato** (`table_spell.range.distance_ft`, `table_spell.damage[1].dice`, `table_class.levels[4].slots[2]`...), tudo de uma vez.
- O conteúdo vale na hora, também nas fichas travadas. A ficha que ficou fora das regras mostra "A classe mudou" (só com um aviso novo que depende da entrada, com as frases do que não combina mais; some sozinho quando o aviso é corrigido, e salvar a ficha não o limpa) e nunca recusa outra edição.
- Uma entrada arquivada não é escolha nova: criar, editar e subir de nível a recusam, e a ficha que já a usa continua. Uma entrada arquivada ainda se edita. Uma magia nunca passa de truque a magia de nível.
- Os jogadores leem as entradas jogáveis por inteiro e nunca as arquivadas (nem na subida de nível nem nos detalhes da magia, a não ser que a ficha deles a use); nenhuma chave arquivada chega a um jogador, nem por referência. `ListContent` nomeia, para todo membro, os idiomas, as proficiências e os tipos de dano (o jogador nunca lê `language:common`), recebe um `character_id` para a ficha de um jogador manter o que o mestre arquivou ou desligou depois, e as respostas de criar, editar, arquivar e desarquivar trazem `characters_using`.
- Testes (`characters`): `TestTableContentEveryKindIsCreatedUpdatedArchivedAndBroughtBack`, `TestTableContentFeatureKeysAreStable`, `TestTableContentKeysNeverChangeAndNamesAreUnique`, `TestTableContentRefusalsAreFieldViolations`, `TestTableContentRefusalListsEveryFieldAtItsPath`, `TestTableContentWritesCarryHowManySheetsUseTheEntry`, `TestListContentNamesLanguagesProficienciesAndDamageTypes`, `TestTableContentLimits`, `TestTableContentAccess`, `TestTableContentStaleAndRevision`, `TestTableContentIsLive`, `TestTableContentConcurrentWrites`, `TestTableContentChangeShowsOnTheSheet`, `TestTableContentAnUnrelatedEditIsNeverRefusedForAChange`, `TestTableContentArchivedIsNeverANewChoice`, `TestSheetsThatUseTableContentStayInTheirCampaign`; no `rules`, `TestEntryViolationPaths` e `TestAnEntryReportsEveryViolationAtOnce`.

**"Opções para os jogadores" (os interruptores).** `TableContentService.SetOptionSwitches` (só o mestre) liga e desliga, numa transação, até 700 classes, subclasses, raças, sub-raças, antecedentes e magias do SRD e da mesa (`campaign_content_off`; tudo ligado por padrão). `ListOptionSwitches` devolve a lista da tela, com o estado de cada opção (`off`, `archived`, `hidden`, a classe ou raça mãe) e quantas fichas de jogador a usam. Ver [Arquitetura](../../architecture.md#player-options-and-the-live-hint).
- Uma opção desligada é tratada como arquivada para os jogadores em toda leitura (o catálogo, `ListTableEntries`, `ListSpells`, `GetSpellDetails`, as opções de subida de nível e as referências dentro de outras entradas). A classe desligada leva as subclasses, e a raça, as sub-raças.
- Uma escolha nova de uma opção desligada é recusada ao jogador (`SWITCHED_OFF_CONTENT` ao criar e editar; `SWITCHED_OFF_CHOICE` na subida de nível). A ficha que já a tem segue funcionando, uma edição sem relação nunca é recusada, e o mestre não é recusado (qualquer ficha que o mestre edita pode ter a opção). Uma ficha que já tem a classe desligada continua chegando ao nível da subclasse: a subclasse é julgada só pelo interruptor dela (o mesmo vale para a sub-raça sob a raça desligada da própria ficha).
- Cada escrita sobe a revisão do conteúdo, e toda escrita no conteúdo da mesa e todo interruptor mandam a dica `content_changed` (sem conteúdo) à sessão ao vivo.
- Tela `/campaigns/:id/content/options`: um grupo por tipo, com o contador ("Raças: 9 de 10 ligadas"), a busca, "Ligar todas" e "Desligar todas", o interruptor de cada opção com quantas fichas a usam e a nota do pai desligado, tudo salvo na hora; a opção desligada que uma ficha usa diz que a ficha continua funcionando. Os editores de magia, raça, sub-raça, antecedente, classe e subclasse têm o interruptor "Disponível para os jogadores". A lista e a página da entrada marcam a desligada. A lista do conteúdo, as opções, "Magias", o editor de personagem, a subida de nível e a ficha aberta leem de novo o que mostram quando a mesa muda (`content_changed`, só com a sessão aberta).
- Testes: `TestRN23_SwitchedOffOptionsAreNeverSeenByPlayers`, `TestRN23_ANewChoiceOfAnOffOptionIsRefused`, `TestRN23_TheRevisionAndTheCacheFollowTheSwitches`, `TestRN10_ContentChangedTellsTheSession`, `TestMR025_TheSwitchesAreTheMastersAlone`, `TestRN23_ConcurrentSwitchesAreOrdered`; Playwright `content-options.spec.ts` (`@MR-025 @RN-23 @MR-045 @RN-10`) e as varreduras de `a11y.spec.ts`.

**Regras da mesa fora do combate.** Ver [RN-24](regras.md) e [Arquitetura](../../architecture.md#table-rules-mr-025-rn-24).
- Os PV da subida de nível: `TestRN24_TheLevelUpFollowsTheHitPointsRule` (a regra "rolar" recusa a média, a regra "a média" recusa o dado, a padrão aceita os dois, e `GetLevelUpOptions` diz qual vale).
- As habilidades de uma ficha nova: `TestRN24_TheScoresFollowTheMethod` (o conjunto padrão, a compra por pontos, os 4d6 e digitar, cada um aceito e recusado; os NPCs e as edições do mestre ficam livres), `TestRN24_ATableCanSwitchMethodsOff`; no `rules`, `TestCheckStandardArray`, `TestPointBuy`, `TestCheckTyped`, `TestAbilityRolls`.
- Os 4d6 do servidor: `TestRN24_TheServerKeepsTheFourD6` (os mesmos jogos numa segunda chamada, gastos por uma ficha, novos para a ficha seguinte; uma ficha recusada não os gasta), `TestRN24_PhysicalDiceAreTypedOnce` (o jogador digita os dados uma vez, e a regra de dado da campanha vale), `TestRN24_ThePendingMemberMakesTheirScoresToo`.
- O estilo e as regras guardadas: `TestRN24_TableRulesRoundTrip`, `TestRN24_TheStyleIsWorkedOutNotStored` (um estilo preenche, e uma edição à mão dá "Personalizado"), `TestRN24_OnlyTheMasterWritesTheRules`, `TestRN24_TheRulesRefuseWhatBreaksTheLimits` (20 lembretes de 200 caracteres), `TestRN24_NewMapsGetTheFogTheTableChoseWithTheirFirstGrid`.
- O modo de XP: `TestRN09_TheXPModeChangesAfterCreation`, `TestRN09_ChangingTheXPModeAsksOnceXPWasAwarded` (muda na hora sem XP dado; senão pede `confirm` com o número de prêmios e de XP; nada é convertido).
- `TestRN24_TheTableRulesAreReadInTheCallersTransaction`: a leitura das regras dentro de uma escrita passa pelo pool de uma conexão.
- **A página "Regras da mesa"** (`/campaigns/:id/rules`, do painel "Regras da mesa" da campanha; o mestre edita e o jogador a lê só para leitura): o estilo da mesa no alto, que preenche os dados, o combate e a névoa e deixa cada um editável; os PV; os jeitos de habilidade (pelo menos um, com o rótulo "SRD 5.2.1 (regras de 2024)" nos três do SRD 5.2.1); o crítico; os testes contra a morte; os lembretes (até 20, de 1 a 200 caracteres, uma linha cada, sem chave de ligar: apagar o lembrete é desligá-lo); o modo de XP com a pergunta no lugar; o link para a grade dos mapas e para o conteúdo da mesa. O modo de dados é editado só aqui: o painel "Dados" da campanha só o diz e leva para cá. "Combate com mapa" é o padrão guardado para os combates novos. Ver [Design](../../design.md#table-rules-ability-scores-and-grid-calibration-mr-025).
- **O passo "Habilidades" do editor**, para o jogador que cria a ficha, nos jeitos que a mesa deixa (conjunto padrão, compra por pontos com "Restam N pontos", 4d6 do servidor, digitar de 3 a 18), com o jeito mandado junto no `CreateCharacter` e a recusa do servidor dita por motivo. O app não mostra o modificador nem o bônus da raça nesse passo: o servidor calcula na ficha. O subir de nível segue a regra de PV da mesa: sem a escolha quando a regra fixa o jeito.
- **A calibração da grade** no painel "Grade" do editor do mapa ("Calibrar o quadrado": 1,5 m, 3 m, 4,5 m, 6 m ou outro múltiplo de 1,5 m até 30 m), com "Mudar a grade?" só quando a mudança apaga o que foi pintado. A página "Créditos" traz a atribuição do SRD 5.2.1, igual à do `NOTICE`.
- Testes: Playwright `e2e/tests/table-rules.spec.ts` (`@RN-24`, `@RN-09`, `@RN-25`) e o Vitest de cada peça.

**Regras da mesa dentro do combate.** O servidor aplica o crítico ("dados dobrados" ou "máximo mais uma rolagem", em todo acerto crítico: arma, magia, criatura e NPC, com mapa e no teatro) e esconde os testes contra a morte de quem não é o dono nem o mestre (a palavra "Caído" no combatente, no registro e no stream; "Estável" e "Morto" continuam de todos). O dano pendente traz a regra (`critical_rule`) para a tela mostrar a dica do dado. Ver [RN-24](regras.md) e [Arquitetura](../../architecture.md#table-rules-in-combat).
- **O combate sem grade (teatro da mente).** O modo é do combate (`encounters.mode`, `grid` ou `theatre`), escolhido ao começar e nunca mudado, e vem de `StartEncounter(mode)` ou, sem ele, da regra da mesa "combate com mapa" (RN-24). Testes: `TestRN25_ACombatStartsInOneModeForGood`, `TestRN25_ReadsAndTheStreamCarryTheMode` (a leitura e a dica `encounter_changed` dizem o modo).
  - O movimento por número: `TestRN25_SpendMovementByNumber` ("Restam 3,0 m", nunca mais que o turno, a Disparada dobra, fora da vez recusa, o mestre gasta por um NPC, o desfazer repõe, o NPC escondido não existe para o jogador).
  - O ataque de oportunidade que o mestre oferece: `TestRN25_OpportunityAttacksAreOfferedByTheMaster` (a pergunta chega ao jogador certo e a mais ninguém, uma reação por rodada, recusa, retirar a oferta, o que não dá para oferecer).
  - Ataques, magias e ações sem alcance: `TestRN25_AttacksAndCoverWithoutReach` (todo alvo que as regras deixam, a cobertura de quatro graus, o NPC escondido fora do JSON do jogador), `TestRN25_SpellsAndActionsWithoutReach` (uma magia com um alvo, com vários, de área, de toque; Ajudar, Disparada, Esquivar, Desengajar).
  - O que não existe sem mapa: `TestRN25_WhatNeedsAMapIsRefused` (mover, saltar e pôr no mapa recusados com motivo; o "onde posso ir" vazio e válido; `SpendMovement` e a oferta recusados no combate com mapa), `TestRN25_ATrapDoesNotExistInACombatWithoutAMap`, `TestRN25_SummonedCreaturesAndWildShapeWithoutASquare` (criaturas convocadas sem quadrado, a vez de uma criatura, a Forma Selvagem, e o olho do familiar recusado com `FAMILIAR_SIGHT_BLOCKED` / `NO_MAP`).
  - Condições sem velocidade: `TestRN25_ConditionsThatLeaveNoSpeed` (agarrado e impedido têm velocidade 0; paralisado, petrificado, atordoado e inconsciente não andam; no mapa e sem mapa) e `TestRN25_AGridOfferIsNotWithdrawn`.
  - O NPC escondido (RN-10): `TestRN10_AHiddenNPCIsNeverNamedInATheatreCombat`. O resto é igual: `TestRN25_DeathSavesUndoAndTheEnd`; o combate no mapa é o de antes.
- **As telas.** A tela só desenha o que o servidor manda: ramifica no `Encounter.mode`, nunca faz conta de dado, de alcance nem de regra. Ver [Arquitetura](../../architecture.md#combat-without-a-grid-theatre-of-the-mind) e [Design](../../design.md#gridless-combat-gastar-movimento-the-offer-and-hidden-saves-mr-025).
  - "Iniciar combate" pergunta "Como este combate é jogado": "Com mapa" ou "Sem mapa (teatro da mente)", com a regra da mesa "combate com mapa" como padrão (e "Sem mapa" quando a sessão não tem mapa atual). Enquanto a página da sessão ainda lê o mapa atual, "Iniciar combate" espera, com "Lendo o mapa atual…": o diálogo nunca abre como se não houvesse mapa. Se essa leitura falha, o botão fica desligado com "Não deu para ler o mapa atual" e "Tentar de novo": um combate iniciado assim seria no teatro da mente, que não muda depois. Em "Sem mapa" o diálogo não mostra nem pede mapa nem grade, e diz uma vez só o porquê ("Sem mapa, o app não sabe onde ninguém está."); o modo vale até o fim do combate.
  - A tela do mestre sem mapa: a faixa do combate ganha a etiqueta "Teatro da mente"; no lugar do mapa, o cartão "Ações do ..." (de um NPC e, nesse modo, também de um jogador, para o mestre gastar o movimento de quem saiu), com "Gastar movimento" (um número, de 1,5 m em 1,5 m, nunca mais que o que resta), a ordem, o registro ("Toren gastou 6,0 m de movimento") e "Cobertura dos alvos" (sem cobertura, meia +2, três quartos +5 e total, que impede mirar). Do cartão de um NPC, "Oferecer ataque de oportunidade" (quem andou é o da vez; o mestre escolhe de quem ele saiu do alcance) abre a pergunta no celular do jogador, e o mestre vê a espera com "Seguir sem esperar" e "Retirar a oferta". Seguir sem esperar nunca tira a reação de ninguém. Nenhum aviso de "Sem quadrado no mapa", nenhuma névoa, armadilha ou porta.
  - A tela do jogador sem mapa: o painel "Combate sem mapa" no lugar do mapa; no lugar de "Mover", "Gastar movimento" abre uma folha com o número, "Depois restam 3,0 m" (a prévia do navegador, uma subtração; o número de verdade é o do servidor, `movement_left_dft`) e o aviso de que o app não confere o caminho nem o alcance; a lista de alvos é a de quem o personagem vê e as regras deixam atacar (o servidor decide quem entra), sem distância e sem "Longe demais" ("O mestre decide quem está ao alcance"); a pergunta do ataque de oportunidade é a de sempre, sem quadrado.
  - O crítico: com dado físico, a etapa do dano diz o que rolar pela regra da mesa ("role os dados duas vezes" ou "o máximo mais uma rolagem", com o máximo como parte fixa que o app soma sozinho, e o total mostrado antes de confirmar é o que o servidor grava), no ataque de arma, de magia e no cartão do mestre, com e sem mapa.
  - Os testes contra a morte escondidos: o outro jogador lê só "Caído" (e "Estável" ou "Morto"), sem marcas nem contagens, e o registro diz "Esta mesa só deixa o dono e o mestre verem os testes contra a morte"; o dono e o mestre veem as marcas, com a etiqueta "Só você e o mestre" (no cartão do dono) e "Dono e mestre" (na ordem do mestre); a linha do registro que traz só "estabilizou" (sem d20 nem contagem) diz "Brisa estabilizou".
  - Testes: Playwright `e2e/tests/theatre.spec.ts` (`@MR-025`, `@RN-24`, `@RN-25`, `@RN-20`, `@RN-18`), as telas no `a11y.spec.ts` e o Vitest de cada peça (`core/combat/theatre.spec.ts`, `critical.spec.ts`, `pages/live-session/combat/theatre/`, `start-combat/`, `death-saves/hidden-death-saves.spec.ts`).

**Magias e o antecedente "Outro".**
- O alvo de toda magia vem do servidor (`SpellDetails.target`): na da mesa, o que o mestre escreveu; na do SRD, a área estruturada do 5e-database (`area_of_effect`, 88 das 319 magias) e, só sem ela, o texto. O texto sai pronto, em metros ("Cone de 4,5 m"). O alcance Pessoal e Toque valem na magia da mesa; Pessoal com "uma criatura" ou "várias" é recusado. O arquivo `effects/spell_targets.json` corrige o alvo de umas 40 magias do SRD. Testes: `TestSpellAreas` (o importador), `TestSRDSpellTargets`, `TestEverySRDSpellHasATarget`, `TestStructuredAreasAgainstTheText`, `TestSpellTargetOverrides`, `TestSpellTargetOverridesAreChecked`, `TestSpellTargetMaxTargets`, `TestMetersPT`, `TestSpellTargetLabels`, `TestTableSpellTargetsAreTheMasters`, `TestTableSpellRangeAndTargetMustAgree` (`rules`), `TestMR025_TheSpellDetailsSayWhomItReaches` (`characters`).
- A magia da mesa em combate passa pelo mesmo `CombatSpell`, com o conteúdo da campanha: o ataque, a resistência com metade, a cura, a área contra três alvos, "várias criaturas" (uma a mais por nível), o truque por degrau, a concentração, os espaços e o registro, com o nome em português. Testes (`play`): `TestMR025_ATableSpellAttackInCombat`, `TestMR025_ATableAreaSpellWithASaveAgainstThreeTargets`, `TestMR025_ATableHealingSpell`, `TestMR025_SeveralCreaturesConcentrationAndOnlyTheCaster`, `TestMR025_ATableCantripGrowsByTheCharactersLevel`.
- O antecedente "Outro" segue a regra do SRD 5.1: duas perícias, duas ferramentas ou idiomas em qualquer mistura, a característica (nome e texto do jogador) e o equipamento. O `Validate` confere os limites, o que falta é um aviso (nunca um erro), e o `Derive` aplica as ferramentas, os idiomas e a característica, e mostra o equipamento. Testes: `TestCustomBackgroundDerive`, `TestCustomBackgroundIssues`, `TestCustomBackgroundValidate`, `TestCustomBackgroundIsLockedInTheLevelUp` (`rules`), `TestMR025_TheOutroBackground` (`characters`). As ferramentas e os idiomas do "Outro" vêm do catálogo (`NamedKey.kind`).
- Raças, sub-raças e antecedentes da mesa carregam tudo o que o editor mostra (tamanho, deslocamento, visão no escuro, o "+2 e +1 à escolha", idiomas, traços; perícias, ferramentas, idiomas, equipamento e a característica como nota). O equipamento de um antecedente da mesa chega à ficha (`background_equipment_pt`).

**Classes e subclasses da mesa, de ponta a ponta.** Ver [RN-23](regras.md) e [Arquitetura](../../architecture.md#table-classes-and-subclasses-end-to-end).
- Os números e o menu vêm do servidor: `TableContentService.GetClassTableDefaults` (o bônus de proficiência, os níveis de incremento no valor de habilidade e uma tabela de 20 linhas para cada jeito de conjurar, do SRD) e `GetEffectMenu` (os tipos, os campos, as listas fechadas com nome em português, as opções do SRD, as funções de fórmula e os limites). Testes: `TestTableDefaultsFollowTheSRDTables`, `TestTableDefaultsMakeValidClasses`, `TestEffectMenuIsWhatTheValidatorAccepts`, `TestEffectMenuRequiredFieldsAreRequired`, `TestEffectMenuFormulaHelpers` (`rules`), `TestTableClassEditorStartsFromTheServer` (`characters`).
- Cada recusa aponta o campo: `TestClassRefusalsNameTheirField` (`rules`), `TestTableClassRefusalsPointAtTheirField` (`characters`).
- A multiclasse na criação: `TestTableClassMulticlassAtCreation` (a Corvina, Maga 3 e Clériga 1 com as duas subclasses da mesa e as magias sempre preparadas do domínio; Guardião 2 e Mago 2; Guerreiro 3 com Mago 2, com os espaços da multiclasse; o pré-requisito e a subclasse cedo viram aviso) e `TestTableClassMulticlassHitPointsFollowTheRule`.
- Criar, subir de nível e jogar: `TestTableClassCreationAndLevelUp` (Ícaro, do nível 1 ao 5), `TestTableClassThirdCasterCreationAndLevelUp`, `TestTableClassSubclassPickedAtLevelUpGivesItsSpells` (`characters`), `TestMR025_AThirdCasterOfTheTableCastsATableSpell` (`play`, a "Lâmina de Nanquim").
- "A classe mudou" para as mudanças comuns: `TestChangedClassSentences` (`rules`), `TestTableClassChangeIsTold`, `TestTableClassThirdCasterChangeIsTold` (`characters`): a frase de cada uma, com o aviso ligado à entrada, que some sozinho quando os números voltam a combinar.

**As telas do conteúdo da mesa.** Ver [Design](../../design.md#table-content-the-list-the-editors-and-the-effect-picker-mr-025) e [Design: classe e subclasse](../../design.md#class-and-subclass-mr-025).
- **A página "Conteúdo da mesa"** (`/campaigns/:id/content`, do painel "Conteúdo da mesa" da campanha): o mestre num notebook vê o menu dos cinco tipos com a contagem (arquivadas inclusive), "7 de 300 entradas", a busca, "Mostrar" (Todas, Em uso, Sem fichas, Arquivadas) e as linhas com o estado em palavras ("Em uso por 2 fichas", "Arquivado · 1 ficha usa"); a campanha sem nada diz "Nada cadastrado ainda." e que o SRD continua valendo. No celular o mestre só lê e arquiva (a pergunta é uma folha de baixo). O jogador lê todas as entradas ligadas, tipo por tipo, com "Da mesa", sem contagem, sem estado e sem nenhum botão.
- **Os editores** (uma página, um "Salvar"): a magia (todos os campos, o "Alvo" antes da mecânica, "Mais criaturas por nível" recolhido até ser pedido, "Pessoal" e "Toque" no alcance, a mecânica opcional, as classes que a aprendem e a prévia "Como os jogadores veem", escrita do formulário); a raça e a sub-raça (os seis bônus, "+2 e +1 à escolha", idiomas, traços com efeito do menu do servidor, as sub-raças da raça); o antecedente (duas perícias, ferramentas, idiomas à escolha, equipamento e a característica). O app só manda os campos do tipo de efeito escolhido.
- **O editor da classe:** o nome, o dado de vida, "Teste de resistência 1" e "2", quantas perícias o jogador escolhe e de quais, as proficiências, o pré-requisito de multiclasse, a conjuração (nenhuma, completa, metade ou pacto; preparadas ou conhecidas; a lista) e o nível da subclasse. A tabela dos 20 níveis começa nos números que o servidor manda (`GetClassTableDefaults`) e o mestre edita célula por célula; trocar a conjuração pergunta antes de refazer uma tabela editada. As características têm nível e efeito do menu do servidor, com o contador "N de 60".
- **O editor da subclasse** (de uma classe do SRD ou da mesa): características por nível; "Esta subclasse conjura" traz a tabela de um terço a partir do nível em que começa; as magias sempre preparadas, por nível da classe.
- **As recusas e os conflitos:** cada violação (`field` e `reason`) volta no campo que o caminho nomeia, com o motivo em português (o app nunca lê a `message`), inclusive na célula da grade e na característica fechada, que se abre. O resumo no alto diz quantos campos precisam de ajuste e o que o mestre digitou continua lá; uma violação sem campo, ou sobre outra entrada, fica no alto. "Esta entrada mudou enquanto você editava" oferece "Recarregar". Depois de salvar, "N fichas ficaram com aviso" (`AffectedCharacter`) lista os personagens que ficaram com aviso ("1 ficha ficou com aviso"). No celular o mestre só lê e arquiva. O jogador lê a classe por inteiro (a tabela, "Testes de resistência"), sem contagem e sem uma arquivada.
- **O editor de personagem e a subida de nível com o conteúdo da mesa.** As classes, subclasses, raças, sub-raças e antecedentes da mesa aparecem entre os do SRD com a etiqueta "Da mesa". A primeira classe diz quantas perícias escolhe e em quais resistências dá proficiência; a raça com bônus à escolha diz onde pô-los; o "Outro" tem o nome, as duas perícias, as duas ferramentas ou idiomas, a característica e o equipamento. "Adicionar classe" faz um bloco por classe (classe, nível e subclasse, a da mesa inclusa), com o nível total só para ler e os pré-requisitos, os pontos de vida e os espaços como avisos e números do servidor (nunca contas da tela). O passo "Magias" tem uma seção por classe (e por subclasse que conjura, a de um terço), pela lista da classe e pelo nível da magia nela, mostra as sempre preparadas (num editar, lidas da ficha calculada) e apaga uma magia fora das listas, com o motivo e o link "Ver em Magias". Ao criar, o editor não mostra "preparadas 0 de 6" por classe (ao editar, o `prepared_max` da ficha salva). Ver [Design](../../design.md#the-character-editor-and-level-up-with-table-content-mr-025-mr-040).
- **Subir de nível:** o resumo mostra só as linhas que mudam, os espaços da tabela de uma classe da mesa com "Da mesa" e "Novas características"; a subclasse que conjura (um terço) faz o passo "Magias" aparecer, com a lista da classe de que ela conjura e, se prepara, o máximo dela. As magias sempre preparadas de uma subclasse vêm no catálogo (`Subclass.always_prepared`), então aparecem ao criar.
- **"A classe mudou":** o aviso na ficha (do dono e do mestre) e a folha "O que mudou", com as frases do servidor como vêm.
- Testes: Playwright `e2e/tests/content.spec.ts` (`@MR-025`, `@RN-23`, `@RN-10`: as duas magias, a recusa no campo, a raça Corujeiro e o personagem do jogador, arquivar e o que o jogador nunca recebe), `e2e/tests/classes.spec.ts` (`@MR-025`, `@RN-23`), `e2e/tests/table-sheet.spec.ts` (`@MR-025 @RN-23 @MR-040`: Davi cria o Ícaro; Rafa cria a Corvina com as duas subclasses e as magias de cada lista; o Ícaro sobe do nível 1 para o 2; a subclasse que conjura ganha as magias do Mago no nível 3; o mestre muda as perícias da classe e a ficha diz "Guardião do Vale agora dá 2 perícias no nível 1; esta ficha tem 3." até corrigir); as telas no `a11y.spec.ts` (`scanTableSheetScreens`, claro e escuro, de 320 a 1280 px); e o Vitest de cada peça (`core/content`, `pages/content`, `shared/` `effect-picker`, `feature-editor`, `form-fields`, `option-switches`, `ContentWatcher`, `class-draft`, `class-editor`, `subclass-editor`, `content-violations`, `content-read`, `class-blocks`, `character-editor-classes`, `levelup-classes`, `levelup-summary`, `changed-content`).

**Fora do escopo:** copiar o conteúdo para outra campanha (MR-026), monstros e itens mágicos próprios, talentos fora do SRD, o flanqueamento e a grade hexagonal.

#### Relacionadas
- [RN-23](regras.md), [RN-24](regras.md), [RN-25](regras.md); ADR-0017 (combate sem grade), ADR-0018 (conteúdo da mesa).
- [MR-040](#mr-040-subir-de-nível-pela-ficha) (subida de nível), [MR-045](#mr-045-consultar-as-magias).

### MR-010: Gerar masmorras

**Como** mestre, **quero** gerar uma masmorra (salas, corredores, portas e escadas, com opções de tamanho e estilo) e ter um mapa que eu possa editar, **para** não desenhar tudo na mão.

- Prioridade: MVP
- Regras: RN-10, RN-26
- Módulos: rules, maps

#### Critérios de aceite
- **Dado** que sou mestre de "Mirathel", **quando** peço uma masmorra com um tamanho (de 21 a 121 quadrados de lado, ou o que eu digitar), o formato (sem forma, anel, cruz, em L, elipse, losango), o tamanho das salas, o jeito dos corredores (labirinto, sinuosos ou retos), a quantidade e o tipo das portas e as escadas, **então** o app gera um mapa com salas, corredores, portas e escadas, conectado: dá para chegar a todas as salas.
- **Dado** o mesmo tamanho, as mesmas opções e a mesma semente, **quando** gero duas vezes, **então** o resultado é o mesmo.
- **Dado** uma masmorra gerada, **quando** ela vira um mapa, **então** é um mapa da campanha como os outros, escondido dos jogadores (RN-10) e com a névoa de guerra ligada, com a luz base clara, que o mestre pode trocar por penumbra ou escuro (assim, mesmo com a imagem com textura, a sala atrás de uma porta secreta só aparece para quem a vê), com a grade, as paredes e as portas já pintadas nas camadas, as escadas como pontos de submapa e a lista das salas ao lado do mapa, só para o mestre, com "Pôr uma cena nesta sala".
- **Dado** uma masmorra gerada, **quando** eu a edito no editor do mapa (paredes, portas, terreno, pontos), **então** "Redesenhar" faz a imagem de novo a partir das paredes e das portas de agora, sem apagar o que eu pintei.
- **Dado** uma porta fechada, **quando** um personagem anda até ela, **então** ela se abre, se não estiver trancada; uma porta secreta é parede para os jogadores até eu revelá-la (RN-26).
- **Dado** a imagem gerada, **quando** os jogadores a veem, **então** ela tem só o chão e as paredes: as portas são desenhadas por cima, pela camada, e nenhum número de sala aparece.

#### No app
- **O gerador** (`rules/dungeon`) é puro e determinístico, feito em sala limpa (ADR-0015): o gerador do donjon é CC BY-NC, incompatível com a Apache 2.0 do MeuRPG, e o código e os dados dele nunca são copiados; uma especificação escrita com as nossas palavras foi conferida e implementada por outro agente. Ver [O gerador de masmorras](../../architecture.md#dungeon-generator-mr-010).
- **A camada das portas.** Seis estados por quadrado; o movimento abre a porta fechada e para na trancada; o que o jogador sabe e nunca sabe: ver [RN-26](regras.md) e [As portas](../../architecture.md#doors). Testes: `TestRN10_PlayersNeverSeeASecretDoor`, `TestRN10_FogFiltersTheDoors`, `TestMR010_AMoveOpensAClosedDoor`, `TestMR010_ALockedDoorStopsTheMove`.
- **Da masmorra ao mapa.** O `DungeonService` faz a prévia (`PreviewDungeon`, a mesma semente dá a mesma planta), cria o mapa (`CreateDungeonMap`: escondido, grade igual à largura, névoa ligada com a luz base clara, as paredes e as portas nas camadas casando quadrado a quadrado com o gerador, as escadas como pontos de submapa sem destino, a imagem só de pisos e paredes na galeria e o registro `generated_dungeons`), lista as salas só para o mestre (`GetDungeonRooms`), põe a cena de RP na sala (`PlaceDungeonScene`, "Sala N") e redesenha a imagem do mesmo tamanho a partir das camadas de agora sem apagar nada (`RedrawDungeonMap`). Ver [O mapa de uma masmorra gerada](../../architecture.md#generated-dungeon-maps). Testes: `TestMR010_PreviewIsDeterministicAndStoresNothing`, `TestMR010_OptionsOutOfRangeAreRefusedByName`, `TestMR010_ADungeonBecomesAMap`, `TestRN10_PlayersReadNothingOfADungeon`, `TestMR010_APlacedSceneIsAnRPSceneInTheRoom`, `TestMR010_RedrawShowsTheMastersEditsAndKeepsTheLayers`, `TestMR010_RedrawNeverChangesTheSize`, `TestMR010_AFailedCreationLeavesNothingBehind`, `TestMR010_CreatingAndRedrawingAreRateLimited`, e `TestRenderDrawsFloorsAndWalls` (`maps/dungeonimg`).
- **As portas nas telas.**
  - No editor: a ferramenta "Porta" em "Pintar" (tipo, pôr, trocar, "Tirar a porta"), com a recusa da própria tela "uma porta precisa de chão dos dois lados" (o servidor não exige chão ao lado da porta) e a pergunta no lugar antes de pôr uma porta fechada, trancada ou secreta onde há um token (RN-26).
  - No mapa: uma marca por tipo de porta (fechada, aberta, trancada, grade, secreta) no editor, na sessão (a névoa, o jogador e o mestre) e no combate, e a legenda feita das portas que o mapa tem. O cadeado e a porta secreta só aparecem para o mestre, com "(só você vê)" e o olho cruzado, e a tela do jogador ainda descarta uma porta trancada ou secreta que chegasse (RN-10).
  - No combate: quem anda até uma porta fechada a abre e o registro diz "Toren abriu a porta."; uma porta trancada para o movimento antes dela, a página "Mover" fica aberta com "A porta está trancada." e o mapa do jogador continua com "Porta fechada".
  - O mestre na sessão toca numa porta do mapa (com névoa, ou o do combate) e abre, fecha ou tranca na folha da porta (um balão ao lado da porta no computador, sem escurecer o mapa; uma folha de baixo no celular); sem névoa, as portas se mudam no editor; a porta secreta tem "Revelar a porta secreta", que a pinta fechada. Fora do combate o servidor não deixa o jogador mover o token (só o mestre), então a porta se abre pela folha do mestre.
  - Testes: Playwright `e2e/tests/doors.spec.ts` (`@MR-010 @RN-26 @RN-10`) e as telas de portas em `a11y.spec.ts`.
- **"Gerar masmorra"** (rota `campaigns/:id/maps/dungeon`, pelo botão ao lado de "Novo mapa" no painel Mapas; só o mestre, e no celular a página diz "Gerar masmorra é no notebook."): tamanho (Mínima 21, Pequena 31, Média 51, Grande 81, Enorme 121 ou "Outro" de 21 a 121; o outro lado vale dois terços, ímpar), formato, o menor e o maior lado das salas, corredores, portas, becos sem saída, escadas (0 a 4) e a semente com "Outra semente". A prévia é a do servidor (`PreviewDungeon`), pedida um instante depois da última mudança e uma por vez; o navegador só a desenha, com as portas e as escadas pela legenda do mapa e a contagem "13 portas e 8 passagens". Uma opção fora da faixa aparece no campo, com ícone e texto. "Criar o mapa" (`CreateDungeonMap`, com o nome, as opções e a semente da prévia) mostra os passos; o mapa nasce escondido, com a névoa ligada e a luz de base "Clara" (a página diz), e abre no editor. "Parar de esperar" só para de esperar: o servidor termina o mapa mesmo assim, e a tela diz isso.
- **No mapa gerado.** A lista "Salas" (só do mestre: número, tamanho, escadas, saídas com o tipo verdadeiro da porta, "Porta com armadilha: …", a sala atrás de uma porta secreta); "Pôr uma cena nesta sala" (`PlaceDungeonScene`, o ponto aparece no mapa na hora); a sala escolhida com moldura sólida de 3 px. As escadas (pontos de submapa que **nascem reveladas**, e o jogador vê uma só onde vê o quadrado) são desenhadas como selo com seta no mapa, na legenda e na lista ("Escada", nunca "Submapa") para o mestre e para os jogadores, pelo campo `stairs` do ponto. O editor só pergunta pela lista das salas se `Map.generated_dungeon` diz que o mapa é uma masmorra. As paredes da camada cobrem toda a rocha, e a imagem tem um preenchimento liso por baixo da marca de parede do mapa.
- **"Redesenhar"** (só quando o servidor diz que a imagem ainda é a do gerador) pergunta no lugar, manda antes os traços que esperam, e cada recusa tem a sua frase (imagem trocada ou grade mudada, grade tirada, paredes mudadas enquanto desenhava, rajada de pedidos).
- Testes: Playwright `e2e/tests/dungeon.spec.ts` (`@MR-010 @RN-26 @RN-10`: a mesma semente dá a mesma prévia, criar, a cena na sala, pintar e redesenhar, o jogador nunca recebe a lista) e a varredura do gerador em `a11y.spec.ts`.
- O gerador não põe monstros, armadilhas nem tesouros: para isso há o bestiário, os encontros e o tesouro ([MR-042](#mr-042-bestiário) a [MR-044](#mr-044-gerar-tesouro)); as armadilhas são a [MR-035](#mr-035-armadilhas). A [MR-039](#mr-039-imagens-geradas-para-masmorras-e-cenas) usa a masmorra gerada para fazer a imagem.
- Fora do escopo: masmorras de vários andares além das escadas ligadas, e a grade hexagonal. Desenhar a masmorra à mão (paredes falsas, água, baús e mímicos) segue como ideia para o editor do mapa.

### MR-029: Ganchos e pistas da cena

**Como** mestre, **quero** anotar, para cada cena de RP, os ganchos, as pistas e o que dizer, **para** conduzir a cena sem perder o fio.

- Prioridade: MVP
- Regras: RN-10, RN-20
- Módulos: play, maps, notes

#### Critérios de aceite
- **Dado** uma cena de RP no mapa, **quando** o mestre escreve nela os ganchos, as pistas e o que dizer, **então** só o mestre vê essas notas.
- **Dado** uma pista numa cena, **quando** o mestre a revela, **então** ela aparece para os jogadores **e** nas anotações deles ([MR-030](#mr-030-anotações-do-jogador)).
- **Dado** uma pista não revelada, **quando** um jogador abre a cena ou a lista de anotações, **então** o servidor não manda a pista nem o nome dela (RN-10).

#### No app
- **Ganchos:** a coluna `map_points.hooks` ("Ganchos e anotações"), Markdown de até 4.000 caracteres, só num ponto de cena, salva com o ponto (`UpdateMapPoint`) e só o mestre a recebe (no mapa e na cena aberta).
- **Pistas:** `AddSceneClue`, `UpdateSceneClue`, `MoveSceneClue` e `RemoveSceneClue`, uma mudança por chamada, de 1 a 500 caracteres, no máximo 30 por cena; o mestre recebe cada uma com quem a tem ("Todos", "Só a Brisa"; "Ninguém ainda" a tela calcula).
- **Revelar:** `RevealSceneClue(pista, character_ids)`, só o mestre. Só os jogadores que o mestre escolher recebem a pista, e a tela não marca ninguém de antemão; cada jogador a recebe uma vez e não há como desfazer nem esconder de novo. O servidor guarda uma cópia do texto, então editar ou apagar a pista depois não muda o que o jogador recebeu. Com sessão aberta vira o evento `clue_revealed` (só IDs) e só quem a recebeu ouve `notes_changed`; quem está offline a lê na próxima vez. Cada jogador conta o que descobriu como quiser, à mesa, em roleplay.
- **Cena sem ações:** `OpenScene` abre qualquer ponto de cena. O `SceneBlocked` `NO_ACTIONS` continua no enum, mas não é mais enviado.
- **Nas telas:** no painel de um ponto de cena do editor, "Pistas" (a lista em ordem com quem a tem, "Todos", "Só Brisa" ou "Ninguém ainda"; ↑ ↓, editar no lugar, remover com a pergunta no lugar e "Voltar" em foco; "30 de 30" e a frase do limite) e "Ganchos e anotações" (até 4.000 caracteres, com o cadeado e "Só você vê"; salvam com "Salvar ponto", as pistas na hora). Na cena aberta, a coluna das pistas e dos ganchos (abertos no computador, recolhidos no celular, com o cadeado sempre à vista) e "Revelar", que abre um diálogo (uma folha no celular) **sem ninguém marcado**: o botão tracejado "Revelar a pista" diz por que não age, "Marcar todos" é uma ação de texto e o botão cheio nomeia quem recebe ("Revelar para Brisa", "Revelar para 2 jogadores", "Revelar para todos"); depois, "Revelada para todos às 21:20" ou "Revelada só para Brisa às 21:26", e "Revelar aos outros" na pista revelada em parte. Qualquer ponto de cena abre no seletor, mesmo sem ações.
- Os links `map:` e `character:` do Markdown dos ganchos aparecem na cena aberta como no documento, mas ainda não abrem nada ali.
- Testes: `TestMR029_TheMastersHooksAndClues`, `TestMR029_ClueLimits`, `TestMR029_ARevealedClueReachesOnlyTheChosenPlayers`, `TestMR029_RevealRules`, `TestClueAuthorizationMatrix`, `TestScenesWithNoActionsOpen`, `TestRN20_PlayersNeverGetHooksOrUnrevealedClues` (lê o que o jogador recebe como JSON). Playwright `e2e/tests/notes.spec.ts`: "o mestre escreve três pistas e os ganchos no ponto, abre a cena e revela uma pista; o jogador a recebe nas anotações e nunca vê os ganchos nem a pista que não foi revelada" (`@MR-029 @MR-030 @RN-20`) e "uma cena sem ações abre, e o jogador a vê; a lista das pistas para em 30 e o jogador escreve até 300 anotações"; as telas passam no axe e nas conferências de layout em `a11y.spec.ts` (`scanNotesScreens`).

### MR-030: Anotações do jogador

**Como** jogador, **quero** um bloco de notas sempre à mão, na página da sessão e na ficha, **para** anotar o que acontece sem sair do app.

- Prioridade: MVP
- Regras: RN-10, RN-20
- Módulos: notes, maps, play

#### Critérios de aceite
- **Dado** que estou na campanha, **quando** escrevo uma nota na página da sessão ou na ficha, **então** só eu vejo a nota **e** ela fica guardada para a próxima sessão.
- **Dado** uma cena que eu já descobri (revelada ou aberta), **quando** etiqueto a nota com ela, **então** a nota mostra a cena.
- **Dado** uma cena que eu ainda não descobri, **quando** abro a lista de cenas para etiquetar, **então** ela não aparece, e o nome dela nunca chega ao meu celular.

#### No app
- O módulo `notes` e o `NotesService` (`ListNotes`, `CreateNote`, `UpdateNote`, `DeleteNote`, `ListNoteScenes`), só para o jogador ativo e só nas próprias anotações: de 1 a 2.000 caracteres, no máximo 300 por jogador por campanha. A lista traz também as pistas reveladas ("Pista do mestre", só leitura, fora das 300), da mais nova à mais antiga, com filtro por cena.
- O mestre e os outros jogadores nunca leem uma anotação (`not_found`). Excluir a conta apaga as anotações.
- Uma cena é descoberta quando o ponto é revelado no mapa ou a cena é aberta numa sessão (mesmo escondida), para o grupo todo, e continua descoberta se o ponto for escondido depois. Vale também com a névoa de guerra (MR-036): revelar é a escolha do mestre, então a cena revelada entra nas anotações de todo o grupo mesmo num lugar que nenhum personagem vê; a névoa continua escondendo o mapa. Só uma cena descoberta serve de etiqueta, e o seletor (`ListNoteScenes`) só lista essas; uma cena não descoberta é recusada igual a uma que não existe.
- **Nas telas:** o botão "Anotações" (contornado, com ícone e palavra) na barra do app, em toda página da sessão do jogador; com uma pista nova ele diz "Anotações · 1 nova" ("Anotações · 1", com `aria-label`, abaixo de 360 px). A pista que chega abre um aviso ("O mestre revelou uma pista para você.", "Abrir anotações" e ✕ de 44 px) que fica até ser aberto ou dispensado. A folha "Anotações" (uma folha no celular, um diálogo no computador) tem a lista (a pista marcada "Pista do mestre", só leitura, com um cadeado), o filtro por cena (só as cenas descobertas e "Sem cena", cada uma com a contagem; volta a "Todas" ao fechar), os dois estados vazios, "Nova anotação" com a etiqueta de cena e o limite de 2.000, e editar e apagar (apagar pergunta no lugar). Lê de novo no `notes_changed` e uma vez depois de cada `ready`.
- Na ficha do personagem (computador), o painel "Anotações" é o primeiro bloco da quarta coluna, com as mesmas anotações; continuam editáveis com a ficha travada, e o mestre nunca recebe o painel, nem na ficha do jogador.
- Testes: `TestMR030_PlayerNotesArePrivate`, `TestMR030_TagsOnlyDiscoveredScenes`, `TestMR030_NoteLimits`, `TestMR030_DeletingTheAccountDeletesTheNotes`, `TestNotesAuthorizationMatrix`, `TestRN20_PlayersNeverGetHooksOrUnrevealedClues`. Playwright: os dois testes de `e2e/tests/notes.spec.ts` citados na MR-029 (o jogador escreve a anotação com a cena descoberta, o seletor nunca lista uma cena não descoberta, o mestre nunca vê o painel, a ficha mostra o limite de 300).

#### Relacionadas
- Uma pista revelada pela [MR-029](#mr-029-ganchos-e-pistas-da-cena) aparece nas anotações do jogador.

### MR-031: NPCs na cena

**Como** mestre, **quero** mostrar os retratos dos NPCs entrando e saindo da imagem durante uma cena, como numa visual novel, **para** os jogadores verem quem está "ali".

- Prioridade: MVP
- Regras: RN-10
- Módulos: play, maps

#### Critérios de aceite
- **Dado** uma cena de RP aberta, **quando** o mestre põe o retrato de um NPC (da galeria) na cena, **então** os jogadores o veem entrar, ao vivo.
- **Dado** um NPC na cena, **quando** o mestre o tira, **então** ele sai da imagem dos jogadores ao vivo.
- **Dado** um NPC que ainda não entrou, **quando** um jogador consulta a sessão, **então** o servidor não manda o retrato nem o nome dele.
- **Dado** um NPC em cena, **quando** um jogador consulta a cena, **então** recebe só o nome, o retrato e se ele fala: nunca o tipo, o PV, a CA, o XP, a ficha nem o ID do personagem (RN-20).
- **Dado** um retrato de NPC, **quando** um jogador tenta baixá-lo, **então** só consegue enquanto o NPC está em cena; fora de cena, com a cena fechada ou trocada, é `404`.
- **Dado** um mestre que quer o retrato de um NPC, **quando** o escolhe na ficha, **então** só vale uma imagem da galeria da campanha, e só num NPC.
- **Dado** um retrato em PNG com fundo transparente, **quando** o NPC entra em cena, **então** a transparência é mantida e o palco o desenha sem caixa atrás, só com uma linha de base.
- **Dado** a cena aberta, **quando** um jogador toca num NPC do palco, **então** vê o NPC maior, com o nome e o retrato, e nada mais: a cena é toda do mestre.

#### No app
- O retrato é o `portrait_image_id` da ficha do NPC. O palco guarda até 4 NPCs por sessão, em ordem, com quem fala, e fechar ou trocar a cena o esvazia. O NPC escondido no combate pode entrar em cena (isso não o revela na ordem do combate). Ver [Arquitetura](../../architecture.md#npcs-on-stage).
- O editor do NPC tem "Retrato" (ficha curta e passo Básico de inimigo e chefe): as iniciais sem imagem, "Escolher/Trocar retrato" no seletor da galeria, "Remover retrato" com a pergunta no lugar, tudo salvo com a ficha; o retrato aparece no cartão do NPC no combate e no cabeçalho da ficha.
- O mestre vê "Em cena" na cena aberta ("Dar a fala"/"Fala agora", "Tirar de cena", "Pôr em cena" no lugar ou numa folha no celular, "4 de 4"); os jogadores veem o palco com recortes sem caixa, quem fala em destaque, e tocam num personagem para vê-lo maior. O palco some quando está vazio.
- A lista "Pôr em cena" vem de uma só chamada, `ListCharacters`, cuja linha de cada NPC traz `portrait_url` (só para o mestre), sem ler as fichas.
- Testes: `TestMR031_TheMasterPutsNPCsOnStage`, `TestMR031_APlayerSeesOnlyNameAndPortrait`, `TestMR031_PortraitsAreVisibleOnlyOnStage`, `TestMR031_DeletingAPortraitClearsIt`, `TestMR031_AnNPCKeepsAPortraitFromTheCampaignsGallery`, `TestMR031_APortraitMustBeAnImageOfTheCampaign`, `TestMR031_TheMastersNPCCardHasThePortrait`, `TestMR031_TheMastersListCarriesThePortraitURL`, `TestStageAuthorizationMatrix`, `TestTransparentPNGKeepsAlpha`; Playwright `stage.spec.ts` (`@MR-031`, `@RN-20`), `a11y.spec.ts` (`scanStageScreens`) e o Vitest de `stage-*`, `portrait-field` e `StageController`.

#### Relacionadas
- Usa as imagens da [galeria](#mr-019-galeria-de-imagens), como a [MR-028](#mr-028-mostrar-uma-imagem-aos-jogadores).

### MR-032: Destaques do combate

**Como** mesa, **quero** uma tela de destaques no fim do combate, **para** celebrar quem fez o quê: qual jogador causou mais dano, curou mais e levou mais dano.

- Prioridade: MVP
- Regras: RN-20
- Módulos: play

#### Critérios de aceite
- **Dado** um combate que termina, **quando** o mestre o encerra, **então** a mesa vê, por categoria, o jogador cujo personagem causou mais dano, o que curou mais e o que levou mais dano (o "tanque").
- **Dado** que um NPC causou ou levou dano, **quando** a tela de destaques aparece para o jogador, **então** ela mostra só os números dos jogadores, sem o PV, a CA nem as rolagens dos NPCs (RN-20).
- **Dado** um combate encerrado, **quando** a mesa abre os destaques, **então** vê cinco categorias, "Mais dano causado", "Mais cura", "Tanque", "Golpe final" e "Acertos críticos", cada uma com o número e todos os empatados; uma categoria em que todos têm 0 fica de fora.
- **Dado** um golpe de 28 de dano num goblin de 7 PV, **quando** os destaques são calculados, **então** contam 7: só os PV que realmente saíram.
- **Dado** uma ação que o mestre desfez, ou um dano que ninguém aplicou, **quando** os destaques são calculados, **então** ela não conta.
- **Dado** um jogador, **quando** pede os destaques, **então** recebe da tabela só a linha do próprio personagem (com os zeros) e nunca a de outro; só o mestre recebe a tabela inteira; um combate que não terminou é recusado.
- **Dado** que a sessão teve tesouros achados ([MR-041](#mr-041-tesouros-e-xp-por-ouro)), **quando** o resumo da sessão é mostrado, **então** há o destaque "Mais tesouro encontrado": as PO que cada personagem achou naquela sessão, com um tesouro achado por dois dividido entre eles, arredondado para baixo; um tesouro achado fora de uma sessão, ou desmarcado depois, não conta; todo membro recebe a categoria, em qualquer modo de XP.
- **Dado** que a sessão teve testes de cena com CD, **quando** o resumo da sessão é mostrado, **então** há o destaque "Mais testes passados fora do combate", como enganação e intimidação, contando só as rolagens de cenas que mostravam a CD aos jogadores e só de ações com CD: uma rolagem numa cena que escondia a CD não entra em conta nenhuma, para ninguém.
- **Dado** uma sessão que acabou, **quando** o mestre abre o resumo, **então** vê a duração, quantos combates e cenas houve, "Testes passados fora do combate" ("9 de 12"), os destaques de todos os combates somados e a tabela por jogador ("Pensantus: 3 de 4"); **e** o jogador vê os vencedores de cada categoria com o número deles (sem o "de N") e o próprio resultado, nunca o de outro jogador (RN-20).

#### No app
- **Destaques do combate.** `CombatService.GetCombatHighlights` calcula tudo dos `session_events` do combate encerrado. "Mais dano causado" e "Tanque" contam também os PV temporários que absorveram dano.
- **Na tela, fim do combate.** O resumo do combate do mestre tem "Destaques do combate" entre os números do resumo e o XP, com a tabela "Números de cada jogador". O jogador vê o cartão "O combate acabou" no alto da sessão, com "Você" e "Seu resultado", até fechar. "Seu resultado" traz os cinco números do personagem dele (dano causado, dano recebido, golpes finais, cura e acertos críticos), com os zeros, vindos da linha que o servidor manda ao jogador.
- **Resumo da sessão.** `PlayService.GetSessionSummary(campaign_id, game_session_id)`, de qualquer membro, só para uma sessão que acabou (a aberta é recusada com `SESSION_NOT_ENDED`: os números dela ainda mudam, e cada combate já tem os destaques dele). Reaproveita o código dos destaques (`tallyHighlights`, `categoriesOf`), soma por personagem os combates da sessão e acrescenta a categoria `CHECKS_PASSED`. O app sabe que o resumo existe pelo `session_ended` do stream, que traz o id da sessão. "Mais tesouro encontrado" é `HIGHLIGHT_KIND_TREASURE_FOUND` e `SessionCharacterSummary.treasure_found_po`, lidos de `maps` pela interface `play.MapKeeper.TreasureFoundIn`; o número é lido dos tesouros como estão agora, não dos eventos.
- **Resumo da sessão na tela.** Quando a sessão acaba (o mestre confirma "Encerrar sessão" ou o stream manda `session_ended`), a página lê `GetSessionSummary`.
  - O **mestre** cai em "Sessão encerrada": a duração, os combates, as cenas abertas e "Testes passados fora do combate 9 de 12". "Resumo da sessão" traz os destaques da sessão inteira (os do combate e "Mais testes passados fora do combate", com "de 5 tentados" quando o vencedor tentou 5) e a tabela "Testes passados fora do combate" ("Pensantus 3 de 4"), só dele, com a nota de que só contam cenas que mostraram a CD. "Voltar à campanha" é o único botão.
  - O **jogador** recebe o cartão "A sessão acabou" com a duração, os vencedores e os números deles (sem "de N"), "Você" no que ganhou e "Seu resultado, Pensantus" (os cinco números do combate, se ele lutou, e "Testes passados fora do combate 3 de 4", se tentou algum). Não há tabela para ele. O cartão substitui o do combate que ainda estivesse aberto. "Fechar" e o X deixam o aviso simples "A sessão acabou". Se o resumo não puder ser lido, a página mostra esse aviso.
  - "Mais tesouro encontrado": o mestre ganha, no "Resumo da sessão", o bloco (a tabela de `shared/highlights`, com a legenda "Só conta o que foi marcado durante a sessão."), uma linha por personagem que achou algo (na ordem em que apareceram), com o valor em PO, e a nota "Quem não encontrou nada não aparece. Um tesouro encontrado por duas pessoas divide o valor, arredondado para baixo."; sem tesouro, a linha "Nenhum tesouro registrado nesta sessão." com o ícone. O quadro da categoria sai dos destaques do mestre (o bloco já a mostra, e inteira). O jogador a vê como um quadro do cartão "A sessão acabou" (o vencedor e o valor) e em "Seu resultado" ("Tesouro encontrado"). O servidor não manda o nome do tesouro nem a hora no resumo, então a linha mostra só quem e quanto. "Achado por Brisa" é só do destaque, nunca uma parte do XP (ver [MR-041](#mr-041-tesouros-e-xp-por-ouro)).
  - As peças (os quadros dos destaques, a tabela por jogador e o cartão) são as mesmas do fim do combate, em `shared/highlights`.
- **Testes.** `TestMR032_HighlightsOfTheAmbush` (o combate de referência: Toren 23 de dano, Brisa 24 levados, Pensantus e Toren com 2 golpes finais), `TestMR032_HighlightsSkipTheUndoneAndCountWhatHappened`, `TestMR032_OverkillDoesNotCount`, `TestMR032_UndoneActionsAreSkipped`, `TestMR032_DamageTakenIsWhatTheMasterApplied`, `TestMR032_HealingIsWhatWasGivenBack`, `TestMR032_TiesNameEveryoneAndZerosAreLeftOut`, `TestMR032_HighlightsAuthorizationMatrix`, `TestMR032_SessionSummary`, `TestRN20_SessionSummaryHidesWhatTheSceneHid`, `TestMR032_SummaryCountsTreasureFoundInTheSession` (pacote `maps`, que marca e desmarca pelo `MapService`). E2E: `stage.spec.ts` (`@MR-032`), `session-summary.spec.ts` (`@MR-032`, `@RN-20`: "ao encerrar a sessão o mestre vê o resumo com a tabela por jogador e o jogador vê o cartão \"A sessão acabou\"; um teste rolado com a CD escondida não conta") e `gold.spec.ts` ("o resumo da sessão tem Mais tesouro encontrado...").

#### Relacionadas
- [MR-041](#mr-041-tesouros-e-xp-por-ouro): o tesouro que alimenta "Mais tesouro encontrado".
- O resumo da sessão aparece no fim da sessão (artboard E8-11, estados 4 e 5). Ver [Arquitetura](../../architecture.md#combat-highlights) e [Arquitetura](../../architecture.md#session-summary).

### MR-033: Imprimir o mapa com a grade

**Como** mestre, **quero** imprimir o mapa atual, ou salvar em PDF, com a grade na escala da mesa, **para** jogar com miniaturas.

- Prioridade: MVP
- Regras: RN-21
- Módulos: maps

#### Critérios de aceite
- **Dado** um mapa com grade, **quando** o mestre abre "Imprimir com a grade", **então** vê a tela "Imprimir o mapa" com o quadrado de 2,54 cm (uma polegada) e o papel A4, e a conta das folhas: 30 × 20 quadrados dão 76,2 × 50,8 cm, em 9 folhas A4.
- **Dado** a tela de impressão, **quando** o mestre muda o tamanho do quadrado (de 1 a 10 cm, com vírgula ou ponto) ou o papel (A4, A3, A2, A1, Carta ou Ofício), **então** o resumo, a prévia das folhas e "As contas" (uma linha por papel, nas duas orientações) mudam na hora; a orientação é a que gasta menos folhas, paisagem se empatam. Com 2 cm em A3, são 3 folhas em retrato.
- **Dado** um mapa maior que uma página, **quando** o mestre imprime, **então** o mapa é dividido em folhas com 1 cm de margem e 1 cm de sobreposição, com a grade alinhada entre elas, cada folha com o rótulo de onde colar ("Página B2 · cole à direita da B1 e abaixo da A2") e uma régua de 5 cm para conferir a escala.
- **Dado** um tamanho fora de 1 a 10 cm, **quando** o mestre digita, **então** a tela mostra o erro, deixa a prévia vazia e o "Imprimir" tracejado, que não faz nada.
- **Dado** uma impressão de mais de 16 folhas, **quando** o mestre a configura, **então** um aviso âmbar nomeia o papel que gasta menos; acima de 36, a prévia deixa de mostrar os rótulos. Nada é bloqueado.
- **Dado** um mapa sem grade, **quando** o mestre olha a página do mapa, **então** o botão "Imprimir com a grade" está desabilitado, com o motivo escrito ao lado.
- **Dado** um mapa escondido dos jogadores, **quando** o mestre imprime, **então** sai o mapa inteiro: a impressão é do mestre. O jogador nunca vê o botão nem a tela (a rota responde "Só o mestre imprime o mapa.").
- **Dado** a impressão, **então** saem só a imagem e a grade: pontos, marcas e tokens ficam de fora.

#### No app
- Só no navegador: a grade e a imagem já vêm do mapa, e nada roda no servidor. A tela é a rota `/campaigns/:id/maps/:mapId/print` (`pages/maps/map-print`); a conta fica em `print-math.ts`, sem DOM.
- O que sai da impressora é só CSS de impressão (`@page` com o tamanho do papel escolhido e margem de 1 cm, `@media print`). Não há PDF no servidor nem dependência nova. Ver [Design](../../design.md#print-the-map).
- O mestre escolhe o tamanho do quadrado e o do papel. O Ofício é o brasileiro (21,6 × 33 cm).
- O navegador pode encolher a página ao imprimir: por isso a tela pede escala 100% e sem cabeçalhos e rodapés, e cada folha leva a régua de 5 cm.
- Testes: Vitest `print-math.spec.ts` (as contas verificadas na revisão do desenho, todo papel nas duas orientações, o desempate, os rótulos, a grade em cada folha), `map-print.spec.ts`, `map-head.spec.ts`. Playwright `map-print.spec.ts` (`@MR-033`): o mestre abre a impressão, muda para 2 cm e A3 e vê 3 folhas; uma medida fora de 1 a 10 cm trava o "Imprimir"; um mapa sem grade mostra o motivo; o jogador não vê o botão e a rota diz que é só do mestre; no papel (`emulateMedia` de impressão e `page.pdf`) sai uma folha por página, na escala, sem nenhum controle. A tela passa o axe e as conferências de alinhamento em `a11y.spec.ts` (`scanPrintScreens`).

#### Relacionadas
- [RN-21](regras.md).

### MR-034: Movimentos especiais

**Como** jogador, **quero** que o app trate salto, terreno difícil e cobertura, **para** as regras de movimento valerem na tela como valem na mesa.

- Prioridade: MVP
- Regras: RN-21
- Módulos: play, rules

#### Critérios de aceite
- **Dado** um personagem com Força 16, **quando** ele salta, **então** o app mostra o alcance do salto em distância e em altura, calculado a partir da Força, e desconta do movimento.
- **Dado** um quadrado marcado como terreno difícil, **quando** o personagem entra nele, **então** cada quadrado custa o dobro do movimento.
- **Dado** um alvo atrás de uma cobertura marcada no mapa (meia ou três quartos), **quando** alguém o ataca ou ele resiste a um efeito de Destreza, **então** o servidor soma +2 ou +5 à CA ou ao teste, pela linha entre os dois; atrás de uma parede, ele não pode ser alvo. O mestre também pode marcar a cobertura de um combatente para o que o mapa não mostra.
- **Dado** a Disparada, **quando** o jogador a usa, **então** ela continua funcionando junto com o terreno difícil.
- **Dado** outra criatura no caminho, **quando** o personagem passa pelo espaço dela, **então** o espaço de quem não é inimigo custa como terreno difícil, o de um inimigo não pode ser atravessado (só com dois tamanhos de diferença), e ninguém termina o movimento no espaço de outro.
- **Dado** um inimigo ao lado, **quando** o personagem sai do alcance dele sem ter usado Desengajar, **então** o app oferece um ataque de oportunidade a quem controla o inimigo, e o movimento espera a resposta.

#### No app
- **Movimento no servidor.** O `MoveCombatant` cobra o círculo (a linha reta, em décimos de pé), o terreno difícil, as paredes, as outras criaturas (lado, tamanho, "Aliado"), o voo e o salto (`jump`, a corrida, os limites no `GetTurnOptions`). O `GetMoveOptions` manda cada quadrado alcançável e o motivo dos recusados. O alcance dos ataques e das magias conta os quadrados entre os centros arredondados para baixo. A cobertura do mapa e a marca do mestre entram na CA e nos testes de Destreza, e a cobertura total tira o alvo. O Desengajar liga a marca do turno.
- **Salto.** Toren (Força 16) salta 16 pés com corrida e 8 parado, por cima do entulho; Brisa, com Força 8, não faz o mesmo (`TestMR034_JumpsAndTheRunningStart`). Em `rules/combat.JumpLimits`: Força 16 dá 16 pés de distância e 6 de altura com corrida.
- **Camadas do mapa.** O mestre pinta quatro camadas na grade de um mapa: terreno difícil, parede, cobertura (meia ou três quartos) e luz (`PaintMapCells`, até 400 quadrados por chamada, a última escrita vale, repetir não muda nada). `GetMapLayers` as devolve empacotadas no formato de `rules/grid`. O mestre lê as quatro. O jogador de um mapa que vê, sem névoa, lê parede, terreno e cobertura, e nunca a luz. Outras colunas na grade ou outra imagem apagam as camadas, e as duas mudanças são recusadas enquanto há um combate no mapa.
- **Ataque de oportunidade.** Um movimento que sai do alcance de um inimigo que vê quem anda, e tem a reação, dá a ele uma oferta (`Encounter.opportunity_offers`). O salto em distância também provoca; o movimento forçado que o mestre marca (`forced`, a caixa "Movimento forçado" do cartão do mapa do combate, que vale para um arrasto e se desliga sozinha) não. O movimento aterrissa na hora e o turno de quem anda espera (`OPPORTUNITY_PENDING`) até cada oferta ser respondida e o dano dela se resolver. O controlador do reator ataca (`RollAttack` com `opportunity_offer_id`, sem a conferência de alcance, com a cobertura medida do quadrado de onde quem anda saiu), recusa (`DeclineOpportunity`) ou o mestre segue sem esperar (`SkipOpportunity`). A 0 PV quem anda volta ao quadrado em que saiu do alcance, se ele estiver livre. Uma oferta que ninguém pode mais responder deixa de segurar o turno. O desfazer do movimento leva as ofertas.
- **Telas.** A página "Mover" lê o `GetMoveOptions` (o círculo, o custo de cada quadrado, o motivo de uma recusa, o aviso de quem pode provocar e a pergunta de uma armadilha conhecida) e tem "Saltar" (distância e altura, com os limites do servidor). As camadas do mapa são desenhadas por `shared/map-layers`. A lista de alvos de um ataque ou de uma magia diz a cobertura com a origem ("Meia cobertura (do mapa)", "Três quartos (marcada pelo mestre)") e deixa de fora quem a parede cobre por inteiro. A ordem do mestre diz a cobertura de cada inimigo contra quem tem a vez e tem "Marcar cobertura" (rádios no lugar) e "Marcar como aliado". O ataque de oportunidade tem o aviso do jogador, o cartão do mestre, a espera de quem andou, "Esperando a reação do mestre" e "Seguir sem esperar". O NPC de ficha curta tem o campo "Tamanho".
- **Editor do mapa.** Na página do mapa, no computador, o mestre escolhe "Pontos | Pintar". Em "Pintar" estão as ferramentas Terreno difícil, Parede, Cobertura (Meia, Três quartos) e Luz (Claro, Penumbra, Escuro), o "Apagar" e o pincel de 1×1 ou 3×3.
  - Arrastar pinta quadrado a quadrado (nenhum fica sem pintar entre dois movimentos do mouse), clicar pinta um, Shift apaga. O teclado pinta também: as setas movem o cursor do pincel, Espaço pinta, Esc sai, e as setas dizem a coluna e a linha em voz baixa. No tablet, um dedo pinta e dois dedos movem e ampliam o mapa.
  - Em "Pintar" o mapa fica limpo: sem nomes, e marcadores e tokens a 40 %.
  - O que se pinta aparece na hora e vai ao servidor em lotes (até 400 quadrados, um lote por vez, na ordem dos traços: a última escrita vale). "Camadas" diz "Tudo salvo", "Salvando" ou "Não salvou" (o motivo é o que o servidor respondeu; "Tentar de novo" só quando o servidor não respondeu, e uma recusa descarta o que esperava). Antes de ler o que já está pintado, ou se a leitura falha, não se pinta e nunca se diz "Tudo salvo". Um traço vai ao mapa em que foi feito e, ao sair da página com traços que o servidor não recebeu, o editor pergunta.
  - Conta cada camada ("4 quadrados · custa +1,5 m por quadrado", "2 quadrados de meia cobertura e 1 de três quartos", nunca o nome de um objeto) e deixa mostrar ou esconder cada uma no mapa do mestre.
  - Sem grade não há o que pintar: as ferramentas ficam tracejadas, com o motivo escrito ("Defina a grade para pintar e ligar a névoa."), e "Definir a grade" fica no painel "Grade". Com um combate no mapa, pintar vale; "Mudar a grade" e "Trocar imagem" ficam desligados, com o motivo dito uma vez ("Combate em andamento").
  - **Mudar a grade, trocar a imagem e "Esquecer o que foi visto" perguntam no lugar** (o foco no título, "Voltar" primeiro, o botão cheio que diz o que apaga; nada é apagado antes do segundo clique; a mesma pergunta serve a "Salvar as mudanças em …?", "Apagar …?" e "Desmarcar …?"). "Mudar a grade?" abre sempre, mas só traz o aviso do que apaga, e só deixa apagar, quando há algo pintado ou visto e o tamanho muda de verdade (com a névoa ligada, o editor presume que os jogadores já viram algo). "Trocar imagem" sem nada a perder vai direto ao seletor.
  - A linha "O que está desenhado na imagem, os jogadores veem." fica sob o estado do mapa. No celular não há pintura: um aviso fixo ("Pintar só no computador"), as camadas desenhadas com a legenda e as configurações da névoa, que são editáveis.
- **Testes.**
  - Regras: `rules/grid` (`TestReachAgainstTheCave`, `TestMoveCostAgainstTheCave`, `TestCreaturesOnTheWay`, `TestCoverBetweenAgainstTheCave`, `TestLeavesReach`, `TestRange`, as camadas e a linha) e `rules/combat` (`TestJumpLimits`): os números da caverna dos desenhos e os do salto de Toren (Força 16) e de Brisa (Força 10).
  - Servidor (`go test`, com o banco, a caverna dos desenhos como mapa): `TestMR034_JumpsAndTheRunningStart`; `TestRN21_DifficultTerrainAndOtherCreaturesCostMore` (cobre também a Disparada, que dobra a velocidade antes do custo); `TestMR034_CoverRaisesTheArmorClassOfTheTarget` (os caixotes, a coluna, uma criatura no meio, a parede, a marca, o teste de Destreza e a CA que o jogador nunca recebe); `TestRN21_TheCircleCostsTheExactLine`, `TestRN21_WallsColumnsAndSqueezesBlockAMove`, `TestRN21_GetMoveOptionsDrawsTheCircle`, `TestMR034_ReachIsCountedInSquaresBetweenCentresRoundedDown`, `TestMR034_TheMapsLayersDecideTheMove`.
  - Camadas: `TestMR034_PaintingTheLayers`, `TestMR034_PaintingRefusals`, `TestMR034_AGridOrImageChangeClearsTheLayers`, `TestMR034_NoGridOrImageChangeDuringACombat`, `TestMR034_PlayersReadOnlyWhatTheyMay`, `TestMR034_LayerChangesReachTheRightStreams`, `TestHintGateMergesTheHintsOfAnInterval`.
  - Ataque de oportunidade: `TestRN21_LeavingAnEnemysReachOffersAnAttack`, `TestRN21_WhoProvokesAnOpportunityAttack`, `TestMR034_GetMoveOptionsWarnsWhichSquaresProvoke`, `TestMR034_TheAnswersToAnOffer`, `TestMR034_TheMoversTurnWaits`, `TestMR034_TheWaitLastsUntilTheDamageSettles`, `TestMR034_TheCoverOfAnOpportunityAttackIsFromWhereTheMoverLeft`, `TestMR034_AForcedMoveIsTheMastersAndNeverProvokes`, `TestMR034_ZeroHitPointsSendsTheMoverBack`, `TestMR034_ZeroHitPointsStaysWhereTheLeftSquareIsTaken`, `TestMR034_OffersNobodyCanAnswerStopHoldingTheTurn`, `TestMR034_DecliningAndSkippingLeaveAnUndoableEntry`, `TestRN20_AHiddenReactorIsNeverNamedToAPlayer`, `TestRN20_ThePlayersWarningDoesNotDependOnAnNPCsReaction`.
  - E2E: `move.spec.ts` (`@MR-034`, `@RN-21`: o círculo com entulho, a parede, o salto, a cobertura e a marca, as ofertas, o movimento cortado, o aliado); `map-editor.spec.ts` (`@MR-034`: pintar paredes, terreno e cobertura e apagar, o servidor guarda; o pincel 3×3 e o teclado; o mapa sem grade; mudar a grade e trocar a imagem perguntando; o combate no mapa); `a11y.spec.ts` (cada estado, nos dois temas, no desktop e no celular). O Vitest cobre `move-plan`, `jump-plan`, `opportunity`, `cover`, `layers`, `paint-layers`, `paint-queue`, `paint-tools` e os componentes do editor.

#### Relacionadas
- [RN-21](regras.md). Ver [Arquitetura](../../architecture.md#grid-vision-and-presets) e [Arquitetura](../../architecture.md#layers-fog-and-point-kinds).
- As regras escolhidas: o mestre pinta no mapa o terreno difícil, as paredes e a cobertura (meia: muro baixo, caixotes; três quartos: coluna, seteira), que os jogadores veem e podem usar; cada quadrado de terreno difícil em que se entra custa 1,5 m a mais; outras criaturas e o ataque de oportunidade como nos critérios acima.

### MR-035: Armadilhas

**Como** mestre, **quero** pôr armadilhas escondidas no mapa, com a CD para notar e para achar, o gatilho e o efeito, **para** surpreender a mesa com as regras do jogo.

- Prioridade: MVP
- Regras: RN-10
- Módulos: maps, play

#### Critérios de aceite
- **Dado** um mapa, **quando** o mestre põe uma armadilha com a CD para notar (Percepção passiva), a CD para achar (Investigação), o gatilho e o efeito (dano, uma resistência), **então** só o mestre a vê.
- **Dado** uma armadilha escondida, **quando** um personagem chega perto e enxerga o quadrado com a Percepção passiva igual ou maior que a CD para notar, ou procura e passa num teste de Percepção (contra a CD para notar) ou de Investigação (contra a CD para achar), **então** o jogador dele passa a ver a armadilha.
- **Dado** uma armadilha disparada, **quando** o gatilho acontece, **então** o efeito é aplicado pelo servidor (dano, resistência) **e** a armadilha passa a aparecer para os jogadores.
- **Dado** uma armadilha que ainda não foi achada nem disparada, **quando** um jogador consulta o mapa, **então** o servidor não manda nem a posição (RN-10).

#### No app
- **Predefinições e conteúdo.** As oito armadilhas de exemplo do SRD são predefinições (`effects/traps.json`, `Content.TrapPresets()`) com a tabela de gravidade do SRD. Testes: `TestTrapPresetsOfTheSRD`, `TestTrapSeverityTables`, `TestLoadTrapsRefuses`.
- **A armadilha no mapa.** `CreateMapPoint` e `UpdateMapPoint` aceitam o tipo `TRAP` com a CD para notar (opcional) e para achar, a área de 1×1 a 4×4, o gatilho (ao entrar ou manual), o efeito em partes (ataque, dano que sempre acerta, condições, resistência com o que acontece ao falhar e ao passar) e o estado (armada, disparada, desarmada), tudo conferido com as mesmas regras das predefinições. Criar de uma predefinição (`ContentService.ListTrapPresets`) copia os números e tudo continua editável. O jogador só recebe uma armadilha revelada ao personagem dele (`RevealTrap`, que escreve `trap_revealed` e avisa só os jogadores escolhidos), disparada ou revelada a todos, e nunca as CDs, o efeito nem a predefinição (RN-10).
- **Notar.** O personagem de jogador, ou uma criatura dele, que **termina um movimento** (um combate, ou o token que o mestre solta) a até 3 m de uma armadilha armada, enxergando um quadrado da área, com a Percepção passiva da ficha (a da ficha da criatura, no caso dela), menos 5 na penumbra ou no escuro visto pela visão no escuro, igual ou maior que a CD para notar, passa a conhecer a armadilha (`how = noticed`, evento `trap_noticed`), e só o jogador dele recebe o aviso. Uma armadilha sem CD para notar nunca é notada assim. O mestre lê "Quem notaria" no `GetTrapNoticers`: cada personagem com a Percepção passiva, o −5 da luz nos quadrados da armadilha como ele os enxerga agora, se está no mapa, a menos de 3 m, se enxerga e se notaria.
- **Procurar.** "Procurar armadilhas" (`SearchForTraps`): o jogador rola Percepção (contra a CD para notar; a armadilha sem essa CD nunca é achada assim) ou Investigação (contra a CD para achar), com o d20 do app ou o dado físico (RN-18), contra cada armadilha armada a até 3 m cujo quadrado o personagem enxerga. Passar revela só a esse personagem (`searched`). A resposta lê igual quando não há nada e quando o teste falhou, sem nunca trazer uma CD. Se o mapa não puder ser lido depois para dar o nome do que foi achado, o jogador é avisado de quantas armadilhas achou e de olhar o mapa. Num combate é a ação Procurar do SRD, só na vez do personagem, e gasta a ação (`ACTION_USED`, `NOT_YOUR_TURN`). A busca de Percepção na penumbra tem desvantagem; a visão às cegas nunca fica em penumbra.
- **Disparar.** A armadilha **dispara** ao entrar na área (o movimento em linha reta para no primeiro quadrado da área, um salto só dispara onde pousa, o token que o mestre solta dentro dela dispara, o NPC nunca; o mestre que arrasta um personagem só conta onde o movimento termina) ou na mão do mestre (`FireTrap`, que escolhe quem é pego; sem alvos, as criaturas na área). O token só dispara num mapa que os jogadores veem.
- **Efeito.** O servidor rola o ataque, o dano e a resistência de cada criatura pega, com o bônus dela, como as resistências das magias. O dano num NPC ou numa criatura cai na hora, as condições vão para os combatentes, e o dano num personagem de jogador **espera o mestre** (RN-02), que o aplica (`ApplyPendingDamage` no combate, `ApplyTrapDamage` fora dele), mudando a quantia antes se quiser, ou descarta. Um NPC fora do combate só tem uma linha no histórico, e as condições viram um lembrete. O dano que espera sobrevive ao fim do combate e da sessão. A armadilha disparada fica visível a todos ("Disparada"). O mestre a desarma depois do teste de ferramentas de ladrão da mesa (`DisarmTrap`, evento `trap_disarmed`), e uma armadilha desarmada nunca dispara.
- **Avisos.** `GetMoveOptions` marca os quadrados cujo movimento entra numa armadilha armada que o personagem conhece (`known_trap_point_id`, `known_trap_name`), para a tela perguntar "Isso entra no Fosso escondido. Mover assim mesmo?", e nunca uma que ele não conhece. Um movimento cortado por uma armadilha desconhecida só diz que parou, e a armadilha vira pública porque disparou. Um "Desfazer" no combate desfaz o disparo inteiro (a armadilha armada de novo, o PV e as condições de volta, o dano pendente apagado), e só depois o movimento.
- **Escolhas nossas, sem regra do SRD.** Os 3 m, medidos entre os centros dos quadrados; o ataque da armadilha vai de uma em uma às criaturas pegas, em ordem; o dano de cada parte de cada criatura é uma rolagem; uma armadilha "Ao entrar na área" pega quem está na área, ou só quem entrou se o efeito é de alvos manuais; o teste ativo na penumbra não tem desvantagem (o mestre decide).
- **Tela do mestre.** "Armadilhas do mapa" na página da sessão: um cartão por armadilha (Armada, Disparada às HH:MM ou Desarmada; quem a conhece; as CDs, o gatilho e o efeito; "Quem notaria" com os números do servidor; "Revelar para…", "Disparar…" e "Desarmar"), o dano que espera por ele (o número editável, os PV antes e depois, "Aplicar N de dano" ou "Não aplicar"), também durante um combate, e o "Registro". O servidor não diz a distância em metros, só se o personagem está a até 3 m (`TrapNoticer.in_range`) e, num campo à parte, se a Percepção dele alcançaria a CD (`passes_dc`): "Quem notaria" escreve "Nota" ou "Não nota" para quem está perto e "Se chegar a 3 m: nota" para quem está longe, sem comparar nada no navegador. "Disparar…" sem ninguém marcado dispara para quem o servidor achar lá (os personagens e as criaturas deles com token no mapa), e a tela diz isso. Se alguém que o mestre marcou sai da lista antes do clique (o token sumiu, o combate acabou), a folha diz quem saiu e espera uma nova escolha: uma escolha vazia quer dizer "todos na área", nunca o que o mestre pretendia. Cada pedido tem a sua chave de idempotência (os mesmos alvos de novo são uma nova tentativa; outros alvos são outro pedido). A lista de quem pode ter sido pego acompanha o "Quem notaria" da armadilha: o diálogo pode abrir antes de a leitura chegar, diz "Lendo quem está no mapa…" e se completa quando ela chega.
- **Tela do jogador.** "Procurar armadilhas" (Percepção ou Investigação, no app ou com o dado físico, com o segundo dado quando o servidor o pede; no combate é a ação Procurar), o aviso "Você notou uma armadilha.", a nota "Você caiu na armadilha…" na vez dele e o registro das próprias linhas. A armadilha e o baú aparecem no mapa pelas marcas de `MAP-LANGUAGE`, com legenda.
- **Editor: pôr a armadilha.** "Armadilha" na barra de pontos põe o ponto no mapa e abre o painel, com as **oito predefinições do SRD** como rádios (cada uma com uma linha: "Queda de 6 m, 2d6", "1 perfurante, 2d10 veneno · resistência de Constituição"; escolher uma preenche o formulário e tudo continua editável) e "Começar do zero". O formulário tem o nome, a "Descrição para você", a "CD para notar (Percepção)" (vazia: ninguém nota sozinho, como na Agulha envenenada) e a "CD para achar (Investigação)", o aviso "Não desenhe a armadilha na imagem: os jogadores veem a imagem.", a área de disparo (1×1 a 4×4), o gatilho (ao entrar ou manual), o efeito **em partes** que o mestre acrescenta e tira como ações de texto (Ataque, Dano que sempre acontece, Condição que sempre acontece, Teste de resistência com o que acontece a quem falha e a quem passa), quem é pego (a área ou o que o mestre escolhe) e o estado (Armada, Disparada, Desarmada). A dica de gravidade do SRD ("revés 10 a 11 · perigosa 12 a 15 · mortal 16 a 20") fica só sob a CD de uma resistência e o bônus de um ataque, escrita como o servidor manda (`ListTrapPresets`); o navegador não diz se a CD escolhida é "perigosa". Cada campo diz o que está errado depois da primeira tentativa de salvar (CD de 1 a 30, dados de d4 a d12 ou um número de 1 a 100, bônus de 0 a 20). "Quem notaria" é a mesma tabela (`GetTrapNoticers`, `passes_dc`), tudo do servidor. As três luzes por personagem do desenho (E9-02) **não** existem, porque o servidor responde um quadro só, o de como o personagem vê agora.
- **Testes.** Conteúdo e mapa: `TestMR035_PresetsFillTrapsAndLights`, `TestMR035_PointKindsAreValidated`, `TestMR035_UpdatingPointsOfTheNewKinds`, `TestMR035_RevealTrap`, `TestRN10_PlayersNeverReceiveTrapsLightsOrHiddenTreasure`. Em jogo: `TestMR035_ThePassiveNotice`, `TestMR035_TheNoticeNeedsADCAndTheDCToBeMet`, `TestMR035_QuemNotaria`, `TestMR035_APassiveNoticeFollowsTheLight`, `TestMR035_ACreatureNoticesWithItsOwnEyesAndFiresTheTrap`, `TestMR035_Searching`, `TestMR035_APhysicalDieFollowsTheCampaign`, `TestMR035_SearchingInACombatCostsTheAction`, `TestMR035_AMoveStopsAtTheFirstSquareOfTheArea`, `TestMR035_AJumpFiresATrapOnlyWhereItLands`, `TestMR035_NPCsNeverFireATrapAndTheMasterPlacesOnlyWhereTheMoveEnds`, `TestMR035_TheMasterFiresATrapByHand`, `TestMR035_TheAttack`, `TestMR035_OutsideACombat`, `TestMR035_ATokenDroppedInTheArea`, `TestMR035_ADisarmedTrapNeverFires`, `TestMR035_TheWarningBeforeSteppingIn`, `TestMR035_TheEffectsArithmetic`, `TestRN10_NoPlayerResponseNamesAnUnrevealedTrap`, `TestRN10_AMoveStoppedByAnUnknownTrapReadsAsTheTrapFiring`. Robustez: `TestMR035_ANoticeAndOtherEventsDoNotBlockTheUndo`, `TestMR035_ADartIsItsOwnSave`, `TestMR035_TheMasterAddsCreaturesToAFiring`, `TestMR035_TheActivityOutsideACombat`, `TestMR035_ATokenFiresOnlyOnAMapThePlayersSee`, `TestMR035_ATrapDisarmedOrDeletedMeanwhileIsJustNotThere`, `TestMR035_ARetriedTrapMoveKeepsTheOffers`, `TestMR035_APerceptionSearchInDimLightHasDisadvantage`, `TestMR035_BlindsightIsNeverLightlyObscured`, `TestMR035_AnEndedCombatKeepsTheTrapDamageForTheMaster`, `TestMR035_AKeyReusedOnAnotherTrapDamageIsRefused`, `TestMR035_APassiveNoticeTellsNobodyElseAndTheMasterOnlyWithNoContent`. E2E: `traps.spec.ts` (`@MR-035`, `@RN-10`, `@RN-02`); specs do Vitest de `core/traps` e `pages/live-session/traps`.

#### Relacionadas
- Vem do que o desenho de masmorras (a antiga MR-010) previa: armadilhas e baús.
- [MR-010](historias.md#mr-010-gerar-masmorras), [MR-038](#mr-038-quebra-cabeças) (uma jogada errada pode disparar uma armadilha do mapa). Ver [Arquitetura](../../architecture.md#layers-fog-and-point-kinds).

### MR-036: Névoa de guerra pela visão

**Como** jogador, **quero** ver no mapa só o que o meu personagem enxerga, **para** a exploração ter o suspense da mesa.

- Prioridade: MVP
- Regras: RN-10
- Módulos: maps, play, rules

#### Critérios de aceite
- **Dado** um mapa com áreas claras e escuras que o mestre marcou, **quando** o jogador o abre, **então** ele vê só o que o personagem dele enxerga.
- **Dado** uma fonte de luz (uma tocha, uma magia), **quando** o mestre a põe no mapa, **então** a área ao redor fica visível.
- **Dado** um personagem com Visão no escuro de 18 m na ficha, **quando** ele está numa área escura, **então** ele enxerga até 18 m, em tons de cinza.
- **Dado** o mestre, **quando** abre o mapa, **então** vê tudo. O que o jogador não enxerga nunca sai do servidor (RN-10).
- **Dado** um druida na forma de lobo, **quando** ele está numa sala escura, **então** não vê nada de novo: a fera não tem visão no escuro, e os sentidos dela valem no lugar dos do personagem (SRD).
- **Dado** um jogador com um familiar no mesmo mapa, a 30 m ou menos, **quando** ele escolhe "Ver pelos olhos do familiar", **então** vê o que o familiar vê, com os sentidos dele (a visão no escuro da coruja), e o personagem fica cego e surdo até o jogador parar; no combate gasta a ação e acaba no começo da próxima vez do personagem.

#### No app
- **Cálculo.** `rules/vision` calcula, de um mapa com paredes, luz base e pintada e fontes de luz, o que cada observador enxerga (claro, penumbra, cinza na visão no escuro, parede vista), a união de vários e a penalidade de −5 na penumbra. `effects/lights.json` guarda as luzes do SRD.
- **Configurações e luzes.** Cada mapa com grade pode ligar a névoa de guerra (desligada por padrão; `SetMapFog`), com a luz base (escuro por padrão, penumbra ou claro) e a "Visão do grupo" (desligada). Desligar a névoa mantém as camadas, e tirar a grade a desliga. O `Map` traz `fog_enabled` e `group_vision` também ao jogador; a luz base, só ao mestre. O ponto do tipo `LIGHT` (predefinição do SRD ou raios personalizados, de 0 a 120 pés) nunca vai a um jogador. A luz que um personagem carrega (`SetCarriedLight`, o jogador no próprio personagem e o mestre em qualquer um) vai só ao mestre e ao dono.
- **O que cada jogador vê.** Num mapa com névoa, cada jogador recebe só o que o personagem dele vê agora ou já viu, em todas as leituras (`GetMap`, `ListMaps`, `GetMapLayers` e o `GetMapVision`): sem imagem (só o tamanho; a rota de imagens também a recusa, e ligar a névoa copia a imagem se ela serve a outra coisa), pontos só nos quadrados vistos ou lembrados, o token de personagem de jogador sempre e o de NPC só num quadrado visto agora, as camadas filtradas e nunca a luz. Cada jogador vê pelo próprio personagem (no quadrado do combatente enquanto há combate), com a visão no escuro da ficha derivada; a "Visão do grupo" dá a todos a união. O que foi visto fica (`map_vision_memory`), sem criaturas, até o mestre pedir "Esquecer o que foi visto" (`ForgetMapVision`) ou trocar a grade ou a imagem. O mestre abre o mapa "como" um personagem (`as_character_id`). O stream manda `vision_changed` só a quem a visão mudou, e o movimento de um NPC só a quem o vê. Nada é lembrado de um mapa que os jogadores não veem.
- **O combate por jogador.** Num mapa com névoa, durante um combate, cada jogador recebe só os NPCs que o personagem dele vê agora (com "Visão do grupo", o grupo): fora da ordem, do turno ("Vez do mestre"), dos alvos de ataque e de magia, do registro, do stream e das ofertas de ataque de oportunidade, e `not_found` se o jogador o nomeia. Os personagens de jogador e as criaturas dele nunca somem. O movimento é planejado com o terreno que o jogador conhece (o que não vê é piso) e cortado onde algo o bloqueia, sem nomear parede nem criatura; o gancho `CombatMoved` roda depois de cada movimento. O registro guarda quem via cada linha quando ela aconteceu (`seen_by`). O reator de um ataque de oportunidade tem de ver quem anda (um NPC, com os sentidos da ficha). A cobertura que o jogador lê é a do que ele conhece e vê; o ataque de oportunidade vale mesmo se o NPC recuou para o escuro; a vista lida antes da transação é conferida (revisão do combate) e relida; o `revision` do jogador é por jogador; `DeleteMap` é recusado durante um combate.
- **A imagem em peças por jogador.** Num mapa com névoa o jogador recebe a imagem em peças de 16 × 16 quadrados, montadas no servidor só com os quadrados que ele viu ou lembra (preto opaco no resto e sem metadado), de `GET /images/maps/{mapa}/tiles/{tx}/{ty}`. Duas imagens que só diferem em quadrados que ele não conhece dão a ele peças idênticas byte a byte (o que `TestRN10_TileOracle` prova); em troca, nas bordas da névoa fica escurecido 1 pixel num PNG e até um bloco de 16 pixels num JPEG. O `GetMapVision` traz o índice (`tiles_path`, `tile_squares`, `tiles` com a revisão de cada peça), e uma peça sem quadrado conhecido não existe. Há uma cópia de trabalho da imagem por mapa (até 2.048 px, encolhida quadrado por quadrado, dois mapas na memória), uma renderização por vez (`503` com `Retry-After` se esperar demais), um limite de faltas de cache por usuário (`429`) e um cache de peças limitado; trocar a imagem ou a grade e "Esquecer o que foi visto" as descartam. Ver [Arquitetura](../../architecture.md#per-player-image-tiles).
- **Forma Selvagem e os olhos do familiar.** Os sentidos de um druida na Forma Selvagem são os da fera (`PartyVision`; um lobo não tem visão no escuro: o `TestMR036_AWolfSeesNothingInTheDark` lê o `GetMapVision` do lobo, o que ele já tinha visto lembrado e a visão de volta ao assumir a forma própria). `StartFamiliarSight` e `StopFamiliarSight` (`PlayService`) ligam e desligam os olhos do familiar: precisa de um familiar (a criatura de origem `familiar`), do personagem e dele no mesmo mapa com grade e a 30 m ou menos (20 quadrados, na posição dos tokens ou, no combate, dos combatentes). Fora do combate vale até o jogador parar; no combate gasta a ação e acaba no começo da próxima vez dele, e parar antes não devolve a ação. O personagem fica cego e surdo e o jogador vê só o que o familiar vê (a vista do próprio personagem sai; as dos outros personagens ainda valem na "Visão do grupo"), com as condições no combatente, que o fim tira, só as que a visão deu. A visão acaba ao entrar num combate. A névoa põe o familiar como o observador do jogador, no quadrado dele, enquanto está a 30 m. Só o familiar enxerga pelo dono: as outras criaturas **aparecem** (o token delas é de grupo) mas não enxergam por ninguém (`TestMR037_ACreatureDoesNotSeeForItsOwnerUnlessItIsTheFamiliarsEyes`; a justificativa está na [Arquitetura](../../architecture.md#wild-shape-the-familiars-eyes-and-creature-tokens-mr-037-mr-036)).
- **Tela do jogador e do mestre (`shared/fog-map`).** O mapa do jogador com névoa na sessão e no combate (as peças montadas no lugar, o sombreado de cada estado, a legenda sob o mapa, o zoom e as fichas do grupo no celular), "Carregando o mapa" com "parte N de M", o aviso "Seu personagem não está neste mapa", a linha "Luz que você carrega" e a folha dela, o painel "Luz dos personagens" e "Ver como" do mestre, e os olhos do familiar (o botão na linha sob o mapa, na lista de ações do combate e no cartão da criatura da ficha; a pergunta, a faixa e a linha "Cego" do turno). Mapas sem névoa seguem como eram. O navegador não calcula regra: desenha o que o servidor manda e só conta quadrados (o "76 quadrados vistos"). Não há "Ver como" da ordem do combate: o servidor não manda ao mestre a ordem como um jogador a vê, e a tela não finge. O botão "Ver pelos olhos" aparece para qualquer familiar; a distância de 30 m é só do servidor.
- **Editor: a névoa, a luz e "Ver como".** "Névoa de guerra" é um painel do editor, também no celular: "Ligar a névoa" (precisa da grade), a "Luz de base" (Claro, Penumbra, Escuro), "Visão do grupo" e "Esquecer o que foi visto", que pergunta no lugar. Cada mudança vai na hora (`SetMapFog`, um campo por chamada). "Luz" é um tipo de ponto: as fontes do SRD que o servidor lista como rádios de 52 px (nome e raios em metros) e "Personalizada", com os raios em metros (múltiplos de 1,5 m, até 36 m, os limites do servidor) e a conta em quadrados embaixo. **O alcance é o dos raios, não dos quadrados iluminados:** nenhuma leitura dá ao mestre quais quadrados a luz alcança (a visão do mestre sem "Ver como" é "tudo visto"), então o mapa desenha dois anéis (o da luz clara, inteiro; o da penumbra, tracejado) e diz que as paredes cortam a luz de verdade só com a névoa ligada, o que se vê em "Ver como". O ponto é só do mestre: o jogador vê a luz, nunca o ponto. "Ver como" aparece no editor com a névoa ligada, na coluna ao lado do mapa: escolher um personagem troca o mapa pelo mesmo `app-view-as-map` da sessão (as peças, os tokens e as camadas daquele jogador) com a faixa "Você está vendo o mapa como Toren" e "Voltar à sua vista"; a lista conta os quadrados que cada um vê.
- **Testes.**
  - Regras: `TestVisionAgainstTheCave` (os desenhos da caverna: Pensantus, Toren, Brisa e Sálvia no escuro, Toren e Brisa com a tocha), `TestSensesBeyondDarkvision`, `TestLightIsBlockedByWalls`, `TestUnionOfViewers`, `TestPassivePenalty` e os benchmarks do orçamento de tempo.
  - Configurações e vistas: `TestMR036_FogSettings`, `TestMR036_CarriedLight`, `TestRN10_FogPlayersReceiveOnlyWhatTheirCharacterSees`, `TestMR036_FogViewsMatchTheCaveOracle` e `TestMR036_GroupVisionIsTheUnion` (os números dos desenhos da caverna, `rules/vision/cave_expected_test.go`: Pensantus, Toren, Brisa e Sálvia, com e sem a tocha, e a união), `TestMR036_WhatWasSeenStays`, `TestMR036_VisionChangedReachesTheRightPlayers`, `TestMR036_SeeAsAPlayersCharacter`, `TestRN10_FogMapImageIsCopiedWhenShared`, `TestMR036_VisionOfAMapWithoutFog`, `TestCoalescedHintsQueueOnce`, `TestMR036_NothingIsRememberedOnAMapPlayersCannotSee`, `TestMR036_AClearBeatsALateRefresh`, `TestMR036_AColdStartTellsNobodyForNothing`, `TestMR036_ParentsAreNamedOnlyThroughWhatThePlayerKnows`, `TestMR036_AFogMapDuringACombat`, `TestMR036_PaintingTellsThePlayersWhoseLayersChanged`, `TestRN10_AFogMapsImageIsNeverReusedRaw`.
  - Combate: `TestRN10_FogCombatEachPlayerSeesOnlyTheirNPCs`, `TestRN10_FogCombatGroupVisionAndTheStream`, `TestRN10_FogCombatMovesPlanOnWhatThePlayerKnows`, `TestRN10_FogCombatTheLogKeepsWhoSawIt`, `TestRN10_FogCombatReactorsMustSeeTheMover`, mais os do RN-20 (cobertura, recuo, vista velha, `not_found` em toda chamada, repetição, apagar o mapa, reator com os próprios olhos, o aviso do Escudo).
  - Peças: `TestRN10_TilesCarryOnlyWhatThePlayerSaw`, `TestRN10_APlayerWhoSawNothingGetsNoTile`, `TestRN10_TileAuthorization`, `TestRN10_TileCaching`, `TestRN10_TilesAreInvalidated`, `TestRN10_TileOracle`, `TestTileBudget_*`.
  - Familiar e Forma Selvagem: `TestMR036_TheFamiliarsEyesGiveItsView` (o jogador vê a guarita acesa pela coruja, ninguém mais vê, a 30 m a visão sai e o que viu fica lembrado), `TestMR036_FamiliarSightOutsideACombat`, `TestMR036_FamiliarSightInCombat`, `TestMR036_FamiliarSightKeepsAConditionTheCharacterHad`, `TestMR036_FamiliarSightNeedsTheFamiliarWithin30m`, `TestMR036_AWolfSeesNothingInTheDark`.
  - E2E: `fog.spec.ts` (`@MR-036`, `@RN-10`: Pensantus e Toren veem mapas diferentes no mesmo instante e nenhum pedido de jogador baixa `/images/<id>`; o que já foi visto fica escurecido e sem NPCs; o personagem fora do mapa; a tocha de Toren; o "Ver como" do mestre; os olhos do Nanquim dentro e fora do combate; o mapa sem névoa), `map-editor.spec.ts` (`@MR-036`, `@RN-10`: ligar a névoa, a luz de base, a visão do grupo e esquecer; o que o mestre pinta chega ao jogador só onde o personagem dele enxerga; a Luz; "Ver como" Toren mostra o mapa de Toren) e `a11y.spec.ts` (o mapa e seus estados, nos dois temas, no desktop e no celular, e em 1024 e 320 px para a sessão).

#### Relacionadas
- [MR-037](#mr-037-criaturas-do-personagem) (o familiar, a Forma Selvagem), [MR-034](#mr-034-movimentos-especiais) (paredes e camadas), [MR-035](#mr-035-armadilhas) (luzes e armadilhas como pontos).
- Regras escolhidas: cada jogador vê o que o próprio personagem enxerga, e o mestre pode ligar "Visão do grupo" em cada mapa; os personagens dos jogadores nunca somem para os outros jogadores; o que já foi visto fica escurecido; as paredes que o mestre pinta bloqueiam a vista e a luz, como nas regras oficiais; e o inimigo que o personagem não vê não aparece para ele: atacar no escuro (até uma Bola de Fogo num canto suspeito) é com o mestre.
- Ver [Arquitetura](../../architecture.md#layers-fog-and-point-kinds).

### MR-037: Criaturas do personagem

**Como** jogador, **quero** controlar uma criatura minha (a forma selvagem do druida, os mortos-vivos de Animar Mortos, um familiar), **para** jogá-la no combate com a ficha dela.

- Prioridade: MVP
- Regras: RN-02, RN-20
- Módulos: play, rules, characters

#### Critérios de aceite
- **Dado** um druida com forma selvagem, **quando** ele a usa, **então** o jogador ganha a ficha da criatura, tirada das criaturas do SRD, e age com ela no combate: a CA, os ataques, os deslocamentos e os sentidos da fera, o PV da fera à parte, e nenhuma magia. A lista de feras segue o nível dele (nível 2: ND até 1/4, sem voo nem natação; 4: até 1/2, sem voo; 8: até 1).
- **Dado** um druida na forma de fera, **quando** a fera cai a 0 PV, **então** ele volta à forma própria e o dano que sobrou passa para o PV dele; "Voltar à forma normal" é uma ação bônus, e a cura na forma cura a fera.
- **Dado** o mestre, **quando** põe o token de uma criatura do personagem num mapa, **então** o grupo todo o vê (é um token de grupo, nunca escondido, mesmo na névoa).
- **Dado** uma criatura do personagem, **quando** chega a vez dela, **então** ela entra na ordem do combate, com a própria economia de ação, e o jogador a controla.
- **Dado** uma criatura do personagem, **quando** ela sofre dano, **então** o PV dela é separado do PV do personagem, e o mestre pode corrigir (RN-02).

#### No app
- **Regras.** As 334 criaturas do SRD 5.1 estão importadas (`srd51/data/monsters.json`, nomes em português `monster:<índice>`), o `MonsterDerived` (a ficha da criatura para o combate, com os outros deslocamentos), o efeito `summon` de Convocar Familiar (com o Pacto da Corrente), Animar Mortos e Conjurar Animais, e a Forma Selvagem do druida (`wild_shape`, níveis 2, 4 e 8, e a ficha combinada pelo SRD). `ContentService.ListCreatures` e `GetCreature` servem a lista e a ficha. Convocar Montaria fica fora por ora (pede as regras de combate montado).
- **Criaturas do personagem.** A criatura pertence ao personagem (`character_creatures`: dono, tipo do SRD, nome de até 40 caracteres, origem, o que pode atacar, o grupo da conjuração, PV) e dura até o jogador ou o mestre a dispensar. `CharacterService`: `ListCharacterCreatures`, `GiveCreature` (o mestre dá qualquer criatura do SRD), `RenameCreature`, `DismissCreature` e `AdjustCreatureHitPoints` (mestre, fora do combate). Um personagem tem no máximo 40 criaturas (`CREATURE_LIMIT`).
- **Convocar.** `PlayService.CastSummon` fora do combate (Convocar Familiar como ritual, **sem espaço**; Animar Mortos, que gasta o espaço; Conjurar Animais) e `CastSpell` com `summon` no combate (Conjurar Animais: as criaturas entram na ordem com **uma iniciativa só**, e o total que empata com outro combatente vira turno conjunto). Um familiar novo substitui o antigo. Regra nossa: as criaturas de uma conjuração de Animar Mortos também rolam uma iniciativa só (o SRD diz isso só de Conjurar Animais), e o grupo usa o bônus de Destreza da primeira criatura.
- **No combate.** A criatura é um combatente `creature` (`CombatantKind.CREATURE`, `controlled_by_me`, `owner_character_id`), com a vez e a economia dela, que o jogador dono controla. O familiar não ataca. O PV fica nela e é número só para o dono e o mestre (RN-20). A 0 PV ela é derrotada e dispensada, e o PV volta para a lista do dono quando o combate acaba. `EndConcentration` dispensa as criaturas da conjuração (RN-22).
- **Forma Selvagem.** `CharacterService.ListWildShapeForms` lista as feras do nível. `PlayService.AssumeWildShape` (num combate ou fora dele, gasta um uso de Forma Selvagem e, no combate, a ação) e `LeaveWildShape` (ação bônus) mudam a forma, que é parte dos `vitals` (a tabela `character_wild_shapes`). O druida luta com os números da fera (CA, ataques, deslocamento, tamanho, sentidos) e não conjura (`WILD_SHAPE_NO_SPELLS`). O dano cai no PV da fera, a cura também, e quando ela cai o que sobrou passa para o druida. O desfazer do mestre repõe a forma e o PV da fera. O PV da fera só é número para o mestre e o jogador do druida (RN-20). O mestre pode tirar a forma pela correção dos `vitals` (`wild_shape_hit_points_current`, 0 acaba). Eventos `wild_shape_started` e `wild_shape_ended` (a fera, o motivo e o dano que sobrou).
- **Tokens.** O mestre põe o token de uma criatura no mapa de exploração com `PlaceMapToken` (`creature_id`; tabela `map_creature_tokens`): é um token de grupo, nunca escondido, como o de um personagem de jogador (só o token de um NPC se esconde; o `SetMapTokenHidden` não recebe criatura). O token de uma criatura segue o combatente dela quando a luta acaba. No mapa de combate o token é redondo com contorno tracejado.
- **Tela da ficha.** O painel "Criaturas" da ficha (depois de "Combate" e "Magias"; só para quem pode ter uma criatura: conjura uma das três magias, é druida com Forma Selvagem ou já tem uma; para outro jogador o servidor responde `not_found` e o painel não existe, RN-20). Um cartão por criatura (o nome que a mesa deu, "Corvo · Miúdo · Familiar de Pensantus", CA, PV "1 de 1" e o deslocamento em metros), "Ver a ficha do Nanquim" (a página da ficha da criatura, `/campaigns/:id/characters/:personagemId/creatures/:criaturaId`, em inglês no que é do SRD, com a linha que diz por que um familiar não ataca), "Renomear" (no lugar, até 40 caracteres), "Dispensar" (pergunta no lugar, foco em "Voltar"; o jogador e o mestre dispensam qualquer criatura, inclusive uma dada (o servidor decide)) e, para o mestre, "Corrigir PV" fora do combate. A folha de conjurar fora do combate: Convocar Familiar como ritual (o nome, as 15 formas, as 4 do Pacto da Corrente, "Conjurar como ritual · 1 hora · sem gastar espaço"), Animar os Mortos (o espaço decide quantos, um tipo só) e Conjurar Animais (as quatro opções com a conta do espaço e o animal). Só durante uma sessão (fora dela o botão fica tracejado e diz "Agora não há sessão aberta."), e a recusa do servidor aparece na própria folha, em palavras. O mestre dá a criatura na lista de personagens da campanha ("Dar uma criatura", busca por nome em português ou inglês, tipo e ND nas 334 do livro) e dispensa dali; o jogador é avisado em tempo real (`creatures_changed`). Uma criatura dada com a folha de conjurar aberta é avisada depois que a folha fecha, e uma releitura que falha mantém a lista e diz "Não deu para atualizar as criaturas agora." com "Tentar de novo". A folha de conjurar lê o `GetSummonOptions` (as magias que o personagem conjura, o que cada nível traz, os espaços, o que uma conjuração mandaria embora), avisa antes de conjurar o que uma conjuração dispensa e mistura tipos de criatura quando a opção conta várias. Ver [Design](../../design.md#character-creatures-mr-037).
- **Tela do combate.**
  - Quando o jogador joga mais de um combatente (o personagem e as criaturas), a barra do turno ganha as abas ("Sálvia · Já agiu · 13", "Lobos atrozes (2) · Sua vez · 10"; no notebook, em cima da página) e a página segue o turno até a aba de quem age; sem criaturas não há abas.
  - A página da vez das criaturas ("Vez dos seus Lobos atrozes", "Depois de vocês: Nanquim") tem um bloco por criatura (imagem, nome, PV do dono, CA do livro, "Ainda age"/"Encerrou", ação e movimento próprios, "Atacar" pelo fluxo de ataque de sempre, "Mover o Lobo atroz 1" pela página Mover) e "Encerrar a parte dos Lobos" (que encerra a parte de cada um) ou "Encerrar a vez do Nanquim". O familiar mostra "O familiar não ataca" e as ações padrão, e o do Pacto da Corrente ganha a linha "Reação · Atacar".
  - Conjurar Animais em combate reaproveita a folha de conjurar fora do combate (as opções e as feras vêm do `GetSummonOptions`), com a linha da concentração no rodapé, o d20 da iniciativa do grupo (o do app, ou o digitado de um dado físico) e o resultado ("Os 2 Lobos atrozes entram no combate com iniciativa 10 (um d20 para os dois: 8 + 2)…").
  - Para o mestre, a ordem mostra as criaturas como fichas redondas tracejadas, "CA 14 · da Sálvia", a caixa do turno conjunto com o nome do grupo, a legenda das três formas e a etiqueta "Concentração" (sólida, pública: todos veem quem concentra; a magia fica na linha "Concentra em Conjurar Animais · 2 Lobos atrozes") e, só para quem tem criaturas, "Perdeu a concentração" (pergunta no lugar, um aviso com ícone, foco em "Voltar", "Dispensar os Lobos"; o desfazer do servidor traz as criaturas de volta, então a pergunta não diz que não se desfaz). Um NPC concentrado em Teia só tem a linha, sem pergunta. O jogador é avisado ("Você perdeu a concentração em Conjurar Animais. Os 2 Lobos atrozes sumiram."); o aviso fica até ele tocar, vem dos dados do combate, vale só para aquele combate, e um desfazer que traz as criaturas de volta o tira. Os outros jogadores veem as criaturas só pela palavra de estado (RN-20).
  - A Forma Selvagem é uma linha da Ação ("Transformar", "restam 2 de 2 usos · volta no descanso curto ou longo", "Sem usos", "Sem ação disponível"); a folha de feras vem do `ListWildShapeForms` (busca por nome em português ou inglês, os números do livro, o custo antes de confirmar); a vez como fera tem a faixa "Na forma de Lobo", as duas reservas de PV, "Sem magias na forma de fera…", "Voltar à forma normal" (ação bônus) e os traços do livro. Quando a fera cai a 0 PV o aviso "O Lobo caiu a 0 PV e você voltou à forma normal. 6 de dano passaram para você." fica até o toque. Na linha do "Grupo" do mestre, um druida em forma de fera tem uma etiqueta junto dos pontos de vida com o nome da fera e a reserva dela ("Lobo 8 de 11 PV"), que some quando a forma acaba. Na ficha, o painel "Criaturas" tem a linha "Transformar: Forma Selvagem" (só durante uma sessão).
  - O que o servidor não manda (e a tela, por isso, lê do livro): a CA de uma criatura ou da fera (vem da ficha do livro). O dano que passa da fera para o druida vem na linha do registro do combate (`CombatLogWildShape.carried_damage`, só para o mestre e o jogador do druida), e o nome em português dos ataques das criaturas vem do servidor (`CreatureAction.name_pt`).
- **Nomes.** Os nomes em português dos ataques das criaturas são `attack:<nome do SRD em minúsculas, com hífens>` em `names_pt.json`; o `TestAttackNamesPT` confere todos os ataques das 334 criaturas.
- **Testes.**
  - Regras e conteúdo: `TestSnapshot`, `TestNamesPT`, `TestReferences`, `TestMonsterDerived`, `TestListCreatures`, `TestSummonOptions`, `TestCheckSummon`, `TestSummonLoaderRefuses`, `TestWildShapeForms`, `TestWildShapeDerived` (`backend/internal/rules`); `TestListAndGetCreatures` e `TestAuthorizationMatrix` (`backend/internal/characters`).
  - Criaturas: `TestMR037_TheRitualFamiliarSpendsNoSlotAndANewOneReplacesTheOld`, `TestMR037_AnimateDeadAtTheThirdAndFifthCircles`, `TestMR037_ConjureAnimalsInCombat`, `TestMR037_TheMastersGiftRenameAndDismiss`, `TestMR037_ConcentrationEndingDismissesTheCastingsCreatures`, `TestMR037_UndoOfAConjuringTakesTheCreaturesAway`, `TestMR037_ACreatureAtZeroLeavesAndTheHitPointsGoBack`, `TestMR037_ExistingCreaturesJoinACombatAndAGroupSharesOneRoll`, `TestMR037_TheKindAudit`, `TestMR037_TheChainFamiliarAttacksOnlyWithItsReaction`, `TestRN20_CreatureHitPointsOnlyToOwnerAndMaster`, `TestMR037_SummonOptions` e as linhas das criaturas do `TestAuthorizationMatrix` (`characters` e `play`).
  - Forma Selvagem: `TestMR037_WildShapeBeastsByLevel`, `TestMR037_WildShapeInCombat`, `TestMR037_WildShapeDamageGoesToTheBeast`, `TestMR037_WildShapeExactDamageEndsTheFormWithNothingLeftOver`, `TestMR037_WildShapeUndoOfTheActions`, `TestMR037_WildShapeOutsideACombat`, `TestMR037_WildShapeLeavesOtherCombatantsAlone`, `TestMR037_ACreatureTokenFollowsItsCombatantWhenTheFightEnds`, `TestMR037_ACreatureDoesNotSeeForItsOwnerUnlessItIsTheFamiliarsEyes`.
  - E2E: `creatures.spec.ts` (`@MR-037`, `@RN-20`) e `creatures-combat.spec.ts` (`@MR-037`, `@RN-20`); as telas novas entram no `a11y.spec.ts`. No Vitest, os auxiliares de visão do combate (`ownCombatant` nunca escolhe uma criatura), mais as specs do painel de criaturas, da folha de conjurar e dos blocos do combate.

#### Relacionadas
- [MR-036](#mr-036-névoa-de-guerra-pela-visão) (os olhos do familiar), [RN-02](regras.md), [RN-20](regras.md), [RN-22](regras.md).
- Ver [Arquitetura](../../architecture.md#creatures-summons-and-wild-shape-rules), [Arquitetura](../../architecture.md#character-creatures-in-play-mr-037) e [Arquitetura](../../architecture.md#wild-shape-the-familiars-eyes-and-creature-tokens-mr-037-mr-036).
- Regras escolhidas: entram a Forma Selvagem (o dano que sobra quando a fera cai a 0 PV passa para o druida, como no SRD), Convocar Familiar (e o Pacto da Corrente do bruxo), Animar Mortos e Conjurar Animais, e o mestre pode dar qualquer criatura do SRD a um personagem. As criaturas invocadas juntas rolam uma iniciativa só e agem juntas (o turno conjunto). Convocar Montaria fica para depois do MVP.

### MR-038: Quebra-cabeças

**Como** mestre, **quero** criar quebra-cabeças que os jogadores resolvem no app, ao vivo numa cena, **para** variar o ritmo da sessão.

- Prioridade: MVP
- Regras: RN-10, RN-18, RN-27
- Módulos: play, maps

#### Critérios de aceite
- **Dado** que sou mestre de "Mirathel", **quando** crio um quebra-cabeça de um dos seis tipos ("Apagar as luzes", a fechadura de combinação, os símbolos giratórios, o enigma, a sequência para repetir ou a cifra), **então** ele fica guardado na campanha e só eu o vejo.
- **Dado** um quebra-cabeça "Apagar as luzes" de 5 por 5, **quando** o crio, **então** o servidor gera um começo que tem solução e nunca já resolvido, **e** eu vejo quantos toques bastam.
- **Dado** uma sessão aberta, **quando** o mestre mostra um quebra-cabeça, **então** os jogadores o veem e o resolvem ao vivo, todos no mesmo estado, com a pista que o mestre escreveu, **e** veem quem fez a última jogada (o nome do personagem).
- **Dado** um quebra-cabeça mostrado, **quando** dois jogadores jogam ao mesmo tempo, **então** as duas jogadas valem, e todos veem o estado novo.
- **Dado** um enigma, **quando** um jogador digita a resposta, **então** o servidor a compara com as respostas que o mestre aceita, sem ligar para maiúsculas e acentos, **e** o jogador nunca recebe a resposta certa.
- **Dado** uma sequência para repetir, **quando** o mestre a toca, **então** os jogadores veem a sequência acontecer **e** depois a repetem; o servidor confere a ordem.
- **Dado** uma cifra, **quando** os jogadores a abrem, **então** veem a mensagem cifrada e decifram com a chave que acharam como pista na aventura; o servidor confere a mensagem decifrada.
- **Dado** um quebra-cabeça com dicas, **quando** o mestre solta a próxima dica, **então** os jogadores a veem; **quando** um jogador passa num teste de perícia contra a CD que o mestre pôs (Investigação, Arcanismo...), **então** ele recebe a próxima dica (RN-18: no app ou com o dado físico).
- **Dado** um quebra-cabeça de informação dividida, **quando** os jogadores o abrem, **então** cada um vê só a sua parte da pista, a que o mestre deu a ele, **e** eles precisam conversar para resolver.
- **Dado** um quebra-cabeça com consequências, **quando** uma jogada errada acontece (uma combinação ou resposta errada), **então** o que o mestre escolheu acontece: uma armadilha do mapa dispara (MR-035), uma tentativa do jogador se gasta, ou o limite de jogadas ou de tempo chega mais perto; no fim do limite, o quebra-cabeça para e o mestre é avisado.
- **Dado** um quebra-cabeça mostrado, **quando** os jogadores o resolvem, **então** o servidor confere a solução (o jogador nunca recebe a resposta), o quebra-cabeça para, o mestre é avisado **e** acontece o que ele escolheu em "Ao resolver": só o aviso (padrão), abrir uma porta (RN-26), revelar um ponto do mapa ou revelar uma pista da cena a quem resolveu (RN-27).
- **Dado** um quebra-cabeça mostrado, **quando** o mestre o recomeça ou o fecha, **então** os jogadores veem o mesmo começo de novo, ou param de vê-lo; "Gerar outro começo" ("Apagar as luzes" e símbolos giratórios) é uma ação à parte do mestre.

#### No app
- **Servidor, os três primeiros tipos.** "Apagar as luzes", a fechadura de combinação e os símbolos giratórios, com a guarda, as rodadas ao vivo, as dicas soltas pelo mestre e o "Ao resolver" (só avisar, abrir uma porta, revelar um ponto, revelar uma pista a quem resolveu). Ver [Arquitetura](../../architecture.md#puzzles-mr-038).
- **Servidor, os outros tipos.** O enigma, a sequência, a cifra, a dica por teste de perícia, a informação dividida e as consequências ("Ao errar": a armadilha, a tentativa e o limite de jogadas ou de tempo) são mais peças do `puzzleKind`, na mesma API e nas mesmas tabelas. Ver [Arquitetura](../../architecture.md#puzzles-mr-038) e [RN-27](regras.md).
- **Mestre: a lista e o editor.** O painel "Quebra-cabeças" na página da campanha: a lista com os quatro estados e o estado vazio, "Editar" até o primeiro "Mostrar", "Arquivar" perguntado na própria linha e "Mostrar os arquivados". As páginas "Novo quebra-cabeça" e "Editar quebra-cabeça": o tipo, o formulário de cada tipo, a pista, as dicas e "Ao resolver" com a porta, o ponto ou a pista escolhidos entre os mapas e as cenas da campanha, e a mensagem. O desenho diz "Apagar" na lista; o servidor só arquiva (e o que foi mostrado é histórico das sessões), então a tela diz "Arquivar".
- **Mestre: os formulários dos outros tipos.**
  - O enigma com o texto e as respostas aceitas (fichas com "×" de 44 px e "Só você vê").
  - A sequência com os sinos, tocados para pôr cada passo, "Apagar o último passo" e "Tocar para testar" (acende os passos só na tela dele).
  - A cifra com a mensagem, o deslocamento ou a palavra-chave, "Como os jogadores a veem", que **o servidor cifra** (`PreviewPuzzleCipher`, depois de uma pausa), e a pista da cena que guarda a chave.
  - Em **todo** tipo: "Dica por teste de perícia" (a perícia entre as 18 e a CD, as duas ou nenhuma), "Informação dividida" (até 8 partes, cada uma para um personagem de jogador, nenhum com duas) e "Ao errar" (um jeito de cada vez: nada, a armadilha de um mapa, uma tentativa por jogador de 1 a 10, ou o limite de jogadas e de minutos; a armadilha e a tentativa só nos três tipos que julgam uma jogada).
  - Cada recusa do servidor aparece no campo a que ela se refere (pelo motivo e pelo campo do detalhe tipado, nunca pela mensagem) e o foco vai para lá; a edição de um quebra-cabeça abre com tudo o que ele tinha.
  - Os sinos são de 3 a 8 e os passos de 3 a 12, como o servidor.
- **Mestre: na sessão.** O painel "Quebra-cabeças" com "Mostrar aos jogadores" e "Ver ao vivo" (a lista fica na coluna ao lado; o cartão ao vivo do quebra-cabeça escolhido, um de cada vez, fica na coluna principal, em cima do mapa): o painel como os jogadores o têm, a última jogada e quem a fez, quantas faltam no mínimo, as dicas, "Mostrar a próxima dica", "Gerar outro começo", "Recomeçar" e "Fechar" perguntados na própria tela, e, resolvido, quem resolveu, o que o servidor fez e a porta aberta no mapa. Só para o mestre e com "Só você vê": as respostas aceitas, os passos da sequência ou a mensagem sem cifra; a última jogada com o que foi digitado ou o sino tocado, as tentativas que cada jogador ainda tem, os contadores de jogadas e de tempo, quantas vezes a sequência foi tocada, a armadilha que disparou e quem errou, quem ganhou uma dica por perícia (com a rolagem) e "Tocar a sequência".
- **Jogador.** O jogador recebe o aviso "O mestre mostrou um quebra-cabeça" na sessão e joga na página do quebra-cabeça (`?puzzle=ID`, dentro da página da sessão, no lugar do painel, com os avisos da sessão por cima: a armadilha notada, o tesouro achado, a pista que chegou e o combate). Um toque por luz, as rodas da fechadura, os pilares com as ligações e o mural; e o "Resolvido" com o texto do mestre. O painel de 7 × 7 cabe em 320 × 568 px.
  - O enigma: o campo, "Responder", "Não é isso." com ícone e palavra, sem dizer o quanto chegou perto, "Suas tentativas 2 de 3", e o quadro tracejado com o motivo quando não há mais tentativas.
  - A sequência é vista passo a passo no ritmo do servidor (só os sinos já revelados; o número do passo, o sino em destaque com o nome escrito e falado por um leitor de tela; nenhum som é preciso), depois repetida por qualquer jogador, com "Errou o passo 4. A tentativa recomeçou; a Lia errou." e os "Passos certos 2 de 6" do servidor.
  - A cifra: a carta, a tabela de decifrar (um ajudante que o app nunca confere), o campo da mensagem e "Conferir".
  - "Tentar uma dica · Investigação": a CD nunca aparece; no app ou digitando o d20 do dado físico, como o RN-18 manda; "Você conseguiu. Esta dica é só sua" ou "Não deu desta vez."
  - Informação dividida: a parte da pista que é só dele, com os nomes de quem tem as outras.
  - Os contadores ("Jogadas 7 de 10", "Tempo 4:48 de 5:00", que corre sozinho a partir do prazo do servidor), "A armadilha disparou: Dardos envenenados." (só para quem vê o ponto), e o "parou" neutro.
- **O que o navegador sabe.** O navegador só desenha o que o servidor manda. A jogada vai com uma chave nova e, se a resposta não chega, é repetida com a **mesma** chave; a tela segue a resposta (e a leitura que a dica `puzzle_changed` provoca) só quando a revisão é maior. O navegador nunca vê o próximo passo da sequência antes de o servidor revelá-lo, a resposta, a mensagem sem cifra, a chave, a CD nem a parte de outro jogador. O servidor não manda a resposta digitada por outro jogador, então "resolveu às 21:34" não diz o que foi respondido; a mensagem da pista da chave está nas notas do jogador, com um botão "Abrir as anotações", porque a nota não leva o ID da pista. Ver [Design](../../design.md#puzzles-mr-038-e10-06-and-e10-12).
- **Extensível.** `PuzzleHost`, um painel por tipo e os painéis do mestre e do jogador foram feitos para receber um tipo novo sem reescrever nada.
- **Testes, servidor, primeiros tipos.** `TestMR038_CreateEachKindAndReadItAsTheMaster` (os três tipos, só o mestre, o começo nunca resolvido e o mínimo de toques), `TestMR038_CreateChecksWhatTheMasterWrote` (cada recusa com o motivo), `TestMR038_ASeedGivesTheSameStart`, `TestMR038_EditUntilShownAndArchive`, `TestMR038_ShowPlayAndSolveEachKind` (todos veem o mesmo estado e quem jogou por último; resolvido congela), `TestMR038_TwoPlayersMovingAtOnce` (as duas jogadas valem), `TestMR038_TwoMovesThatSolveAtOnce`, `TestMR038_AMoveWithTheSameKeyIsMadeOnce`, `TestMR038_ResetReseedAndClose`, `TestMR038_AnEditDropsThePreparedRun`, `TestMR038_TheMasterSolveMessage`, `TestMR038_TheHintIsThrottled`, `TestMR038_APuzzleNeverClosesTheUndoChain`, `TestMR038_HintsAreReleasedOneByOne`, `TestMR038_SolvingOpensADoor`, `TestMR038_SolvingRevealsAPoint`, `TestMR038_SolvingRevealsAClueToTheSolverOnly`, `TestRN10_PlayersNeverReceiveTheAnswer` e, em `rules/puzzle`, `TestLightsMinimumMatchesBruteForce`, `TestLightsKernelCases`, `TestPillarsMinimumAndLinks`.
- **Testes, servidor, outros tipos.** `TestMR038_RiddleCreateAnswerAndSolve` e `TestMR038_RiddleChecksWhatTheMasterWrote` (criar, mostrar, a resposta errada, a certa sem maiúsculas nem acentos, o recomeço e cada recusa), `TestMR038_SequencePlayedThenRepeatedWithAWrongStep` (tocar passo a passo sem mostrar um passo antes da hora, as jogadas recusadas antes e durante, o passo errado, a sequência inteira), `TestMR038_SequenceChecksWhatTheMasterWrote`, `TestMR038_APlayScheduleItsSteps`, `TestMR038_CipherRoundTripAndTheKeyAsAClue` (a ida e volta, a chave como pista que só quem a achou lê, a palavra-chave), `TestMR038_CipherChecksWhatTheMasterWrote`, `TestMR038_AHintWonBySkillCheckIsOnlyThePlayers`, `TestMR038_AHintByAppDiceAndTheDiceMode` e `TestMR038_HintChecksWhatTheMasterWrote` (dado físico e do app, passar e falhar, uma tentativa por dica, o modo de dados da campanha, a CD que nunca vai), `TestMR038_SplitInformationEachPlayerReadsOnlyTheirPart` e `TestMR038_SplitInformationChecksWhoGetsAPart` (o membro pendente sem nada), `TestMR038_AWrongMoveFiresTheTrapOnce`, `TestMR038_OnWrongChecksWhatTheMasterWrote`, `TestMR038_AnAttemptIsSpentPerPlayer`, `TestMR038_AMovesLimitStopsThePuzzle`, `TestMR038_ATimeLimitStopsThePuzzle`, `TestMR038_ConcurrentWrongAnswersAreCountedExactly` (as tentativas e o limite contados com exatidão com jogadas ao mesmo tempo), `TestRN10_PlayersNeverReceiveWhatTheNewKindsHide` e, em `rules/puzzle`, `TestFold`, `TestMatches`, `TestCipherRoundTrip`, `TestSequenceStrike`, `TestSequencePlayback`.
- **Testes, web.** Vitest: `puzzle-draft.spec.ts` (cada formulário vira o pedido e volta na edição, as recusas, a perícia e a CD, as partes, "Ao errar" com um jeito só), `puzzle-play.spec.ts` (a resposta errada própria e não a de outro, "Tentar uma dica" no app e com dado físico, a chave que se repete, a sequência lida de novo a cada passo com um relógio falso), `puzzle-boards.spec.ts` (o nome "Luz na linha 2, coluna 3, acesa", o teclado, cada jogada de cada tipo), `puzzle-form.spec.ts`, `puzzles-panel.spec.ts`, `master-run.spec.ts`, `puzzle-errors.spec.ts`. E2E: `puzzles.spec.ts` (`@MR-038 @RN-10`: a fechadura que abre uma porta de ponta a ponta com o JSON do jogador sem a solução, o painel de 7 × 7 em 320 × 568, a lista, os pilares e as perguntas na própria tela) e `puzzles-more.spec.ts` (`@MR-038 @RN-27 @RN-10 @RN-18`: o enigma errado e depois certo, com o JSON do jogador sem as respostas; a sequência vista e repetida, com um passo errado que dispara a armadilha; a cifra resolvida com a chave achada como pista da cena; a dica por perícia com o dado físico; a informação dividida, com o JSON de cada jogador só com a parte dele; o limite de jogadas que para o quebra-cabeça; e os três cabendo em 320 × 568). As telas e os estados entram no `a11y.spec.ts`.

#### Relacionadas
- [MR-047](#mr-047-mais-quebra-cabeças): as ideias que ficaram para depois do MVP.
- [MR-035](#mr-035-armadilhas) (a armadilha que uma jogada errada pode disparar), [RN-10](regras.md), [RN-18](regras.md), [RN-26](regras.md), [RN-27](regras.md). Ver [Arquitetura](../../architecture.md#puzzles-mr-038) e [Design](../../design.md#puzzles-mr-038-e10-06-and-e10-12).
- Os seis tipos, as dicas, o "Ao resolver", as consequências, a dica por teste de perícia e a informação dividida estão todos no MVP, para a sessão não ficar repetitiva. Os símbolos giratórios lembram os pilares de Skyrim, com símbolos nossos.

### MR-039: Imagens geradas para masmorras e cenas

**Como** mestre, **quero** gerar uma imagem a partir da masmorra, de um mapa ou da descrição de uma cena (a arte da cena, uma vista isométrica do mapa ou o próprio mapa com textura, casando com a grade) e pedir ajustes, **para** mostrar à mesa o lugar de que falo.

- Prioridade: MVP
- Regras: RN-10, RN-28
- Módulos: maps (galeria)

#### Critérios de aceite
- **Dado** uma masmorra gerada ([MR-010](#mr-010-gerar-masmorras)) ou a descrição de uma cena, **quando** o mestre pede uma imagem, com o texto e o estilo dele, **então** o app gera a imagem e a guarda na galeria da campanha, escondida dos jogadores.
- **Dado** uma imagem gerada, **quando** o mestre escreve um novo pedido ("mais escura", "com uma ponte"), **então** o app a edita sabendo da cena, sem recomeçar do zero, **e** guarda a nova ao lado da anterior.
- **Dado** que o limite do mês da campanha foi atingido, **quando** o mestre pede outra imagem, **então** o app recusa e diz por quê e quando volta.
- **Dado** um mapa com grade (gerado ou desenhado), **quando** o mestre pede "O mapa com textura", **então** o app gera o próprio mapa visto de cima, com o chão e as paredes no lugar, recortado e ajustado para casar com a grade, **e** o mestre pode pô-lo como imagem do mapa sem apagar as camadas nem o que os jogadores já viram.
- **Dado** um mapa, **quando** o mestre pede "Vista isométrica", **então** o app gera a arte do mapa em perspectiva isométrica a partir do que os jogadores veem agora, **e** ela vai para a galeria como arte da cena. O desenho de referência leva só os quadrados que algum personagem vê, só as criaturas que os jogadores veem e a porta secreta não revelada como parede; as outras salas e os inimigos escondidos ficam de fora. Num mapa sem névoa, vai o mapa todo, menos o que está escondido dos jogadores.
- **Dado** um mapa com tokens, um combate ou NPCs em cena, **quando** o mestre pede uma imagem, **então** ele escolhe quais NPCs e inimigos aparecem nela (os retratos da galeria vão como referência) **e** só esses aparecem; ele também pode escolher outras imagens da galeria como referência. Numa imagem feita a partir de um mapa, a escolha só traz os NPCs e inimigos que os jogadores veem agora.
- **Dado** um pedido de imagem, **quando** o app o envia ao serviço de IA, **então** vão só o desenho do mapa, a lista das salas (só no mapa com textura de uma masmorra gerada), o texto do mestre e as referências que ele escolheu, nunca dado pessoal (RN-28).
- **Dado** um mapa com uma porta secreta ainda não revelada, **quando** o app monta o desenho de referência do mapa com textura, **então** a porta vai como parede, **e** a imagem não mostra a passagem (RN-10, RN-26).
- **Dado** uma arte da cena ou uma vista isométrica feita a partir de um mapa, **quando** o app monta o desenho de referência, **então** ele leva só o que os jogadores veem agora (os quadrados, as criaturas, a porta secreta como parede), **e** a imagem não mostra o que eles ainda não descobriram. A ideia é a imersão: a imagem mostra a cena como o grupo a vê.
- **Dado** que o serviço recusa um pedido ou não devolve imagem, **quando** o mestre espera a imagem, **então** o app diz isso em português ("O serviço não gerou esta imagem"), sem gastar a vaga do mês.

#### No app
- **Serviço.** `ImageGenerationService`: `GetImageGenerationStatus`, `GenerateSceneImage`, `EditGeneratedImage`, `GetImageGeneration` (espera longa), `CancelImageGeneration`, `ListImageEdits` e, para as imagens feitas de um mapa, `GetMapImageReference` (o desenho de referência pequeno e os NPCs que o mestre pode marcar, antes de pedir), `GenerateMapImage` e `UseGeneratedImageAsMapImage` ("Usar como imagem do mapa"). O gerador fica atrás de uma interface pequena (`maps/images/gen`: o Gemini, sobre `net/http`, e um falso). Tabelas: `image_requests`, mais `gallery_images.generated` e `parent_image_id`. Ver [Arquitetura](../../architecture.md#ai-generated-images-mr-039-rn-28-adr-0019) e [imagens feitas de um mapa](../../architecture.md#images-made-from-a-map-mr-039-rn-28).
- **Três jeitos, todos no MVP.** A arte da cena, a vista isométrica de um mapa e o mapa com textura, que casa com a grade. O modelo devolve uma de 10 proporções fixas (1:1, 3:2, 2:3, 3:4, 4:3, 4:5, 5:4, 9:16, 16:9 e 21:9, de 512 px a 4K). Por isso o servidor manda o desenho do mapa (o chão e as paredes) na proporção mais próxima da grade, completado com rocha, recorta o mapa do resultado e o ajusta ao tamanho da grade. As camadas continuam sendo a verdade do jogo: se a imagem escorregar um pouco de uma parede, o mestre gera de novo. Cada pedido aceita até 10 imagens de objetos e 4 de personagens como referência.
- **A vista dos jogadores.** A arte da cena e a vista isométrica partem do que os jogadores veem agora: a união do que os personagens deles veem (a conta da névoa, sem a memória), as criaturas que eles veem e a porta secreta não revelada como parede; num mapa sem névoa, o mapa todo menos o que está escondido. O mapa com textura parte do mapa inteiro (a porta secreta como parede), porque a névoa já o esconde de cada jogador; é completado com rocha até a proporção do modelo mais próxima, recortado de volta e ajustado ao tamanho da imagem do mapa. A lista das salas vai só para o mapa com textura de uma masmorra gerada.
- **NPCs.** Na arte da cena e na vista isométrica só aparecem os NPCs que os jogadores veem (o servidor recusa os outros e o retrato de um NPC do mapa que eles não veem, também em `object_image_ids`), com o retrato como referência. O mapa com textura não leva criatura nem imagem de personagem (a imagem vira o fundo do mapa, e uma criatura pintada nela não é uma criatura do mapa): "Quem aparece na imagem" some nesse jeito, e o retrato de qualquer NPC do mapa é recusado como imagem de objeto aí (`object_image_ids`). Nenhum NPC é oferecido enquanto um combate roda no mapa (os combatentes não são tokens do mapa).
- **Limites.** Um mapa cuja imagem passa de 16 megapixels (4.000 × 4.000 px, qualquer grade: 200 × 400 quadrados cabem) não vira mapa com textura, e a tela sabe disso antes (`texture_too_large`). O limite do mês é por campanha (padrão 20; ver [Operação](../../operations.md#generated-images-the-gemini-api)).
- **Imagens do mapa inteiro.** Uma imagem que mostra o mapa inteiro (o mapa com textura e todo ajuste dele) vem marcada pelo servidor (`GalleryImage.shows_whole_map`, a coluna `generated_kind` de `gallery_images`, herdada pela cadeia). A tela nunca a oferece aos jogadores com um toque, e na galeria e no seletor de imagens da sessão pergunta antes ("Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram.", RN-10). "Usar como imagem do mapa" aceita um ajuste de um mapa com textura (feito no tamanho do mapa, com as mesmas conferências do original); o ajuste pede a proporção mais próxima da imagem do mapa e é cortado no meio, nunca esticado. A migration 00161 cobre uma cadeia de qualquer tamanho.
- **Nomes.** A imagem tem um nome que os jogadores entendem: o do mestre (`name` nos pedidos) ou, vazio, o do mapa e o jeito (só de um mapa que os jogadores veem), o jeito e o dia ("Arte da cena · 06/10") ou o da imagem ajustada com " (ajuste)". Nunca "Imagem N", e nunca o texto do pedido nem o nome de um mapa ou ponto escondido (RN-10: o jogador a quem a imagem é mostrada lê o nome).
- **Idempotência.** A chave contra repetição só muda depois de uma resposta ou de uma mudança no formulário, e o app repete uma vez, com a mesma chave, a resposta perdida.
- **Telas.** O diálogo "Gerar imagem" (folha de baixo no celular), aberto do mapa, de um ponto de cena e da galeria: os três jeitos (os que não cabem ali ficam tracejados com a razão), o desenho que o servidor monta do que os jogadores veem, "Quem aparece na imagem" só com os NPCs que eles veem, as referências da galeria, o que vai ao Google embaixo de cada campo e a conta do mês; a espera com `GetImageGeneration` longo, "Cancelar" e "Parar de esperar"; o resultado com "Mostrar aos jogadores" (o fluxo da [MR-028](#mr-028-mostrar-uma-imagem-aos-jogadores)), "Pedir um ajuste" e a cadeia, e "Usar como imagem do mapa" perguntado no lugar; as falhas pelo detalhe tipado. O seletor de imagens da sessão ("Mostrar uma imagem aos jogadores") marca "Mapa inteiro". Ver [Design](../../design.md#ai-generated-images-mr-039-e10-07).
- **Testes (Go).** `TestMR039_GenerateAndEditAScene`, `TestRN10_AGeneratedImageIsHiddenUntilShown`, `TestMR039_TheMonthlyCap`, `TestMR039_FailuresGiveTheSlotBack`, `TestMR039_OnlyTheMasterGenerates`, `TestRN28_NothingPersonalGoesToTheModel`, `TestMR039_TheSameKeyGeneratesOnce`, `TestMR039_CancelBeforeAndAfterTheRequestLeaves`, `TestMR039_TheLastSlotGoesToOneRequest`, `TestMR039_TheSameKeyAtTheSameTime`, `TestMR039_TheLongPoll`, `TestMR039_APictureThatArrivesAfterTheExpiryIsStoredOnce`, `TestMR039_AnUnsentStaleRequestRefunds`, `TestMR039_Shutdown`, `TestMR039_AParentDeletedDuringAnEdit`, `TestMR039_ARefusedKeyIsNotARefusal`, `TestMR039_RequestsInFlightCountAgainstTheGallery`, `TestMR039_TheReferencesAreShrunkAndTheRequestIsCapped`, `TestMR039_GenerationOffWithoutAKey`, `TestMR039_TheRequestIsChecked`, `TestMR039_AFullGalleryRefuses`. Para os jeitos do mapa: `TestMR039_ThePlayersViewOfTheCave`, `TestMR039_ASecretDoorIsAWallInThePlayersView`, `TestMR039_AMapWithoutFogIsShownWhole`, `TestMR039_NoNPCIsOfferedWhileACombatRuns`, `TestMR039_NobodyOnTheMap`, `TestMR039_TheSceneArtAndTheIsometricViewOfAMap`, `TestMR039_OnlyNPCsThePlayersSeeCanAppear`, `TestMR039_TheTexturedMapIsPaddedAndCroppedBack`, `TestMR039_ADungeonsTexturedMapSendsItsRooms`, `TestMR039_UseTheTexturedMapAsTheMapImage`, `TestMR039_UseIsRefusedWhenTheMapChanged`, `TestMR039_UseIsRefusedWhenTheWallsChanged`, `TestMR039_UseAfterAndRacingRedraw`, `TestMR039_UseGivesAFogMapACopyOfAPictureUsedElsewhere`, `TestMR039_TheMapRPCsAreTheMastersOnly`, `TestRN10_NothingOfTheMapPicturesReachesAPlayer`, `TestMR039_TheCapAndTheLongPollForMapKinds`, `TestMR039_AFailedMapRequestGivesTheSlotBack`, `TestMR039_TheSameKeyMakesOneMapPicture`, `TestMR039_TheTexturedMapTakesNoCharacters`, `TestMR039_AHiddenNPCsPortraitStaysOut`, `TestMR039_ASightThatSeesNothingIsNobody`, `TestMR039_AMapTooLargeForATextureIsSaidInAdvance`, `TestRN10_AMapWithoutFogDrawsNoHiddenToken`, `TestRN10_TheGalleryTellsWhichImagesShowTheWholeMap`, `TestMR039_UseAnEditOfATexturedMap`, `TestMR039_TheImageHasAMeaningfulName`, `TestMR039_AHiddenNPCsPortraitIsRefusedAsAnObject`, `TestMigrationsBackfillAChainOfEditsOfAnyDepth`. Pacotes puros: `TestCropByFractionsOfTheDrawing`, `TestFitToMapNeverStretches` e `TestCenterCrop` (`refimg`, `images`), `TestCropFitRefusesAnAnswerThatBreaksTheMemoryBudget` (`images`) e, em `gen` (sem banco), `TestBuildBody`, `TestParseAnswer`, `TestGemini*`, `TestFake`; a chave nunca impressa: `TestLoadImages`, `TestGeminiErrorsNeverCarryTheKey`.
- **Testes (navegador).** `e2e/tests/images.spec.ts` (`@MR-039 @RN-28 @RN-10`: gerar a arte da cena de um mapa, ajustar, mostrar, e o jogador só a vê depois; o mapa com textura vira a imagem do mapa e as camadas ficam; só os NPCs que os jogadores veem; o limite do mês; geração desligada; o seletor da sessão marca o mapa inteiro), mais o Vitest `imagegen-form`, `imagegen-errors`, `imagegen-run`, `image-generate-dialog` e `image-picker-dialog`.

#### Relacionadas
- Usa a API do Gemini (o modelo de imagem, o "Nano Banana"), com uma chave do Google AI Studio, pelo servidor, atrás de uma interface pequena (ADR-0019, que substitui a ADR-0014, que usava o Vertex AI). O modelo é configurável: `gemini-3.1-flash-image` por padrão. O operador novo, o Google (API do Gemini), entra em [Privacidade](../privacidade.md#o-que-as-imagens-geradas-mandam-ao-google-mr-039-rn-28); a chave é um segredo, e o custo e o limite ficam em [Operação](../../operations.md#generated-images-the-gemini-api).
- Em aberto: o número do limite por campanha por mês (proposta: 20) sai de vez depois de medir o custo com a chave de verdade (US$ 0,067 por imagem de 1K no `gemini-3.1-flash-image`).

### MR-040: Subir de nível pela ficha

**Como** jogador, quando meu personagem pode subir de nível, **quero** editar a ficha para acrescentar só o que o próximo nível dá, **para** não esperar o mestre aplicar o nível por mim.

- Prioridade: MVP
- Regras: RN-01, RN-12
- Módulos: characters, rules

#### Critérios de aceite
- **Dado** um personagem que "Pode subir de nível" (RN-12), **quando** o jogador abre a ficha travada, **então** pode editá-la; sem essa marca, a ficha continua só para leitura (RN-01).
- **Dado** a edição guiada, **quando** o jogador a usa, **então** soma um nível de classe e só as escolhas desse nível, como os PV, as habilidades, as magias e o incremento no valor de habilidade.
- **Dado** a edição guiada, **quando** o jogador tenta mudar qualquer outra coisa da ficha, **então** o servidor recusa: o resto continua travado.
- **Dado** uma edição que as regras do D&D 5e não permitem, **quando** o jogador a envia, **então** o motor de regras a recusa.
- **Dado** o dado de vida de um nível, **quando** o jogador rola no app, **então** o servidor rola e guarda o resultado, e rolar de novo devolve o mesmo; numa campanha que força dado físico, o app não rola e o jogador digita o resultado (RN-18).
- **Dado** um personagem com os PV atuais definidos, **quando** a subida de nível aumenta o máximo, **então** o atual sobe o mesmo tanto: um ferimento continua um ferimento e quem estava no máximo continua no máximo; quem nunca teve os PV definidos continua cheio (RN-12).
- **Dado** que o jogador subiu de nível, **quando** o mestre abre a lista de personagens, **então** é avisado e vê "O que mudou": as escolhas do jogador naquele nível (habilidade, PV e como, truques, magias, preparadas), com a hora. Não há aprovação nem veto: o mestre continua editando a ficha como sempre (RN-02).
- **Dado** que o jogador confirmou a subida, **quando** a ficha é gravada, **então** "Pode subir de nível" some sozinho.

#### No app
- **Espaços do pacto.** Entre o que o nível dá sozinho (o passo Vida), o nível de um Bruxo lista "Espaços do pacto" (por exemplo "1 de 1º círculo → 2 de 1º círculo") quando muda o número ou o círculo dos espaços do pacto.
- **Servidor.** O `rules` tem `LevelUpOptions` (o que o próximo nível de uma classe dá), `ApplyLevelUp` (a ficha que as escolhas fazem) e `CheckLevelUp` (recusa tudo o que o nível não permite, com o campo e um motivo). O `CharacterService` tem `GetLevelUpOptions`, `PreviewLevelUp` (o "Resumo": a ficha derivada de depois, feita pelo servidor), `RollLevelUpHitPoints`, `LevelUpCharacter` e `ListLevelUps` (o "O que mudou" do mestre). Tabelas: `character_level_ups` (o registro) e `character_level_up_rolls` (o dado guardado). Ver [Arquitetura](../../architecture.md#leveling-up-from-the-sheet-mr-040) e [RN-01](regras.md).
- **Alcance.** O servidor cobre os PV, o incremento no valor de habilidade, as magias (truques, conhecidas ou do grimório, e as preparadas), a subclasse, as opções das features do nível e as novas invocações do Bruxo. Numa ficha com duas ou mais classes, o primeiro painel pergunta qual delas ganha o nível ("Qual classe sobe de nível?": a primeira classe vem marcada, como o servidor faz com `class_key` vazio, e trocar depois de escolher pergunta "Trocar de classe?"); um dado já rolado para outra classe aparece no cartão do dado com o motivo. O que fica de fora (uma classe nova, subclasse própria, a Arcana Mística do Bruxo, a Maestria de Magia e a Magia Característica do Mago, os Segredos Mágicos Adicionais do Colégio do Conhecimento, o Inimigo Favorito e os terrenos favoritos do Patrulheiro) o mestre faz no editor (`LevelUpOptions.master_adds` lista tudo isso). A tela completa de subir de nível continua na [MR-017](#mr-017-subir-de-nível), depois do MVP. O mestre não veta uma subida: é avisado, vê o que mudou e corrige a ficha se quiser.
- **A ficha.** Na ficha travada de quem "Pode subir de nível", só o dono vê o bloco "Pensantus pode subir de nível" (o motivo, "O mestre marcou “Chegar ao Vale Seco”." ou "Você chegou a 2.700 XP.", e o botão cheio "Subir para o nível 4"). O botão abre a página `/campaigns/:id/characters/:id/level-up`, com os passos Habilidades, Vida, Magias e Resumo. O passo sem escolha não existe (Toren no nível 5 só tem Vida e Resumo); "Escolhas", para a subclasse, as opções das features, as perícias e a especialização, entra antes de Magias nos níveis que as dão. Nada é guardado até "Confirmar o nível 4"; "Cancelar" e "Voltar para a ficha" perguntam no lugar antes de descartar.
- **Números e dado.** Os números do "O que muda" vêm do `PreviewLevelUp` (o navegador não calcula regra). O dado de vida usa o mesmo seletor de rolagem do combate e da cena (RN-18). Uma revisão velha (`aborted`) ou uma recusa do motor aparecem no lugar, com o motivo. "Ler a ficha de novo" guarda só as escolhas que ainda cabem no que o servidor pede agora. A regra de pontos de vida da mesa decide o cartão (mesa que só permite a média não tem dado). Uma rolagem feita no app só fica quando é a que o servidor guardou para esta classe e este nível, e uma digitada só para o mesmo nível e um dado em que caiba. As magias preparadas e a especialização voltam para as contagens novas, para a tela nunca mandar uma escolha que o nível não permite mais. O resumo lê a conjuração da classe que sobe de nível, não a primeira da ficha. Ele também lista os ataques cujo acerto ou dano o nível muda (um aumento de Força leva o "+5 · 1d8+3" de uma arma a "+7 · 1d8+5"; um truque rola mais dados), uma linha para cada, com o tipo de dano, a partir dos ataques das duas fichas derivadas.
- **Depois de confirmar.** A ficha mostra o nível novo e "Pensantus subiu para o nível 4. O mestre foi avisado.", e a etiqueta some. O mestre vê "Subiu para o nível N" e um aviso no alto da campanha (por 24 horas, ou até dispensar; a campanha aberta com sessão lê de novo no `xp_changed`), e "O que mudou" abre as escolhas no lugar, com a hora. Ver [Design](../../design.md#level-up-mr-040).
- **Testes (Go).** `TestMR040_ThePlayerLevelsUpALockedSheet`, `TestMR040_OnlyWhenTheCharacterCanLevelUp`, `TestMR040_TheRestStaysLocked`, `TestLevelUpRefusals`, `TestMR040_TheRulesRefuseWhatTheLevelDoesNotGive`, `TestLevelUpRefusesBadSpells`, `TestLevelUpPensantus` (Mago 3 para 4), `TestMR040_TheHitPointRollIsKept`, `TestMR040_ATypedPhysicalDie`, `TestMR040_TheMasterSeesWhatChanged`, `TestMR040_CanLevelUpClearsAfterwards` (no `progression`, por XP e por marcos), `TestLevelUpSweep` (as 12 classes de 1 a 20, com cada subclasse do SRD: as opções e a checagem concordam), `TestLevelUpOptionsForTheSubclassLevel`, `TestLevelUpGainsTheEngineModels`, `TestLevelUpRefusesDuplicates`, `TestMR040_OneRollPerLevelNotPerClass`, `TestMR040_DuplicatesAreInvalid`, `TestMR040_AStoredSheetThatFailsToday`, `TestMR040_ListLevelUpsPages`, `TestMR040_StaleRevision`, `TestMR040_TheCurrentHitPointsRiseWithTheMaximum`, `TestMR040_ACharacterWithoutVitalsGetsNone`, `TestMR040_TheMastersEditMovesTheCurrentHitPointsWithTheMaximum`, `TestMR040_ADeadCharacterDoesNotLevelUp`, `TestMR040_AFighterWithNothingToChoose` (Toren) e as linhas novas do `TestAuthorizationMatrix`.
- **Testes (navegador).** `e2e/tests/levelup.spec.ts` (`@MR-040 @RN-01 @RN-12`: "Pensantus sobe do Mago 3 para o 4…", com o mestre vendo; "Toren sobe do Guerreiro 4 para o 5…", só Vida e Resumo e dado rolado no app; "quem não pode subir de nível…": sem a marca não há botão e a rota responde como a de uma ficha travada), mais o Vitest dos passos, das escolhas, da Constituição, do dado (média, rolado, digitado, a regra de dados), dos números, dos erros, do aviso e do "O que mudou" do mestre, do Bardo (colégio e Especialização), do clérigo (prepara da lista da classe), da subclasse que dá truque e perícia, dos Segredos Mágicos e do bloco da ficha; as telas estão no `a11y.spec.ts` (`scanLevelUpScreens`).

#### Relacionadas
- [MR-017](#mr-017-subir-de-nível), [RN-01](regras.md), [RN-12](regras.md).

### MR-041: Tesouros e XP por ouro

**Como** mestre, **quero** pôr tesouros no mapa antes da sessão e converter em XP o que o grupo achou, **para** dar XP por ouro sem fazer a conta na hora.

- Prioridade: MVP
- Regras: RN-09
- Módulos: maps, progression

#### Critérios de aceite
- **Dado** um mapa, **quando** o mestre põe um tesouro (um baú, por exemplo) com um valor em PO, **então** o tesouro fica guardado no mapa, com o valor em PO.
- **Dado** um tesouro que o grupo achou, **quando** o mestre o marca como encontrado, **então** ele passa a contar como achado e ainda não convertido.
- **Dado** tesouros encontrados, **quando** o mestre usa "Voltar à cidade", **então** o app os converte em XP, 1 XP por PO (RN-09), dividido como o mestre escolher, e cada tesouro só é convertido uma vez.

#### No app
- **O ponto de tesouro.** O ponto do tipo `TREASURE` guarda o valor em PO (inteiro, de 0 a 1.000.000) e a descrição, escondido como qualquer ponto. O mestre o marca como achado por um ou mais personagens (`MarkTreasureFound`, e `UnmarkTreasureFound` para tirar a marca): a partir daí todos que veem o mapa o veem, com a descrição, o valor e quem achou. Com a sessão aberta, o tesouro guarda a sessão (o resumo dela o conta), e `treasure_found` e `treasure_unfound` entram no histórico, só com IDs e as PO; fora de uma sessão ele não conta no resumo de nenhuma. Com o tesouro convertido por um prêmio, o valor, a marca e o tipo ficam travados, e ele não se apaga antes de desmarcar (o servidor recusa os dois, `MapBlocked`).
- **O editor.** "Tesouro" na barra de pontos põe o ponto e abre o painel: o nome, a "Descrição para os jogadores" (o que tem dentro, que eles leem quando o tesouro é achado) e o "Valor em ouro" (PO inteiros, de 0 a 1.000.000). Em "Encontrado", "Marcar como encontrado" abre no lugar a mesma escolha da sessão (quem encontrou, ninguém marcado, ao menos um) e diz sob o botão que **marcado fora de uma sessão o tesouro não entra em resumo nenhum** (com uma sessão aberta, "Marcado durante a Sessão N, o tesouro entra no resumo dela."). Achado, mostra por quem e quando, e "Desmarcar" pergunta no lugar com o foco em "Voltar". Convertido em XP, os campos ficam travados e "Desmarcar" e "Apagar ponto" viram o botão tracejado que não age, com o motivo escrito. A lista "Pontos do mapa" separa uma armadilha e um baú do mesmo quadrado.
- **A página da sessão.** O mestre vê "Tesouros do mapa": um cartão por tesouro (escondido, achado ou convertido em XP), com o que tem dentro (só dele até achar). "Marcar como encontrado" abre o formulário no próprio cartão: "Quem encontrou", a linha do que entra no resumo (o total em PO, que o servidor divide; a tela não faz a conta). Depois, "Encontrado por Brisa às 21:40"; "Desmarcar" pergunta no lugar; convertido, o cartão mostra o cadeado e o caminho para desfazer. O jogador vê o baú no mapa (marca cheia), a linha "Baú de moedas, Tesouro, encontrado por Brisa" com a folha do valor e do conteúdo, e o aviso "Brisa encontrou o Baú de moedas.".
- **"Voltar à cidade" (servidor).** `ProgressionService.ListTreasuresToConvert(campaign_id)` (`IDEMPOTENT`, só o mestre) lista os tesouros achados que nenhum prêmio converteu, do achado mais antigo ao mais novo, com o mapa, o nome, as PO, quem achou, quando e se foi achado com uma sessão aberta. Traz no máximo 100 (os mais antigos) e o `total`. "Voltar à cidade" é **um `AwardXP` de modo `GOLD`** com `treasure_point_ids` no lugar de `gold`: o servidor soma as PO dos tesouros (1 XP por PO), divide entre os personagens escolhidos como em todo prêmio (partes iguais, arredondadas para baixo; o resto some e vem em `lost_xp`) e, na mesma transação, liga cada tesouro ao prêmio, travando as linhas. Dois prêmios ao mesmo tempo pelo mesmo tesouro convertem uma vez só, e o outro recebe `TREASURE_ALREADY_CONVERTED`. Quem recebe é quem o mestre mandar em `character_ids` (a tela marca todos os vivos), sem relação com quem achou: "achado por" é só do destaque. "Dar XP por ouro" com as PO digitadas continua como antes.
- **Histórico e privacidade.** A resposta, o histórico (`XPAward.treasures`, só para o mestre: o ID e as PO de cada tesouro, que continuam no prêmio mesmo depois de desfeito; o jogador recebe só `treasure_count` e o `gold`, RN-10) e o evento `xp_awarded` (só IDs e PO) levam os tesouros convertidos ("Voltar à cidade: 3 tesouros, 420 PO").
- **Recusas.** Tesouro de outra campanha ou que não é tesouro (um ponto de cena, por exemplo): `not_found`. Ainda não achado: `TREASURE_NOT_FOUND_YET`. Já convertido: `TREASURE_ALREADY_CONVERTED` (o detalhe traz o ID). Somando mais de 1.000.000 PO: `TREASURES_OVER_LIMIT`. Sem PO nenhuma: `NOTHING_TO_GIVE`. `treasure_point_ids` junto de `gold`, fora do modo `GOLD`, repetido ou com mais de 100: `invalid_argument`. Campanha por inimigos ou por marcos: `MODE_NOT_ALLOWED` (o tesouro dela continua no mapa e no resumo, sem conversão).
- **Desfazer.** `UndoLastXPAward` desfaz a conversão se ela é o último prêmio, e os tesouros voltam a "achado, não convertido" na mesma transação (podem ser convertidos de novo). Depois de um prêmio mais novo, a conversão não é mais a última e os tesouros continuam convertidos até esse prêmio ser desfeito (o desfazer com `expected_award_id` da conversão responde `aborted`): quem quiser corrigir antes, corrige à mão.
- **A página da campanha, campanha por ouro.** O painel "Experiência" do mestre ganha a faixa "Encontrado, ainda não convertido" ("3 tesouros · 420 PO", e uma linha por tesouro: "Baú de moedas, 250 PO, de Brisa"; sem nenhum, "Nenhum tesouro esperando. Os que o grupo encontrar aparecem aqui.") e "Voltar à cidade" ao lado de "Dar XP", os dois contornados do mesmo tamanho (empilhados, de largura toda, no celular). A faixa lista no máximo 5 tesouros e conta o resto ("e mais 2"). "Dar XP", em campanha por ouro, abre com a mesma faixa e o botão "Voltar à cidade", e "ou digite o ouro" separa as duas formas; apertar "Voltar à cidade" ali fecha o "Dar XP" e abre a conversão (uma folha de cada vez no celular).
- **O diálogo.** "Voltar à cidade" abre o diálogo de 600 px (folha de baixo no celular): "Tesouros para converter" (todos marcados, cada um com o nome, "Encontrado por Brisa às 21:40" e as PO; um achado fora de uma sessão leva a etiqueta "fora de uma sessão", e uma linha só acima da lista diz que ele não conta em nenhum resumo de sessão), "Quem recebe" (todos os vivos marcados) e a conta escrita, ao vivo: "420 PO em 3 tesouros = 420 XP", "420 XP ÷ 4 = 105 XP para cada" e "Sobra 0 XP." (ou "Sobra 1 XP, que não vai para ninguém."). O rodapé, que não rola, tem a conta, "Para 3: Pensantus, Toren e Brisa" e o botão cheio, que diz o número ("Dar 105 XP para cada"). Enquanto a lista não é lida o diálogo diz "Lendo os tesouros encontrados..."; "Nenhum tesouro" só vale depois de uma leitura que deu certo (se não deu, "Tentar de novo"). Sem tesouro ou sem personagem marcado, a caixa diz "Marque pelo menos um tesouro e um personagem." e o botão fica tracejado. A conta da tela é só a prévia: o servidor soma as PO e divide, e quem vale é a resposta (`xp_each`, `lost_xp`). Com mais de 100 tesouros esperando: "Há mais N tesouros encontrados, que ficam para a próxima vez".
- **Erros e depois.** Os erros ficam no diálogo, pelo motivo do `XPBlocked` (`TREASURE_ALREADY_CONVERTED`, `TREASURE_NOT_FOUND_YET`, `TREASURES_OVER_LIMIT`, `MODE_NOT_ALLOWED`); a lista é lida de novo, o que saiu some e o resto da escolha fica. Depois de converter: "Voltar à cidade: Pensantus, Toren e Brisa receberam 140 XP cada. Os 3 tesouros foram convertidos.". O motivo do prêmio é "Voltar à cidade"; a linha do histórico se escreve dos tesouros ("Voltar à cidade · 420 PO em 3 tesouros"; `treasure_count` e `gold`, igual para o jogador, que nunca vê quais). "Desfazer", só no último prêmio, pergunta no lugar ("Os 3 tesouros (420 PO) voltam a “encontrado, não convertido”."); um "Voltar à cidade" que já não é o último diz, só para o mestre, que os tesouros ficam livres quando os prêmios mais novos forem desfeitos. Em campanha por inimigos não há botão: a faixa só existe quando há um tesouro achado, chama-se "Tesouro encontrado" e diz "Esta campanha dá XP por inimigos, então o tesouro não vira XP. Ele aparece no resumo de cada sessão."; em campanha por marcos não há faixa. O jogador vê só o histórico (a linha, o XP de cada um), sem faixa e sem botões.
- **Testes (Go).** `TestMR041_TreasureFound`, `TestMR035_PointKindsAreValidated` (o valor), `TestRN10_PlayersNeverReceiveTrapsLightsOrHiddenTreasure`, `TestMR041_VoltarACidadeConvertsTreasuresIntoOneGoldAward` (420 PO em 3 tesouros, 140 XP para cada um de 3, o histórico, o evento, a repetição da chave), `TestMR041_ATreasureIsConvertedOnce` (uma transação segura as linhas travadas enquanto dois prêmios começam: nenhum termina antes de ela soltar, e depois um vence), `TestMR041_ADoubleSubmitConvertsOnce`, `TestMR041_TheListStopsAtWhatOneConversionTakes`, `TestMR041_UndoFreesTheTreasures`, `TestMR041_VoltarACidadeRefusesWhatDoesNotFit`, `TestMR041_OnlyTheMasterReadsTheTreasuresToConvert` e a linha `ListTreasuresToConvert` do `TestAuthorizationMatrix`.
- **Testes (navegador).** Vitest `town-sheet`, `treasure-strip`, `treasure`, `xp-errors`, `award-history`, `experience-panel`, `experience-store`, `xp-give-button`, `award-xp-sheet`, `treasure-card`, `treasure-point-panel`, `treasure-draft`. Playwright `gold.spec.ts` (`@MR-041 @MR-032`: três tesouros viram um prêmio só, de 420 XP para o Pensantus, a linha do histórico, o desfazer, o que o jogador lê, o tesouro convertido na corrida, o "Dar XP" da página e o da sessão ao vivo, a campanha por inimigos, com a recusa `MODE_NOT_ALLOWED` do servidor, e o resumo; a divisão entre vários, com sobra, é do Vitest `town-sheet` e `treasure`, que conferem a prévia com os números do `TestSplitXP`, porque a suíte tem um jogador só, RN-03), `traps.spec.ts` (`@MR-041`), `map-editor.spec.ts` (`@MR-041`: o tesouro fora de uma sessão, quem achou, o jogador só o vê achado, "Desmarcar" e o apagar) e as telas em `a11y.spec.ts`.

#### Relacionadas
- O destaque "Mais tesouro encontrado" está na [MR-032](#mr-032-destaques-do-combate). O mestre também pode digitar as PO em "Dar XP por ouro" ([MR-016](#mr-016-dar-xp)), que fica como alternativa. Armadilhas e baús: [MR-035](#mr-035-armadilhas). O ouro só vira XP em campanha por ouro; nas outras o tesouro conta no resumo da sessão. Ver [Arquitetura](../../architecture.md#layers-fog-and-point-kinds).

### MR-042: Bestiário

**Como** mestre, **quero** procurar as criaturas do SRD e pôr uma no combate, **para** montar um encontro sem fazer a ficha de cada inimigo.

- Prioridade: MVP
- Regras: RN-20, RN-29
- Módulos: rules, play, characters

#### Critérios de aceite
- **Dado** que sou mestre de "Mirathel", **quando** abro o "Bestiário", **então** vejo as 334 criaturas do SRD com o nome em português, e filtro pelo nome, pelo tipo, pelo tamanho e pelo ND.
- **Dado** uma criatura do bestiário, **quando** a abro, **então** vejo a ficha dela (CA, PV, deslocamento, habilidades, ataques e ações), com o texto do SRD em inglês.
- **Dado** um combate em preparação, **quando** ponho três Goblins do bestiário, **então** eles entram como NPCs escondidos, cada um com a própria iniciativa (RN-19), os PV médios, ou rolados se eu pedir, **e** o jogador vê só a palavra do estado deles (RN-20, RN-29).
- **Dado** um combate com monstros do bestiário, **quando** ele termina, **então** o XP por inimigos conta o ND de cada um, como o de um NPC.
- **Dado** uma criatura do bestiário, **quando** o mestre escolhe "Criar NPC", **então** o app faz uma ficha básica de NPC com os números dela, que o mestre pode renomear e editar.

#### No app
- **Leituras.** `ContentService.ListCreatures` filtra por **tamanho** (`size`, os seis do SRD), por **ND mínimo** (`min_cr`, ao lado do `max_cr`; o mínimo não pode passar do máximo), por tipo, ND e nome. Cada linha traz também a CA e os PV médios ("Lobo · Wolf · SRD", "CA 13 · PV 11"). A busca acha o nome em português ou em inglês: "lobo" acha a Aranha-lobo gigante pelo português, e "wolf" acha também as formas do Lobisomem. O `page_size` vai até 400, para o bestiário trazer as 334 de uma vez. O jogador pode ler o bestiário (é regra do SRD), mas o app o mostra só ao mestre. Os rótulos de tipo e de tamanho em português vêm do servidor (`type_pt`, `size_pt`).
- **"Criar NPC".** `CharacterService.CreateNpcFromCreature` faz o NPC de uma criatura: só o mestre, tipo **Minion (o padrão) ou NPC de história** (MINION e STORY: a ficha básica não serve ao "Inimigo" e ao Boss, que têm ficha completa; o app oferece só esses dois), com o nome do mestre e uma ficha básica com a CA, os PV médios, o deslocamento, as habilidades, a iniciativa (o modificador de Destreza), o tamanho, o ND, o XP e até três ataques da criatura (de arma ou de magia, com o primeiro dano; as outras partes do dano, como o fogo a mais da mordida do dragão, vão na descrição da ficha como "Mordida: +2d6 fogo."; resistência e efeitos ficam na ficha do SRD), com os nomes em português, e o `monster_key` que liga à ficha do SRD. É idempotente pela chave do diálogo. O NPC é uma cópia: o bestiário não muda e o NPC se edita como qualquer outro. Como todo NPC, o jogador não o lista nem o lê (RN-04), e o token dele nasce escondido (RN-10). Os NPCs que o "Criar NPC" faz seguem o ataque múltiplo da criatura.
- **"Pôr no combate".** `CombatService.AddMonsters` põe de 1 a 10 monstros de uma criatura num combate em preparação ou em andamento: os nomes ("Bandido 1" a "Bandido 3", ou o nome-base do mestre; com Bandidos já no combate, ele numera adiante: "Bandido 4, Bandido 5 e Bandido 6"), os PV médios (o padrão) ou rolados pelo servidor, um por monstro, escondidos por padrão (o pedido pode revelá-los) e a iniciativa rolada pelo servidor para cada um, com o modificador de Destreza da criatura. Cada monstro é um NPC de combate ligado a um NPC escondido da campanha que o app faz sozinho: **um NPC por campanha e criatura**, reaproveitado, nunca na lista do mestre nem como participante. No mapa começa sem quadrado, e no teatro da mente ninguém tem. O jogador lê só a palavra do estado do monstro revelado e nada dos escondidos; os ataques passam por `RollAttack` (o crítico segue a regra da mesa); o XP por inimigos conta o ND de cada um. A chave de idempotência vale para o pedido todo. A criatura, o ND e os PV rolados são só do mestre (uma linha do registro, também antes do combate); o jogador só sabe o nome que o mestre deu.
- **Ataque múltiplo.** O ataque múltiplo segue o SRD 5.1 (os números errados do snapshot estão corrigidos: Veterano 3, Montículo Movediço 2, Urso Pardo 2, Mandíbulas Tagarelas 1); o app guarda N ataques, quaisquer dos da ficha, e o mestre joga a mistura.
- **A tela do bestiário.** O painel "Bestiário" na página da campanha (só o mestre) leva a `/campaigns/:id/bestiary`: as 334 criaturas com o nome em português e o do SRD em letra pequena, o tipo e o tamanho, o ND e "CA 13 · PV 11", a busca (os dois nomes; a dica sob a lista não depende da palavra), os filtros de tipo, tamanho e ND (faixas ou um nível), a contagem ("5 de 334 criaturas"), o vazio ("Nenhuma criatura com “…”.") e os estados de carregando e de erro. Uma linha abre a ficha (`/campaigns/:id/bestiary/:criatura`) com o texto do SRD em inglês e o SRD creditado. O ícone da criatura segue o tipo (pessoa, figura grande, lobo, aranha, pata, glifo neutro). O jogador não tem o ponto de entrada (RN-04): a página, se aberta pelo endereço, diz que é do mestre.
- **O diálogo "Criar NPC".** O nome do mestre para o NPC, **Minion** ou **NPC de história**, os números e os ataques que vão junto (o servidor os diz, `Creature.npc_attack_names`, pela mesma função que monta a ficha) e a confirmação ("NPC criado: Capitão bandido", os ataques que a ficha recebeu, o caminho para ela). A chave de criação nasce por diálogo e vale para todas as tentativas dele, então um toque duplo faz um NPC só e uma tentativa repetida depois de uma resposta perdida diz "Já foi criado." (a recusa de `idempotency_key` diz o campo no detalhe `InvalidField`).
- **A folha "Pôr no combate".** Está na linha de cada criatura do bestiário e na ficha dela (o botão cheio do painel "Usar esta criatura"; "Criar NPC" fica contornado ao lado). A folha mostra o combate onde os monstros entram ("Emboscada na ponte · em preparação" ou "em andamento"; sem combate aberto o botão vira "Criar o combate e pôr", que começa um com os monstros e o grupo todo; sem sessão, ele espera), "Quantos" (de 1 a 10, e nunca mais do que cabe nos 40 combatentes), a prévia "Entram como Bandido 1, Bandido 2 e Bandido 3.", o nome-base, os PV ("Média (11)" ou "Rolar") e "Escondidos no início" (ligado por padrão). A chave de idempotência nasce por folha aberta e se repete na tentativa com os mesmos parâmetros; mudou a escolha, é outra chave. O que entrou é anunciado sobre a lista ou na ficha, com o caminho para a sessão. O teto de 40 tem detalhe tipado (`EncounterBlocked`, `TOO_MANY_COMBATANTS`), dito em palavras.
- **Na sessão.** A ordem do mestre mostra o ND do monstro ("NPC · ND 1/8 · CA 12"), o cartão do turno leva "Ficha da criatura" (a chave `bestiary_creature_key` só chega ao mestre), o registro tem a linha "Você pôs 3 monstros no combate. Bandido 1: 9 PV (2d8 + 2: 3, 4); …" só para ele, num grupo "Antes do combate" quando veio antes do começo, e o XP do fim do combate lista "Bandido 1 a 3 · ND 1/8 · 25 XP cada · 75 XP" antes da divisão. Ver [Design](../../design.md#monsters-in-combat-and-the-encounter-builder-mr-042-mr-043).
- **Testes (Go).** `TestListCreaturesBestiary` (`rules`); `TestListCreaturesBestiaryRequests`, `TestCreateNpcFromCreature`, `TestNpcSheetFromCreature`, `TestEveryCreatureMakesAnNpc` (`characters`); `TestMR042_ThreeBanditsJoinTheCombat`, `TestMR042_MonstersRollTheirHitPoints`, `TestMR042_AddMonstersRefusals`, `TestMR042_AMonstersCriticalFollowsTheTablesRule`, `TestRN20_PlayersNeverReceiveAMonstersNumbers`, `TestMR042_MonstersGiveXPByTheirChallengeRating`, `TestMR042_MonstersInTheatreHaveNoSquare`; as linhas dos métodos em `TestAuthorizationMatrix`.
- **Testes (navegador).** Vitest `bestiary-list`, `bestiary-creature`, `create-npc-sheet`, `bestiary-format`, `put-sheet`, `monsters` (`core/combat`), `combat-log`, `order-list`, `combat-xp` e o painel em `campaign-detail`. Playwright `bestiary.spec.ts` (`@MR-042 @RN-04 @RN-10`) e `monsters.spec.ts` (`@MR-042 @RN-29 @RN-20 @RN-10`: três Bandidos de uma vez, o ND e o registro do mestre, o que o jogador recebe, os rolados e revelados, o XP 3 × 25 = 75; a divisão por quatro, 18 para cada e sobram 3, é do Vitest, porque a suíte tem um jogador só) e as telas em `a11y.spec.ts`.

#### Relacionadas
- [MR-043](#mr-043-gerar-encontros), [MR-044](#mr-044-gerar-tesouro), [RN-20](regras.md), [RN-29](regras.md). Monstros próprios ficam fora do MVP. Ao pôr no combate, os PV são a média, e o mestre pode rolar.

### MR-043: Gerar encontros

**Como** mestre, **quero** montar um encontro sabendo se ele é fácil ou mortal para o meu grupo, e gerar um quando estiver sem ideia, **para** preparar a sessão mais rápido.

- Prioridade: MVP
- Regras: RN-29
- Módulos: rules, play, maps

#### Critérios de aceite
- **Dado** o grupo de "Mirathel" (os personagens de jogador vivos da campanha, mais os NPCs que eu puser no grupo, com o nível que eu disser), **quando** monto um encontro com criaturas do bestiário e as quantidades, **então** vejo o XP total e a dificuldade (baixa, moderada ou alta) contra o orçamento do grupo, com o rótulo "Guia de dificuldade do SRD 5.2.1 (regras de 2024)" e o aviso de que, com os monstros de 2014, o encontro tende a ficar um pouco mais fácil.
- **Dado** um encontro montado, **quando** o guardo num ponto de batalha do mapa, **então** "Começar este combate" põe os monstros dele no combate, como na [MR-042](#mr-042-bestiário).
- **Dado** uma dificuldade e, se eu quiser, um tipo de criatura, **quando** peço "Gerar encontro", **então** o app monta um com criaturas do SRD, um líder e um grupo, que nunca passa do orçamento nem traz criatura de ND acima do menor nível do grupo mais 3 (os NPCs do grupo contam, com o nível que o mestre deu); "Gerar outro" faz um novo, e posso trocar uma criatura.
- **Dado** a mesma dificuldade, as mesmas opções e a mesma semente, **quando** gero duas vezes, **então** o resultado é o mesmo.

#### No app
- **Quem é o grupo.** Os personagens de jogador vivos da campanha, mais os NPCs que o mestre puser no grupo naquele momento da história. Nunca um morto nem um pendente.
- **Serviço.** O `EncounterService` (`proto/meurpg/play/v1/encounters.proto`, `play/encounters.go`), só do mestre (o jogador, o membro pendente e quem é de fora recebem `not_found`: o montador e o encontro guardado são o segredo do mestre, RN-10). `EvaluateEncounter` mede criaturas e quantidades contra o grupo (os personagens de jogador vivos, mais os `extra_party` com o nível que o mestre dá) e devolve os orçamentos de baixa, moderada e alta, o XP total, a faixa, quanto passa de alta, o ND máximo (o menor nível do grupo mais 3) e os avisos (acima de alta, criatura acima do ND máximo, grupo vazio, gente demais para um combate). `GenerateEncounter` monta um líder e um grupo de uma ou duas criaturas, do SRD, para a dificuldade e o tipo pedidos, e é o mesmo para a mesma semente (sem semente, o servidor sorteia uma e a devolve: "Gerar outro"). `ListEncounterSwaps` lista as criaturas de mesmo XP (e, com tipo, do mesmo tipo) para o "Trocar criatura". `SaveBattleEncounter`, `GetBattleEncounter`, `ClearBattleEncounter` e `ListBattleEncounters` guardam, leem, tiram e listam o encontro de um ponto de batalha (`battle_encounters`, migration `00160`); guardar o mesmo de novo não muda nada.
- **"Começar este combate".** O `StartEncounter` tem `monsters`, `monster_hit_points` e `monsters_hidden` (o app lê o encontro do ponto, deixa o mestre mudar e manda); os monstros entram na mesma transação do início, pelo mesmo código do `AddMonsters`, com a mesma chave de idempotência, também no teatro da mente. Ver [Arquitetura](../../architecture.md#encounter-builder-mr-043).
- **A tabela de orçamento.** É `effects/encounter_budget.json` (revisão `fx.16`), a "XP Budget per Character" do SRD 5.2.1 (CC BY 4.0), com crédito no `NOTICE` e na página "Créditos"; o SRD 5.1 não tem tabela de dificuldade. Se ela combina com os monstros de 2014, a mesa vê depois de jogar.
- **A página.** `campaigns/:id/encounters` (o painel "Encontros" da página da campanha, só do mestre; o jogador lê "Só o mestre monta encontros."). O grupo em chips (os personagens de jogador vivos e os NPCs que o mestre põe, com "Pôr um NPC no grupo": um NPC da campanha ou só um nome, e o nível de 1 a 20, com o orçamento que isso faz pedido ao servidor), a barra das três faixas com o total e a faixa em palavras ("Baixa", "Moderada", "Alta" e "Acima de alta", nunca "mortal"), o rótulo "Guia de dificuldade do SRD 5.2.1 (regras de 2024)" com o link dos Créditos e a ressalva dos monstros de 2014, a linha do ND máximo e os avisos (acima de alta, criatura acima do teto, grupo vazio, demais para um combate, criatura que o SRD perdeu). As criaturas e as quantidades (de 1 a 40; o "−" em 1 tira a criatura) com a busca "Adicionar criatura" pelo nome em português ou do SRD.
- **A conta é toda do servidor.** A cada mudança o `EvaluateEncounter` mede de novo (espera 250 ms, uma conta por vez, resposta velha descartada).
- **Gerar, trocar, guardar.** "Gerar encontro" (dificuldade e tipo; o resultado já vem com a semente em letra pequena; "Gerar outro" sorteia outra; a mesma semente dá o mesmo encontro; "Usar este encontro" leva ao montador), "Trocar criatura" (o mesmo XP, sem as criaturas que o encontro já tem; a quantidade fica) e "Guardar no ponto de batalha" (o mapa, o ponto, "Já guarda um encontro" e a pergunta no lugar antes de trocar). O montador aberto de um ponto (`?point=`) traz o encontro guardado dele para o rascunho, e "Tirar o encontro do ponto" (pergunta no lugar, `ClearBattleEncounter`) o apaga.
- **Na sessão.** O ponto de batalha do mapa atual que guarda um encontro mostra a faixa de hoje e "Começar este combate", que abre o "Iniciar combate" já preenchido (o nome do ponto, os monstros em linhas de leitura, Média ou Rolar e escondidos); ao confirmar, o `StartEncounter` vai com os monstros, o modo dos PV, `hidden`, o ponto (`map_point_id`) e uma chave só. A escolha "Com mapa / Sem mapa (teatro da mente)" do "Iniciar combate" vale aqui: o `CombatClient.start` leva `mode` nos extras e, no teatro, os monstros entram sem quadrado e o combate não tem ponto. **De um ponto de batalha num mapa com grade o diálogo abre em "Com mapa"**, e a regra "combate com mapa" da mesa não o vira para "Sem mapa" (o mestre ainda pode escolher). A confirmação de "Pôr no combate" diz os nomes que o servidor deu. O cartão do ponto mostra o aviso "demais para um combate" que o servidor já mandava.
- **Testes (Go).** `TestMR043_TheBuilderMeasuresAnEncounterAgainstTheParty` (os números do desenho: 1.550 XP Moderada, com o Orin 1.400 / 2.100 / 3.000, 2.900 acima de alta por 300), `TestMR043_TheBuilderRefusals`, `TestMR043_GenerateIsDeterministicAndKeepsItsPromises`, `TestMR043_GenerateRefusals`, `TestMR043_SwapOptionsKeepTheTotal`, `TestMR043_SaveReadAndClearOnABattlePoint`, `TestMR043_BeginningThisCombatPutsTheSavedMonstersIn`, `TestMR043_BeginningThisCombatWorksInTheatreAndWithRolledHitPoints`, `TestMR043_StartingWithMonstersRefusals`, `TestMR043_AStartRetryIsCheckedAgainstTheRequest`, `TestMR043_AStartWithMonstersAndNpcParticipants`, `TestMR043_TheFortyCountsThePartysCreatures`, `TestMR043_AGeneratedEncounterNeverFillsTheCombat`, `TestMR043_ASavedEncounterWithAGoneCreature`, `TestMR043_TheSavedEncounterFollowsItsPoint`, `TestMR043_SavesAtOnceOnOnePoint`, `TestRN10_PlayersNeverGetTheBuilderOrTheSavedEncounter`, `TestMR043_TheBuilderAuthorizationMatrix`; os de regras, sem banco, em `rules/encounter` e `rules/encounters_test.go`.
- **Testes (navegador).** Vitest `encounter-builder`, `budget-bar`, `generate-sheet`, `party-npc-sheet`, `save-sheet`, `battle-encounters`, `start-combat-saved`, `encounter-text`, `encounter-draft`. Playwright `monsters.spec.ts` (`@MR-043 @RN-29 @RN-10`: o montador contra o grupo, o NPC no grupo, a mesma semente por duas vezes, "Trocar", guardar e trocar com a pergunta, "Começar este combate" e o que o jogador nunca recebe) e as telas em `a11y.spec.ts`.

#### Relacionadas
- [MR-042](#mr-042-bestiário), [RN-29](regras.md). O teto de 40 combatentes é um detalhe tipado do servidor (`EncounterBlocked`, `TOO_MANY_COMBATANTS`, no `AddMonsters` e no `StartEncounter`, contando também as criaturas do grupo).

### MR-044: Gerar tesouro

**Como** mestre, **quero** gerar o tesouro de uma criatura ou de um covil, com moedas, gemas, obras de arte e itens mágicos, **para** pôr no mapa sem inventar tudo na hora.

- Prioridade: MVP
- Regras: RN-09, RN-10
- Módulos: rules, maps, progression, characters

#### Critérios de aceite
- **Dado** o nível do grupo, **quando** peço um tesouro "individual" ou "de covil", **então** o app gera as moedas (individual) ou as moedas, as gemas, as obras de arte e os itens mágicos do SRD 5.1 (de covil), de tabelas nossas em português, com o nome em português e o valor em PO.
- **Dado** um item mágico gerado, **quando** o abro, **então** vejo a raridade, o valor com o rótulo "valores do SRD 5.2.1 (regras de 2024)" e a descrição do SRD em inglês.
- **Dado** um tesouro gerado, **quando** o ponho num mapa, **então** ele vira um ponto de tesouro escondido (RN-10) com o ouro (moedas, gemas e arte) em PO e os itens na descrição, que numa campanha por ouro vira XP em "Voltar à cidade" ([MR-041](#mr-041-tesouros-e-xp-por-ouro)).
- **Dado** o mesmo pedido e a mesma semente, **quando** gero duas vezes, **então** o resultado é o mesmo.

#### No app
- **Itens mágicos.** Os 362 itens mágicos do SRD 5.1 estão no conteúdo de regras (`srd51/data/magic-items.json`, nomes em português `item:<índice>`, revisão fx.12), com `Content.MagicItems()`, `MagicItem(chave)` e `MagicItemUnits(raridade)` (o que um tesouro sorteia: cada item avulso e uma unidade por família e raridade). Ver [Arquitetura](../../architecture.md#magic-items).
- **Serviço.** O `TreasureService` (módulo `maps`, só o mestre; o jogador, o membro pendente e quem não é membro recebem `not_found` de tudo) gera o tesouro individual ou de covil de um nível de grupo (por padrão o menor nível dos personagens vivos) e de uma semente: o mesmo pedido e a mesma semente dão o mesmo tesouro. As moedas (PC, PP, PE, PO e PL; 10 PP = 1 PO), as gemas e as obras de arte vêm de tabelas nossas, em português, feitas por critérios nossos (metas de ouro ligadas aos valores do SRD 5.2.1, duas escalas de valor nossas e nomes nossos), em quatro faixas de nível (1 a 4, 5 a 10, 11 a 16 e 17 a 20). Os itens mágicos são os 362 do SRD 5.1, com o nome em português, e **nunca sai um artefato**. O `Treasure` leva o `content_version`, e o `PlaceTreasure` o exige: uma semente só dá o mesmo tesouro dentro de uma versão do conteúdo. Ver [Arquitetura](../../architecture.md#treasure-generator-mr-044).
- **Valores.** O valor de cada item é a tabela do SRD 5.2.1 ("Magic Item Rarities and Values", CC BY 4.0, p. 205, com crédito): comum 100 PO, incomum 400, raro 4.000, muito raro 40.000, lendário 200.000, com o rótulo "Valores do SRD 5.2.1 (regras de 2024)". **Um consumível vale a metade e um Pergaminho de magia vale o valor inteiro da raridade dele** (a nota do SRD 5.2.1 não divide o pergaminho, e os nossos são os de 2014, com a raridade do SRD 5.1 por nível da magia; a tela diz isso). Uma família "Varia" sai como uma variante, com o valor dela. O artefato não tem preço e nunca sai num tesouro. O SRD 5.1 não tem valores nem tabelas de tesouro aleatório, então as tabelas de moedas, gemas e arte são nossas. `GetMagicItem` dá o que "Ver descrição" mostra (raridade, valor, sintonização e o texto do SRD em inglês).
- **"Pôr no mapa".** `PlaceTreasure` gera de novo no servidor, a partir do modo, do nível e da semente (nunca de um total do app), e cria um ponto de tesouro **escondido** (RN-10) no meio de um quadrado, com o **ouro** (moedas, gemas e arte) como valor e tudo o que há no tesouro, em português, na descrição; **os itens mágicos nunca viram XP**. A chamada leva uma chave de idempotência, que vale na campanha e é guardada com o hash do pedido (`map_points.create_key` e `create_hash`, migrations `00163` e `00164`): a mesma chave com outro pedido é recusada. Numa campanha por ouro, o ponto vira XP em "Voltar à cidade" como qualquer tesouro (MR-041); a tela lê o modo de XP da campanha pelo `GetCampaign`.
- **A página.** `/campaigns/:id/treasure` (`web/src/app/pages/treasure/`), só do mestre (o jogador lê "Só o mestre gera o tesouro da campanha."), com a entrada "Gerar tesouro" no painel "Mapas" da página da campanha. "Gerar tesouro" tem o tipo (Individual ou De covil), o nível do grupo (um contador de 1 a 20, o `app-number-stepper`, que começa no menor nível vivo, vindo do `GetTreasureParty`, e se pode mudar; se a leitura do grupo falha, a página diz isso, com "tente de novo") e a semente, que aparece depois de gerar; "Gerar outro" pede outro sem semente.
- **O resultado.** Desenha o que o servidor devolveu: as moedas com o valor de cada pilha em PO (e "10 PP valem 1 PO"), as gemas e a arte com os valores, os itens mágicos com o nome em português e o do SRD em inglês, a raridade, o valor ou "sem preço" e a sintonização, os idênticos juntos ("2 ×"), o ouro que vira o do ponto (moedas, gemas e arte) e o valor dos itens à parte, dito como o que não vira XP, e a linha do que a campanha faz com o ouro (por inimigos e por marcos, que não vira XP; por ouro, que se converte em "Voltar à cidade", MR-041). "Ver descrição" abre o item (raridade, valor com "Valores do SRD 5.2.1 (regras de 2024)" e o link "Créditos", a metade ou a regra do pergaminho em palavras, o texto do SRD em inglês com `lang="en"`). A página "Créditos" nomeia a p. 205 (os valores dos itens) ao lado da p. 201.
- **"Pôr no mapa" na página.** Escolhe o mapa e o quadrado (no computador, no desenho do mapa do mestre, com as setas também; no celular, a sala de uma masmorra gerada, ou o meio do mapa; no computador as salas da masmorra também são botões de rádio ao lado do mapa) e chama `PlaceTreasure` com o modo, o nível, a semente, o `content_version` e uma chave por pedido. O ponto nasce escondido e a confirmação o diz. Depois de posto, "Abrir o mapa" é o botão principal e "Pôr no mapa" some para aquele tesouro (um segundo clique faria um segundo ponto e, numa campanha por ouro, dobraria o XP); só "Gerar outro" traz de novo o botão. As recusas têm as próprias frases: tabelas mudadas ("Gere de novo", com o botão), mapa sem grade ("Escolha um mapa com grade"), quadrado fora da grade e o limite de 200 pontos.
- **Testes (Go).** `TestMR044_GenerateAsTheMaster`, `TestMR044_PartyLevelComesFromTheLivingCharacters`, `TestTreasureAuthorizationMatrix`, `TestMR044_GetMagicItem`, `TestMR044_PlaceTreasureMakesAHiddenTreasurePoint`, `TestMR044_PlaceTreasureRefusals`, `TestMR044_PlaceTreasureIsIdempotent`, `TestRN10_APlayerNeverSeesAPlacedTreasureUntilRevealed` (lê o que o jogador recebe, em JSON e no stream), `TestMR044_AGoldCampaignConvertsTheGoldOnly`; no `rules`, `TestTreasureIsDeterministic`, `TestTreasureInvariants`, `TestMagicItemValues`, `TestLoadTreasureRefuses` e `FuzzGenerateTreasure`.
- **Testes (navegador).** `@MR-044 @MR-041 @RN-10` em `e2e/tests/treasure.spec.ts`.

#### Relacionadas
- [MR-041](#mr-041-tesouros-e-xp-por-ouro), [MR-042](#mr-042-bestiário), [MR-043](#mr-043-gerar-encontros). Itens mágicos próprios ficam fora do MVP.

### MR-045: Consultar as magias

**Como** jogador, **quero** consultar todas as magias da mesa no app, **para** ler o que cada uma faz antes de escolher e durante a sessão.

- Prioridade: MVP
- Regras: RN-23
- Módulos: characters, rules

#### Critérios de aceite
- **Dado** que sou jogador de "Mirathel", **quando** abro "Magias", **então** vejo todas as magias disponíveis na mesa (as do SRD e as da mesa que o mestre deixou ligadas), com a busca pelo nome e os filtros por classe, nível da magia e escola, **e** cada uma abre a descrição inteira.
- **Dado** uma magia que o mestre desligou em "Opções para os jogadores", **quando** procuro por ela, **então** ela não aparece.
- **Dado** a minha ficha, **quando** escolho magias, **então** só posso escolher as da lista da minha classe; numa ficha com multiclasse, as de cada classe, pelo nível nela.

#### No app
- **Servidor.** `ContentService.ListSpells` lista as magias do SRD e as da mesa juntas, em ordem de nome em português: busca pelo nome em português ou em inglês (sem acento e sem diferenciar maiúscula), filtros por classe (a lista de uma classe da mesa e a do conjurador de um terço incluídas), nível e escola, e "só as que posso aprender" (a lista de cada classe da ficha até o nível que ela conjura; na multiclasse, cada uma pelo nível nela). Só o membro ativo lê (o pendente recebe `not_found`), o jogador nunca recebe uma magia da mesa arquivada, e a descrição inteira, com o alvo ("Só quem conjura", "Cone de 4,5 m"), é o `GetSpellDetails`.
- **O que o mestre desliga.** A magia que o mestre desligou em "Opções para os jogadores" (a do SRD ou a da mesa) não aparece em `ListSpells` nem em `GetSpellDetails` para um jogador (a não ser, nos detalhes, para quem tem a magia na ficha), e a classe desligada some de `class_keys`; o mestre as lê com a marca `off`. Ver [MR-025](#mr-025-cadastrar-conteúdo-da-mesa).
- **Leitura ao vivo.** Com a sessão aberta, quando o mestre liga ou desliga uma magia (ou escreve uma da mesa), a lista, a magia aberta e os nomes das classes se atualizam sem recarregar: a que foi desligada sai da lista e o cartão diz "Esta magia não está disponível." (`content_changed`).
- **A tela "Magias".** `/campaigns/:id/spells` (`web/src/app/pages/spells/`), para todo membro ativo, o mestre também; o painel "Magias" da página da campanha leva até ela. O app só pergunta e desenha: a busca, os filtros (classe, nível da magia, escola), "Só as que posso aprender" (com o personagem do próprio jogador; o mestre não tem o interruptor), o total ("73 magias") e as páginas (`page_token`, "Mostrar mais") são do `ListSpells`.
- **Disposição.** Em 1100 px ou mais são três colunas: os filtros, a lista e a descrição ao lado. Abaixo disso a página é uma coluna de até 680 px, com a busca e "Filtros (n)" (uma folha no celular, um diálogo do tablet para cima; a partir de 768 px os dois dividem uma faixa) e os chips do que está ligado; a magia abre no lugar da lista, como um passo do histórico: "Voltar para Magias" e o Voltar do navegador fecham a magia e trazem a mesma lista, na mesma altura, com a mesma busca e o foco na linha que estava aberta.
- **A descrição.** É a peça compartilhada `shared/spell-details` (`SpellBody`), com a linha "Alvo" vinda de `target.label_pt`. Uma magia da mesa leva a etiqueta "Da mesa", os campos "Ataque" e "Dano" e o texto do mestre em português, sem a marca do SRD; a do SRD leva o texto em inglês e "Créditos". A magia arquivada só chega ao mestre, com "Arquivada".
- **Estados e link.** Carregando (linhas cinzas da mesma altura, sem pulo), vazio ("Nenhuma magia com “zzz”." e "Limpar a busca"), erro com "Tentar de novo" e ficha básica com o filtro (uma frase, não um erro, com "Ler todas as magias"). Os filtros (escritos quando uma busca começa) e a magia aberta ficam no link (`?q=`, `class`, `levels`, `school`, `mine`, `spell`), e a página relê o link quando o histórico traz outro. As respostas guardadas (descrições e nomes das classes) valem pela versão do conteúdo da mesa: uma edição do mestre aparece sem recarregar. Uma magia que não existe, ou que o mestre arquivou para o jogador, tem a própria frase ("Esta magia não está disponível."), sem "Tentar de novo".
- **Testes (Go).** `TestListSpellsSearch`, `TestListSpellsFilters`, `TestListSpellsLearnable`, `TestListSpellsHidden` (`rules`), `TestMR045_TheSpellsPage`, `TestMR045_OnlyTheSpellsACharacterCanLearn` e a linha do método em `TestAuthorizationMatrix` (`characters`).
- **Testes (navegador).** Vitest (`spells-filter`, `spells-state`, `spells`, `spell-rows`, `spell-details-map`); Playwright `@MR-045` (`e2e/tests/spells.spec.ts`: a busca "maos" e o "Cone de 4,5 m"; "Só as que posso aprender" de um Mago de nível 1 só com truques e 1º nível; a magia arquivada fora do jogador, na tela e no JSON, e com "Arquivada" para o mestre) e as telas no `a11y.spec.ts`.

#### Relacionadas
- Uma parte da [MR-020](#mr-020-consultar-o-livro-de-regras), só com as magias. O mestre aceitar uma magia fora da lista da classe fica para depois do MVP ([MR-046](#mr-046-o-estilo-da-mesa-recurso-por-recurso)). Conteúdo da mesa: [MR-025](#mr-025-cadastrar-conteúdo-da-mesa).

## Prioridade: MVP (pré-requisito)

### MR-002: Gerar convite

**Como** mestre, **quero** gerar um link de convite, **para** os jogadores entrarem na campanha.

- Prioridade: MVP (pré-requisito)
- Regras: RN-07
- Módulos: campaigns

#### Critérios de aceite
- **Dado** que sou mestre de "Mirathel", **quando** gero um convite, **então** recebo um link que vale para uma pessoa por 7 dias **e** o servidor guarda só o hash do token.
- **Dado** um convite ainda não usado, **quando** o mestre o revoga, **então** o link para de funcionar **e** quem já entrou continua na campanha.
- **Dado** que sou jogador de "Mirathel", **quando** tento gerar, listar ou revogar convites, **então** o servidor recusa.

O mestre pode escolher o número de usos e a validade do convite; o comportamento implementado é a RN-07.

#### No app
- O servidor deixa o mestre escolher de 1 a 20 usos (padrão 1) e de 5 minutos a 30 dias de validade (padrão 7 dias), e revogar; a tela oferece validade de 1, 7 ou 30 dias.
- A seção "Convites" de `/campaigns/:id` (`web/src/app/pages/campaign-detail/invites/`) é só para o mestre. Cria convite (usos e validade, com os presets 1/7/30 dias), mostra o link uma vez com aviso e botão de copiar, lista cada convite numa linha (usos, validade, se exige aprovação e o estado: Ativo, Usado, Expirado ou Revogado) e revoga.
- Testes Go: `TestMR002_MasterGetsASingleUseSevenDayInviteStoredAsAHash`, `TestMR002_RevokedInviteStopsWorking`, `TestMR002_OnlyTheMasterManagesInvites`.
- Teste Playwright: `o mestre gera um convite, vê o link uma vez e o revoga` (`@MR-002`, `e2e/tests/campaigns.spec.ts`).

#### Relacionadas
- RN-07 (ver [Regras de negócio](regras.md)).
- RN-15 (convite com aprovação) e [MR-024](#mr-024-aprovar-o-personagem-do-convite) estendem esta história: o mestre marca "Exigir aprovação do mestre" no convite, o jogador já cria o personagem pelo convite, e o mestre aprova.

### MR-005: Criar NPCs

**Como** mestre, **quero** criar NPCs de cada tipo (inimigo, boss, minion, história), com ficha completa ou básica conforme o tipo.

- Prioridade: MVP (pré-requisito)
- Regras: RN-04
- Módulos: characters

#### Critérios de aceite
- **Dado** que sou mestre de "Mirathel", **quando** crio um inimigo ou um boss, **então** ele tem ficha completa; **quando** crio um minion ou um NPC de história, **então** ele tem ficha básica.
- **Dado** que sou jogador de "Mirathel", **quando** abro a campanha, **então** não vejo nenhum NPC **e** o servidor recusa se eu tentar criar um.
- **Dado** um NPC criado em "Mirathel", **quando** o mestre abre outra campanha, **então** o NPC não aparece lá: usar o mesmo NPC em outras campanhas é a [MR-022](#mr-022-reutilizar-npcs).

#### No app
- O mestre cria NPCs com `CharacterService.CreateCharacter`. Inimigo e boss levam a ficha completa (com os valores calculados), minion e NPC de história levam a ficha básica, e o servidor recusa a ficha do tipo errado.
- O NPC fica na campanha em que foi criado, e o dono é o mestre.
- Testes Go: `TestMR005_MasterCreatesNpcsOfEachKind` (primeiro critério) e `TestMR005_PlayersCannotSeeOrCreateNpcs` (segundo e terceiro).
- Testes Playwright: "o mestre cria um inimigo com ficha completa e um minion com ficha básica" e "o jogador não vê os NPCs da campanha".

## Prioridade: Depois

Fora do MVP. Entram na Etapa 11 do [roadmap](../../roadmap.md).

### MR-007: Importar ficha em PDF

**Como** jogador, **quero** importar minha ficha de um PDF escolhendo o formato (D&D Beyond ou ficha em português), **para** não digitar tudo de novo.

- Prioridade: Depois
- Regras: RN-08
- Módulos: characters

#### Relacionadas
- RN-08: sem DOCX; só PDF editável, no formato do D&D Beyond ou da ficha em português.

### MR-017: Subir de nível

**Como** jogador, ao subir de nível, **quero** escolher o que ganho pela minha classe, subclasse ou uma segunda classe, seguindo as regras do D&D 5e.

- Prioridade: Depois
- Regras: RN-12
- Módulos: progression, rules

#### Relacionadas
- Uma parte desta história está no MVP: a edição guiada da ficha, [MR-040](#mr-040-subir-de-nível-pela-ficha). Esta história fica depois do MVP como a tela completa de subir de nível.
- O motor de regras (regras como dados) é pré-requisito direto desta história. Ver [ADR-0008](../../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

### MR-020: Consultar o livro de regras

**Como** mestre, **quero** consultar o livro de regras com uma busca inteligente.

- Prioridade: Depois
- Regras: —
- Módulos: rules

As magias vieram antes, no MVP: [MR-045](#mr-045-consultar-as-magias).

### MR-021: Copiar personagem

**Como** jogador, **quero** copiar meu personagem para outra campanha, **para** jogar as duas ao mesmo tempo ou uma continuação.

- Prioridade: Depois
- Regras: RN-03, RN-23
- Módulos: characters

#### Critérios de aceite (proposta)
- **Dado** uma ficha que usa conteúdo da mesa (uma classe, raça ou magia que o mestre cadastrou), **quando** o jogador tenta copiá-la para outra campanha, **então** o app recusa e diz por quê: o conteúdo da mesa vale só na campanha dele (RN-23, que veio da MR-025).

#### Relacionadas
- RN-03 (ver [Regras de negócio](regras.md)).

### MR-022: Reutilizar NPCs

**Como** mestre, **quero** usar meus NPCs em várias campanhas, **para** não recriar o mesmo vilão.

- Prioridade: Depois
- Regras: RN-04, RN-23
- Módulos: characters

#### Critérios de aceite (proposta)
- **Dado** um NPC que usa conteúdo da mesa, **quando** o mestre o leva para outra campanha, **então** o app recusa e diz por quê (RN-23, que veio da MR-025).

### MR-023: Passar ou dividir a campanha

**Como** mestre, **quero** passar minha campanha para outro mestre, ou ter um segundo mestre nela, **para** a campanha continuar mesmo se eu sair.

- Prioridade: Depois
- Regras: RN-13
- Módulos: campaigns

#### Consequência
Hoje, excluir a conta de quem criou a campanha apaga a campanha inteira (ver [Privacidade](../privacidade.md#excluir-a-conta)). Com mais de um mestre, ou depois de uma passagem de campanha, isso muda: a campanha só é apagada quando o último mestre sai. Ver [ADR-0011](../../adr/0011-autorizacao-papeis-por-campanha.md), como proposta.

### MR-026: Propor uma raça ou classe nova

**Como** jogador, **quero** propor uma raça ou uma classe que não existe no app ao criar o personagem, com o PDF ou o link das regras, **para** o mestre ler e decidir.

- Prioridade: Depois
- Regras: RN-15 (a mesma ideia de aprovação do convite)
- Módulos: rules, characters

Vem depois da MR-025, e a proposta do jogador só vale depois que o mestre aprova.

#### Critérios de aceite (proposta)
- **Dado** que quero jogar de cozinheiro, uma classe feita por fãs, **quando** crio o personagem e proponho a classe com o link do PDF, **então** o mestre vê o pedido **e** o personagem fica esperando a decisão.
- **Dado** um pedido de classe nova, **quando** o mestre aprova e cadastra as regras dela (MR-025), **então** o personagem passa a usar a classe; **quando** o mestre recusa, **então** o jogador escolhe outra classe.

#### Exemplo
O jogador quer jogar de cozinheiro, uma classe não oficial. Ele cadastra a classe ao criar o personagem e põe o link do PDF (ou o PDF) para o mestre ler, aprovar ou recusar, e cadastrar como funcionam as regras dela.

### MR-027: Ler as regras de um PDF

**Como** mestre, **quero** mandar o PDF com as regras e ver o app cadastrar sozinho as classes, raças e regras dele, **para** não digitar tudo.

- Prioridade: Depois
- Regras: —
- Módulos: rules

Vem depois do cadastro pelo mestre (MR-025), e o mestre revisa tudo antes de valer.

#### Critérios de aceite (proposta)
- **Dado** um PDF de regras enviado pelo mestre, **quando** o app o lê, **então** mostra o que encontrou ao mestre **e** nada vale na campanha até o mestre revisar e aprovar.
- **Dado** um PDF enviado, **quando** o processamento termina ou falha, **então** o arquivo é apagado; um prazo curto (TTL) no arquivo guardado garante o apagamento mesmo se o processamento falhar.

#### Dúvidas
- Ler um PDF de regras automaticamente precisa de um serviço de IA, que custa por uso e recebe o PDF. Um livro oficial tem direito autoral: o app não pode redistribuir o texto, e o resultado só pode aparecer para a mesa. Quando não der para ler o PDF, o cadastro fica com o mestre (MR-025). O app não guarda o PDF: ele fica só enquanto é processado (ver [Privacidade](../privacidade.md#a-definir)).

### MR-046: O estilo da mesa, recurso por recurso

**Como** mestre, **quero** escolher, recurso por recurso, o que o app faz na minha mesa, **para** usar o app do jeito que eu mestro, do tudo digital à mesa clássica.

- Prioridade: Depois
- Regras: RN-24
- Módulos: campaigns, play, maps, characters

No MVP entram os três estilos prontos da página "Regras da mesa" (RN-24); o resto das ideias fica guardado aqui.

#### Ideias (das personas: o mestre narrador, o da mesa física, o tático, o iniciante, o improvisador e a mesa sem celulares)
- O app avisar em vez de impedir: o movimento além do deslocamento, o alcance, os espaços de magia (o mestre decide).
- Esconder os mapas dos jogadores, ou o celular do jogador só com a ficha.
- Quem age pelo celular: os jogadores, ou o mestre lança tudo e os jogadores só acompanham.
- Quem sobe de nível: o jogador pela ficha (hoje, MR-040) ou só o mestre.
- As palavras de estado dos inimigos ("Ferido"): mostrar ou esconder.
- O mestre aceitar uma magia fora da lista da classe ([MR-045](#mr-045-consultar-as-magias)).
- Um interruptor por recurso, com os estilos prontos como ponto de partida.

### MR-047: Mais quebra-cabeças

**Como** mestre, **quero** mais tipos de quebra-cabeça e mais jeitos de dar recompensa, **para** casar o desafio com a história da minha campanha.

- Prioridade: Depois
- Regras: RN-27
- Módulos: play, maps

O MVP já tem seis tipos, as dicas, a dica por teste de perícia, a informação dividida, as consequências e "Ao resolver" ([MR-038](#mr-038-quebra-cabeças)); o resto das ideias fica guardado aqui.

#### Ideias
- Peças deslizantes.
- Placas de pressão na grade do mapa (pisar nos quadrados na ordem certa).
- Balança e pesos.
- Um raio de luz e espelhos na grade (usando a visão do mapa).
- Os símbolos tirados da galeria do mestre e um texto para cada estado do quebra-cabeça.
- Recompensas: XP, um item posto como tesouro no mapa, uma nota da história.

## Ver também

- [Regras de negócio](regras.md)
- [Visão do produto](visao.md)
- [Glossário](glossario.md)
- [Roadmap](../../roadmap.md)
