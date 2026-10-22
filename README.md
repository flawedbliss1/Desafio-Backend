API em Go para criação e consulta de carteiras, com processamento de apostas, usando PostgreSQL e autenticação por Keycloak.
Ainda há muitas pendências, esse projeto é uma entrega parcial.

## Funcionalidades implementadas

- Criação de carteira em BRL, com saldo inicial zero ou positivo.
- Consulta de carteira e saldo.
- Controle de concorrência com bloqueio da carteira no PostgreSQL.
- Autenticação por introspecção no Keycloak e autorização por cliente e role.
- Inicialização e encerramento dos recursos com Uber Fx.

## Pré-requisitos e portas

São necessários Go compatível com o `go.mod`, Git e Docker Desktop com Docker Compose.


API Go `http://127.0.0.1:8080` 
PostgreSQL `127.0.0.1:5433` 
Keycloak `http://localhost:8081`

## 1. Iniciar os serviços e preparar o banco

Abra um terminal dentro da pasta do projeto e execute:

```bat
docker compose up -d db keycloak
docker compose ps
```

Em um outro CMD:

```bat
docker compose exec -T db psql -U jungle -d jungle -v ON_ERROR_STOP=1 < migrations\001_create_wallets.up.sql
docker compose exec -T db psql -U jungle -d jungle -v ON_ERROR_STOP=1 < migrations\002_create_wager_transactions.up.sql
docker compose exec -T db psql -U jungle -d jungle -v ON_ERROR_STOP=1 < migrations\003_create_wallet_ledger_entries.up.sql
docker compose exec -T db psql -U jungle -d jungle -v ON_ERROR_STOP=1 < migrations\004_prepare_bet.up.sql
docker compose exec -T db psql -U jungle -d jungle -v ON_ERROR_STOP=1 < migrations\005_add_wager_result_check.up.sql
```

## 2. Configurar o Keycloak

Infelizmente, a configuração da autenticação só pode ser feita manualmente. Com o keycloak executado, acesse `http://localhost:8081` com o login e senha configurados no `compose.yaml`. O realm a ser nomeado é **jungle** e os clientes precisam ser configurados da seguinte forma:

1. `jungle-api`: A introspecção é consultada com esse cliente. Sua secret key tem que ser copiada para `JUNGLE_API_CLIENT_SECRET`.
2. `servico-interno`: Usado pra criar e consultar carteiras. Na crição desse cliente, habilite "Service Accounts" e crie as client roles `wallet:read` e `wallet:write`.
3. `provider-a`: Envia as apostas. Na criação desse cliente, habilite "Service Accounts" e crie a client role `wager:write`.



Em `servico-interno` e `provider-a`, um mapper **Audience** precisa ser adicionado. 

- Nome: `jungle-api-audience`.
- Included Client Audience: `jungle-api`.
- Add to access token: habilitado.
- Add to token introspection: habilitado.

## 3. Executar a API

No CMD que executará a API:

```bat
set "DATABASE_URL=postgres://jungle:jungle_local@127.0.0.1:5433/jungle?sslmode=disable"
set "JUNGLE_API_CLIENT_SECRET=SECRET_JUNGLE_API"
go run ./cmd/api
```

O arquivo `.env.example` documenta as variáveis. A aplicação usa `os.Getenv` e **não carrega um arquivo `.env` automaticamente**. Cada janela do CMD precisa das próprias variáveis.

## 4. Fazer chamadas

Use outro CMD, mantendo a API em execução. Obtenha um token interno:

```bat
set "JUNGLE_CLIENT_SECRET=SECRET_SERVICO_INTERNO"
curl.exe -sS http://localhost:8081/realms/jungle/protocol/openid-connect/token --data-urlencode "grant_type=client_credentials" --data-urlencode "client_id=servico-interno" --data-urlencode "client_secret=%JUNGLE_CLIENT_SECRET%"
set "JUNGLE_ACCESS_TOKEN=ACCESS_TOKEN_AQUI"
```

Cria uma carteria com R$ 100,00. O jogador do exemplo é novo no banco e uma carteira e moeda já existente para o mesmo jogador retorna `409` 

```bat
curl.exe -i http://127.0.0.1:8080/wallets -H "Authorization: Bearer %JUNGLE_ACCESS_TOKEN%" -H "Content-Type: application/json" -d "{\"playerId\":\"99999999-9999-4999-8999-999999999999\",\"initialBalance\":{\"amount\":\"100.00\",\"currency\":\"BRL\"}}"
set "JUNGLE_WALLET_ID=ID_DA_CARTEIRA"
curl.exe -i http://127.0.0.1:8080/wallets/%JUNGLE_WALLET_ID% -H "Authorization: Bearer %JUNGLE_ACCESS_TOKEN%"
```

Para testar a aposta:

```bat
set "JUNGLE_PROVIDER_SECRET=SECRET_PROVIDER_A"
curl.exe -sS http://localhost:8081/realms/jungle/protocol/openid-connect/token --data-urlencode "grant_type=client_credentials" --data-urlencode "client_id=provider-a" --data-urlencode "client_secret=%JUNGLE_PROVIDER_SECRET%"
set "JUNGLE_PROVIDER_TOKEN=ACCESS_TOKEN_PROVIDER"
```

Envie a aposta:

```bat
curl.exe -i http://127.0.0.1:8080/wagering/transactions -H "Authorization: Bearer %JUNGLE_PROVIDER_TOKEN%" -H "Content-Type: application/json" -H "Idempotency-Key: provider-a:bet-exemplo-001" -d "{\"providerId\":\"provider-a\",\"externalTransactionId\":\"bet-exemplo-001\",\"playerId\":\"99999999-9999-4999-8999-999999999999\",\"walletId\":\"%JUNGLE_WALLET_ID%\",\"roundId\":\"round-exemplo-001\",\"gameId\":\"game-exemplo\",\"kind\":\"BET\",\"money\":{\"amount\":\"10.00\",\"currency\":\"BRL\"}}"
```

Na primeira execução, o resultado esperado é `PROCESSED`, saldo `90.00` e `idempotentReplay: false`. Repetir o mesmo comando com o mesmo conteúdo resulta uma resposta com o mesmo transactionId, porém com `idempotencyKey: true`
