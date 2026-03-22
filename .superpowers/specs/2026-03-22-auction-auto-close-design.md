# Design: Fechamento Automático de Leilões

**Data:** 2026-03-22
**Status:** Aprovado
**Escopo:** `internal/infra/database/auction/create_auction.go` + teste automatizado

---

## Contexto

O sistema de leilões permite criar leilões (`Active`) e registrar lances. A lógica de bids já valida se o leilão está no prazo usando `AUCTION_INTERVAL` (timestamp do leilão + intervalo), mas **nunca persiste** a mudança de status no banco. O objetivo é adicionar o fechamento automático: quando o tempo expira, o status no MongoDB muda para `Completed`.

---

## Decisões de Design

| Decisão | Escolha | Motivo |
|---|---|---|
| Env var de duração | `AUCTION_INTERVAL` (existente) | Única fonte de verdade; evita inconsistência entre bid e auto-close |
| Mecanismo de goroutine | `time.AfterFunc` | Não bloqueia, idiomático, sem estado extra |
| Context da atualização | `context.Background()` novo | O ctx da request HTTP expira antes do timer disparar |
| Testes | `testcontainers-go` com MongoDB real | Confiabilidade de integração; Docker já é requisito do projeto |

---

## Arquitetura

### Mudanças em `create_auction.go`

**`AuctionRepository`** — novo campo:

```go
type AuctionRepository struct {
    Collection      *mongo.Collection
    auctionInterval time.Duration  // lido de AUCTION_INTERVAL
}
```

**`NewAuctionRepository`** — popula o campo via `getAuctionInterval()`:

```go
func NewAuctionRepository(database *mongo.Database) *AuctionRepository {
    return &AuctionRepository{
        Collection:      database.Collection("auctions"),
        auctionInterval: getAuctionInterval(),
    }
}

func getAuctionInterval() time.Duration {
    d, err := time.ParseDuration(os.Getenv("AUCTION_INTERVAL"))
    if err != nil {
        return 5 * time.Minute
    }
    return d
}
```

**`CreateAuction`** — dispara timer após inserção:

```go
func (ar *AuctionRepository) CreateAuction(ctx context.Context, entity *auction_entity.Auction) *internal_error.InternalError {
    // ... insert existente ...

    time.AfterFunc(ar.auctionInterval, func() {
        closeCtx := context.Background()
        filter := bson.M{"_id": entity.Id}
        update := bson.M{"$set": bson.M{"status": auction_entity.Completed}}
        if _, err := ar.Collection.UpdateOne(closeCtx, filter, update); err != nil {
            logger.Error("Error closing auction", err)
        }
    })

    return nil
}
```

### Fluxo de dados

```
POST /auction
  └─ CreateAuction (usecase)
       └─ AuctionRepository.CreateAuction
            ├─ InsertOne(ctx, entity)          → MongoDB
            └─ time.AfterFunc(interval, fn)    → goroutine agendada
                                                    (dispara após AUCTION_INTERVAL)
                                                    └─ UpdateOne(status=Completed) → MongoDB
```

---

## Tratamento de erros

- Falha no `InsertOne`: retorna `InternalServerError` (comportamento existente).
- Falha no `UpdateOne` do timer: loga o erro (não há como propagar — goroutine desacoplada da request).
- `AUCTION_INTERVAL` inválido: fallback para 5 minutos (mesmo padrão do `create_bid.go`).

---

## Testes

**Arquivo:** `internal/infra/database/auction/create_auction_test.go`

**Dependência:** `testcontainers-go` com imagem `mongo:latest`

**Cenário principal:**

1. Subir container MongoDB efêmero via testcontainers.
2. Setar `AUCTION_INTERVAL=2s` via `os.Setenv`.
3. Criar `AuctionRepository` e chamar `CreateAuction`.
4. Verificar imediatamente que `status == Active`.
5. `time.Sleep(3 * time.Second)` (intervalo + margem).
6. Buscar o leilão no MongoDB e verificar que `status == Completed`.

**Critério de sucesso:** O status muda de `Active` para `Completed` automaticamente, sem nenhuma chamada manual.

---

## Arquivos Alterados

| Arquivo | Tipo de mudança |
|---|---|
| `internal/infra/database/auction/create_auction.go` | Modificação (campo + timer) |
| `internal/infra/database/auction/create_auction_test.go` | Criação (teste de integração) |
| `go.mod` / `go.sum` | Adição de `testcontainers-go` |

---

## Fora do Escopo

- Recuperação de leilões ativos após restart do servidor (não pedido no enunciado).
- Cancelamento do timer (não há caso de uso de exclusão de leilão).
- Alterações em controllers, usecases ou entidades.
