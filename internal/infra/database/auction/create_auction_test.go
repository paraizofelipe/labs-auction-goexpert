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
