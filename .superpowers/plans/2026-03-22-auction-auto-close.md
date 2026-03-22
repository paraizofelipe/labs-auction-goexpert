# Auction Auto-Close Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adicionar fechamento automático de leilões via goroutine que, após `AUCTION_INTERVAL`, atualiza o status para `Completed` no MongoDB.

**Architecture:** Ao criar um leilão, `AuctionRepository.CreateAuction` dispara `time.AfterFunc(auctionInterval, fn)` logo após o insert. A função `fn` executa um `UpdateOne` no MongoDB com `context.WithTimeout` de 10s. Nenhuma outra camada é alterada.

**Tech Stack:** Go 1.20, MongoDB (mongo-driver v1.14), testcontainers-go (módulo mongodb), Docker

---

## Mapa de arquivos

| Arquivo | Ação | Responsabilidade |
|---|---|---|
| `internal/infra/database/auction/create_auction.go` | Modificar | Adicionar `auctionInterval`, `getAuctionInterval()`, timer em `CreateAuction` |
| `internal/infra/database/auction/create_auction_test.go` | Criar | Teste de integração do auto-close com MongoDB real |
| `go.mod` / `go.sum` | Modificar | Adicionar `testcontainers-go` e módulo mongodb |
| `README.md` | Criar | Instruções de execução, Docker, variáveis de ambiente |

---

## Task 1: Adicionar dependências de teste

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Adicionar testcontainers-go e o módulo MongoDB**

```bash
cd /caminho/do/projeto
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/mongodb@latest
```

Esperado: `go.mod` e `go.sum` atualizados sem erros.

- [ ] **Step 2: Verificar que o módulo foi adicionado**

```bash
grep testcontainers go.mod
```

Esperado: linhas com `github.com/testcontainers/testcontainers-go` presentes.

- [ ] **Step 3: Commit das dependências**

```bash
git add go.mod go.sum
git commit -m "chore: add testcontainers-go for integration tests"
```

---

## Task 2: Escrever o teste de integração (deve falhar)

**Files:**
- Create: `internal/infra/database/auction/create_auction_test.go`

O teste verifica o cenário completo: cria um leilão, confirma status `Active`, aguarda o timer, confirma status `Completed`. Deve **falhar** neste momento porque a feature ainda não existe.

- [ ] **Step 1: Criar o arquivo de teste**

Criar `internal/infra/database/auction/create_auction_test.go` com o conteúdo abaixo:

```go
package auction

import (
	"context"
	"fullcycle-auction_go/internal/entity/auction_entity"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestCreateAuction_AutoClose(t *testing.T) {
	ctx := context.Background()

	mongoContainer, err := mongodb.Run(ctx, "mongo:latest")
	if err != nil {
		t.Fatalf("falha ao iniciar container mongodb: %s", err)
	}
	defer func() {
		if err := mongoContainer.Terminate(ctx); err != nil {
			t.Logf("falha ao terminar container mongodb: %s", err)
		}
	}()

	endpoint, err := mongoContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("falha ao obter connection string: %s", err)
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(endpoint))
	if err != nil {
		t.Fatalf("falha ao conectar ao mongodb: %s", err)
	}
	defer client.Disconnect(ctx)

	database := client.Database("auctions_test")

	os.Setenv("AUCTION_INTERVAL", "2s")
	defer os.Unsetenv("AUCTION_INTERVAL")

	repo := NewAuctionRepository(database)

	auctionEntity, internalErr := auction_entity.CreateAuction(
		"Produto Teste",
		"Eletronicos",
		"Descrição completa do produto teste para leilão",
		auction_entity.New,
	)
	if internalErr != nil {
		t.Fatalf("falha ao criar entidade auction: %v", internalErr)
	}

	createErr := repo.CreateAuction(ctx, auctionEntity)
	if createErr != nil {
		t.Fatalf("falha ao criar auction no banco: %v", createErr)
	}

	// Verifica status inicial: deve ser Active
	result, findErr := repo.FindAuctionById(ctx, auctionEntity.Id)
	if findErr != nil {
		t.Fatalf("falha ao buscar auction: %v", findErr)
	}
	if result.Status != auction_entity.Active {
		t.Errorf("status inicial esperado Active (0), obtido %d", result.Status)
	}

	// Aguarda o timer disparar (2s) + margem (500ms)
	time.Sleep(2*time.Second + 500*time.Millisecond)

	// Verifica status final: deve ser Completed
	result, findErr = repo.FindAuctionById(ctx, auctionEntity.Id)
	if findErr != nil {
		t.Fatalf("falha ao buscar auction após timer: %v", findErr)
	}
	if result.Status != auction_entity.Completed {
		t.Errorf("status após timer esperado Completed (1), obtido %d", result.Status)
	}
}
```

- [ ] **Step 2: Executar o teste para confirmar que falha**

```bash
go test ./internal/infra/database/auction/... -run TestCreateAuction_AutoClose -v
```

Esperado: **FAIL** — o teste deve falhar porque `AuctionRepository` não tem `auctionInterval` e `CreateAuction` não dispara nenhum timer ainda. O teste pode falhar em compilação ou em tempo de execução (status nunca muda para `Completed`).

> **Nota:** Docker deve estar rodando localmente para o testcontainers iniciar o container MongoDB.

- [ ] **Step 3: Commit do teste falhando**

```bash
git add internal/infra/database/auction/create_auction_test.go
git commit -m "test: add failing integration test for auction auto-close"
```

