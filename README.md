# Labs Auction — GoExpert

Sistema de leilões com fechamento automático via Goroutines.

## Pré-requisitos

- Docker e Docker Compose instalados
- Go 1.25+ (apenas para desenvolvimento local)

## Executar com Docker Compose

```bash
docker compose up --build
```

A API estará disponível em `http://localhost:8080`.

## Variáveis de Ambiente

Configuradas em `cmd/auction/.env`:

| Variável | Descrição | Padrão |
|---|---|---|
| `AUCTION_INTERVAL` | Duração do leilão. Aceita formato Go: `30s`, `5m`, `1h` | `5m` |
| `BATCH_INSERT_INTERVAL` | Intervalo de batch insert de bids | `20s` |
| `MAX_BATCH_SIZE` | Tamanho máximo do batch de bids | `4` |
| `MONGODB_URL` | URL de conexão com o MongoDB | — |
| `MONGODB_DB` | Nome do banco de dados | `auctions` |

### Configurar duração do leilão

Edite `cmd/auction/.env` e ajuste `AUCTION_INTERVAL`:

```env
AUCTION_INTERVAL=30s   # leilão fecha em 30 segundos
# AUCTION_INTERVAL=5m  # leilão fecha em 5 minutos
# AUCTION_INTERVAL=1h  # leilão fecha em 1 hora
```

## Endpoints da API

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/auction` | Criar leilão |
| `GET` | `/auction` | Listar leilões |
| `GET` | `/auction/:auctionId` | Buscar leilão por ID |
| `GET` | `/auction/winner/:auctionId` | Lance vencedor do leilão |
| `POST` | `/bid` | Criar lance |
| `GET` | `/bid/:auctionId` | Listar lances de um leilão |
| `GET` | `/user/:userId` | Buscar usuário |

## Executar testes

> Requer Docker rodando localmente (testcontainers sobe MongoDB automaticamente).

```bash
go test ./internal/infra/database/auction/... -v -timeout 60s
```

## Funcionamento do fechamento automático

Ao criar um leilão, uma goroutine é agendada via `time.AfterFunc`. Quando `AUCTION_INTERVAL` expira, ela atualiza o status do leilão para `Completed` no MongoDB. Lances em leilões fechados ou expirados são automaticamente rejeitados.
