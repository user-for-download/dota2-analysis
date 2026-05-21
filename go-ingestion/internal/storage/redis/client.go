package redis

import (
	"context"
	"fmt"
	"runtime"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func nonZero[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}

type Config struct {
	Addrs    []string
	Password string
	DB       int

	PoolSize        int
	MinIdleConns    int
	MaxActiveConns  int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration

	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	ReadOnly bool
}

type Client struct {
	master  *goredis.Client
	replica *goredis.Client
}

func New(cfg Config) (*Client, error) {
	if len(cfg.Addrs) == 0 {
		return nil, fmt.Errorf("redis: at least one address required")
	}

	defaultPoolSize := runtime.NumCPU() * 10

	opts := &goredis.Options{
		Addr:            cfg.Addrs[0],
		Password:        cfg.Password,
		DB:              cfg.DB,
		PoolSize:        nonZero(cfg.PoolSize, defaultPoolSize),
		MinIdleConns:    nonZero(cfg.MinIdleConns, 10),
		MaxActiveConns:  nonZero(cfg.MaxActiveConns, defaultPoolSize),
		ConnMaxLifetime: nonZero(cfg.ConnMaxLifetime, 30*time.Minute),
		ConnMaxIdleTime: nonZero(cfg.ConnMaxIdleTime, 5*time.Minute),
		DialTimeout:     nonZero(cfg.DialTimeout, 5*time.Second),
		ReadTimeout:     nonZero(cfg.ReadTimeout, 3*time.Second),
		WriteTimeout:    nonZero(cfg.WriteTimeout, 3*time.Second),
	}

	master := goredis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := master.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	var replica *goredis.Client
	if cfg.ReadOnly && len(cfg.Addrs) > 1 {
		replicaOpts := *opts
		replicaOpts.Addr = cfg.Addrs[1]
		replica = goredis.NewClient(&replicaOpts)
	}

	return &Client{master: master, replica: replica}, nil
}

func (c *Client) Master() *goredis.Client {
	return c.master
}

func (c *Client) Read() *goredis.Client {
	if c.replica != nil {
		return c.replica
	}
	return c.master
}

func (c *Client) Close() error {
	var errs []error
	if err := c.master.Close(); err != nil {
		errs = append(errs, err)
	}
	if c.replica != nil {
		if err := c.replica.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("redis close: %v", errs)
	}
	return nil
}
