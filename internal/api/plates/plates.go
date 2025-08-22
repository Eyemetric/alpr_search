package plates

import (
	"context"
	"fmt"

	"github.com/Eyemetric/alpr_service/internal/repository"
)

func AddPlate(ctx context.Context, plate_doc []byte, repo repository.ALPRRepository) error {
	res, err := repo.IngestPlateRead(ctx, plate_doc)
	fmt.Printf("Result (empty ok): %s\n", res)

	if err != nil {
		return err
	}

	return nil
}
