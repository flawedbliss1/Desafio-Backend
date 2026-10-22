Organização do código:

- `main.go`: composição da aplicação com Fx.
- Configuração e construtores: leitura das variáveis e criação do cliente HTTP e do pool PostgreSQL.
- `lifecycle.go`: início do servidor e encerramento dos recursos.
- `routes.go`: rotas de saúde e de carteiras.
- `auth.go`: autorização das chamadas.
- `money.go`: conversão de valores decimais para centavos.
- `wallets.go`: consultas e alterações de carteiras e lançamentos financeiros.
- `wagering.go`: payload, hash, persistência e processamento da aposta.
- `wagering_http.go`: validações HTTP e tradução dos resultados de aposta em respostas.
- `migrations`: criação das tabelas e adição das constraints.

## Valores monetários

Os valores chegam e saem como strings com duas casas decimais. Os valores são `int64`, não é usado valores float. A moeda é BRL.

## Operação

Criar uma carteira com saldo positivo envolve três registros: carteira, a transação `OPENING` e o lançamento `CREDIT`. Qualquer erro na hora da criação da carteira, um rollback é provocado.

## Concorrência e idempotência

A leitura da carteira é feita com `SELECT ... FOR UPDATE`. O bloqueio de linha é adquirido antes da inserção da nova aposta, de modo que qualquer outra transação que tente bloquear a mesma carteira aguarda a transação corrente concluir e, só então, avalia o saldo já atualizado.

Dois índices únicos protegem as combinações `(provider_id, idempotency_key)` e `(provider_id, external_transaction_id)`. Uma chave existente com conteúdo diferente gera conflito.


O saldo resultante é persistido na própria transação. Um replay devolve esse resultado original.

`PENDING` é um estado intermediário dentro da transação SQL. O caminho implementado confirma a aposta já como `PROCESSED` ou `REJECTED`.

## Autorização

O middleware lê o token de `Authorization: Bearer ...` e consulta o endpoint de introspecção do Keycloak usando as credenciais de `jungle-api`. Confere atividade, emissor, cliente permitido e role antes de chamar o handler.

As carteiras exigem o cliente `servico-interno` e a role da operação. As apostas exigem um cliente presente no mapa de provedores e `wager:write`. O provedor autorizado é comparado com `providerId` do corpo, antes do processamento financeiro e replay.

Ausência de credencial válida produz `401`; cliente ou permissão inadequados produzem `403`; indisponibilidade na introspecção produz `503`.