---

## Task 3: Implementar o auto-close em `create_auction.go`

**Files:**
- Modify: `internal/infra/database/auction/create_auction.go`

- [ ] **Step 1: Substituir o conteúdo de `create_auction.go`**

O arquivo atual tem 50 linhas. Substituir pelo conteúdo abaixo (adiciona `auctionInterval`, `getAuctionInterval()` e o `time.AfterFunc` em `CreateAuction`):

```go
package auction

import (
	"context"
	"fullcycle-auction_go/configuration/logger"
	"fullcycle-auction_go/internal/entity/auction_entity"
	"fullcycle-auction_go/internal/internal_error"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type AuctionEntityMongo struct {
	Id          string                          `bson:"_id"`
	ProductName string                          `bson:"product_name"`
	Category    string                          `bson:"category"`
	Description string                          `bson:"description"`
	Condition   auction_entity.ProductCondition `bson:"condition"`
	Status      auction_entity.AuctionStatus    `bson:"status"`
	Timestamp   int64                           `bson:"timestamp"`
}

type AuctionRepository struct {
	Collection      *mongo.Collection
	auctionInterval time.Duration
}

func NewAuctionRepository(database *mongo.Database) *AuctionRepository {
	return &AuctionRepository{
		Collection:      database.Collection("auctions"),
		auctionInterval: getAuctionInterval(),
	}
}

// getAuctionInterval lê AUCTION_INTERVAL do ambiente.
// Fallback: 5 minutos. Mesmo padrão de create_bid.go.
func getAuctionInterval() time.Duration {
	duration, err := time.ParseDuration(os.Getenv("AUCTION_INTERVAL"))
	if err != nil {
		return time.Minute * 5
	}
	return duration
}

func (ar *AuctionRepository) CreateAuction(
	ctx context.Context,
	auctionEntity *auction_entity.Auction) *internal_error.InternalError {
	auctionEntityMongo := &AuctionEntityMongo{
		Id:          auctionEntity.Id,
		ProductName: auctionEntity.ProductName,
		Category:    auctionEntity.Category,
		Description: auctionEntity.Description,
		Condition:   auctionEntity.Condition,
		Status:      auctionEntity.Status,
		Timestamp:   auctionEntity.Timestamp.Unix(),
	}

	_, err := ar.Collection.InsertOne(ctx, auctionEntityMongo)
	if err != nil {
		logger.Error("Error trying to insert auction", err)
		return internal_error.NewInternalServerError("Error trying to insert auction")
	}

	time.AfterFunc(ar.auctionInterval, func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		filter := bson.M{"_id": auctionEntity.Id}
		update := bson.M{"$set": bson.M{"status": auction_entity.Completed}}

		if _, err := ar.Collection.UpdateOne(closeCtx, filter, update); err != nil {
			logger.Error("Error trying to close auction", err)
		}
	})

	return nil
}
```

- [ ] **Step 2: Verificar que o projeto compila**

```bash
go build ./...
```

Esperado: sem erros.

- [ ] **Step 3: Executar o teste**

```bash
go test ./internal/infra/database/auction/... -run TestCreateAuction_AutoClose -v -timeout 60s
```

Esperado: **PASS** — o container MongoDB sobe, leilão é criado com status `Active`, após ~2.5s o status é `Completed`.

- [ ] **Step 4: Commit da implementação**

```bash
git add internal/infra/database/auction/create_auction.go
git commit -m "feat: add automatic auction closing via goroutine

Dispara time.AfterFunc após CreateAuction insert. Quando AUCTION_INTERVAL
expira, goroutine faz UpdateOne no MongoDB setando status=Completed.
Fallback de 5 minutos quando AUCTION_INTERVAL não está configurado."
```

---

## Task 4: Criar README com instruções de uso

**Files:**
- Create: `README.md`

O enunciado exige README com instruções de execução, configuração de variáveis de ambiente e Docker.

- [ ] **Step 1: Criar `README.md` na raiz do projeto**

```markdown
# Labs Auction — GoExpert

Sistema de leilões com fechamento automático via Goroutines.

## Pré-requisitos

- Docker e Docker Compose instalados
- Go 1.20+ (apenas para desenvolvimento local)

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
```

- [ ] **Step 2: Commit do README**

```bash
git add README.md
git commit -m "docs: add README with setup and environment variable instructions"
```

---

## Task 5: Verificação final

- [ ] **Step 1: Rodar todos os testes do projeto**

```bash
go test ./... -v -timeout 60s
```

Esperado: `TestCreateAuction_AutoClose` passa. Nenhum outro teste quebrou.

- [ ] **Step 2: Verificar build Docker**

```bash
docker compose build
```

Esperado: build sem erros.

- [ ] **Step 3: Verificar histórico de commits**

```bash
git log --oneline
```

Esperado: histórico limpo com os commits das tasks anteriores.

---

## Referência rápida: fluxo completo

```
POST /auction
  └─ CreateAuction (usecase)
       └─ AuctionRepository.CreateAuction
            ├─ InsertOne(ctx, entity)        → MongoDB: status=Active
            └─ time.AfterFunc(interval, fn)  → goroutine agendada
                 (dispara após AUCTION_INTERVAL)
                 └─ UpdateOne(filter, update) → MongoDB: status=Completed
                      context.WithTimeout(10s)
                      logger.Error se falhar
```
