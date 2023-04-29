package storage

import bolt "go.etcd.io/bbolt"

const (
	BucketNameBlocks    = "blocks"
	BucketNameNumberIdx = "numberIdx"
)

func bucketBlocks(tx *bolt.Tx) *bolt.Bucket {
	return tx.Bucket([]byte(BucketNameBlocks))
}

func bucketNumberIdx(tx *bolt.Tx) *bolt.Bucket {
	return tx.Bucket([]byte(BucketNameNumberIdx))
}
