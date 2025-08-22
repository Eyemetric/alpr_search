package hotlist

import (
	"context"
	"fmt"
	"log"

	"github.com/Eyemetric/alpr_service/internal/repository"
)

func AddHotlist(ctx context.Context, hotlist []byte, repo repository.ALPRRepository) error {

	cnt, err := repo.AddHotlist(ctx, hotlist)
	if err != nil {
		return fmt.Errorf("failed to add hotlist: %w", err)
	}

	log.Printf("added %d items to hotlist\n", cnt)
	return nil

}
