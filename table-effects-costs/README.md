# Custos, cura, gatilhos e recarga nos efeitos de conteúdo da mesa

Antes e depois da PR #516 (trem 47, `refactor/train-47`) contra a `main` anterior (trem 46, #515).
Todo o conteúdo é inventado (raça "Brasa de Teste", traço "Reserva de Teste"). Cada estado tem quatro imagens por lado:
`<estado>-<antes|depois>-<390|1280>-<light|dark>.png`.

Na `main` os campos de custo, cura, gatilho e recarga não existem. Por isso o "antes" mostra a mesma tela com a raça
cadastrada sem os campos novos (recurso que volta no descanso longo, ação de bônus que gasta 1 uso, traços só com texto).

| Estado | O que mostra |
| --- | --- |
| `editor` | Editor da raça pelo mestre, página inteira. Depois: recurso com recarga "Ao rolar iniciativa", ação com custo, custo máximo ("o jogador escolhe"), cura em dados e valor por ponto, e gatilhos (descanso longo com PV temporários; falha no teste contra a morte). Antes: o mesmo traço sem esses campos. |
| `folha-quantos` | Aba Habilidades do jogador, no turno dele. Depois: a folha "Usar" com "Quantos?" (passo de 1 a 3), "Para quem" (o próprio ou outra pessoa do grupo) e a cura "3d6". Antes: não existe folha; só a linha "Usar" da habilidade, que gasta direto (é o estado mais próximo). |
| `gatilho-descanso` | Prévia do descanso longo, na aba Grupo do mestre. Depois: linha do gatilho ("Fôlego da Mesa entrou em ação: ganha 5 PV temporários"). Antes: sem a linha do gatilho. |
| `gatilho-log` | Registro do combate (mestre) depois de um teste contra a morte que falhou. Depois: o registro diz que o traço levantou o personagem com 22 PV. Antes: só a falha comum. |
| `gatilho-morte` | Resultado do teste contra a morte na tela do jogador. Depois: "o traço entrou em ação: você volta com 22 PV. Nenhuma falha entra na conta." Antes: falha normal, com a marca nas falhas. |
| `recusa-sem-usos` | Habilidade sem usos restantes, desligada, com o motivo. Depois: "Faltam usos: gasta 1, restam 0" e "Sem usos: volta um uso ao rolar iniciativa". Antes: "Sem usos: volta num descanso longo" (a recarga por iniciativa não existe na `main`; é o mais próximo). |

## Limites

- A recusa mostrada é a da linha desligada na lista de habilidades, que é como a tela avisa. A mensagem de erro do servidor para um uso sem saldo (quando a tela está desatualizada) não foi capturada.
- O gatilho `drop_to_zero` não foi usado; o registro foi mostrado pelo gatilho de falha no teste contra a morte, que a pilha alcança de forma confiável.
- No "antes" do `gatilho-morte`, a imagem é o estado da `main`; a quantidade de falhas marcadas pode variar entre as imagens porque o mesmo personagem repetiu o teste em cada tamanho e tema.
