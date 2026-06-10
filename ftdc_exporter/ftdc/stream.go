package ftdc

import (
	"context"
	"os"
)

func streamFTDCMetricsInBatches(ctx context.Context, path string, includePatterns map[string]struct{}, batchSize, buffer int) (<-chan StreamBatch, <-chan error) {
	file, err := os.Open(path)
	if err != nil {
		errc := make(chan error, 1)
		errc <- err
		out := make(chan StreamBatch)
		close(out)
		return out, errc
	}

	out := make(chan StreamBatch, buffer)
	errc := make(chan error, 1)

	iter := readFTDCData(ctx, file)

	go func() {
		defer close(out)
		defer close(errc)
		defer file.Close()
		defer iter.Close()

		for {
			sb := StreamBatch{
				Items: make([]map[string]interface{}, 0, batchSize),
			}

			for i := 0; i < batchSize; i++ {
				if iter.Next() {
					sb.Items = append(sb.Items, iter.NormalisedDocument(includePatterns))
				} else {
					break
				}
			}
			if len(sb.Items) == 0 {
				return
			}
			select {
			case out <- sb:
			case <-ctx.Done():
				errc <- ctx.Err()
				return
			}
		}
	}()

	return out, errc
}
