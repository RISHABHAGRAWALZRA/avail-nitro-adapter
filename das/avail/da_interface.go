package avail

import (
	"context"
)

type DataAvailabilityWriter interface {
	Store(context.Context, []byte) ([]byte, error)
}

type DataAvailabilityReader interface {
	Read(context.Context, BlobPointer) ([]byte, error)
	// Not the best design decision here
	VerifyAgainstVectorX(blobPointer BlobPointer) (MerkleProofInput, error)
}
