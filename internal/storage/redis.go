package storage

import "github.com/redis/go-redis/v9"

func NewRedis(connStr string) (*redis.Client, error) {
	opts, err := redis.ParseURL(connStr)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)
	return client, nil
}
