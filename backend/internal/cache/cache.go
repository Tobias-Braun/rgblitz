package cache

import (
	"time"

	gocache "github.com/patrickmn/go-cache"
)

const (
	TTLTournamentActive   = 60 * time.Second
	TTLTournamentFinished = 24 * time.Hour
	TTLTournamentMiss     = 15 * time.Minute
	TTLDivisionTeams      = 5 * time.Minute
	TTLPlayerProfile      = 60 * time.Second
	TTLImageDisk          = 7 * 24 * time.Hour
)

type Cache struct {
	c *gocache.Cache
}

func New() *Cache {
	return &Cache{c: gocache.New(gocache.NoExpiration, 10*time.Minute)}
}

func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.c.Set(key, value, ttl)
}

func (c *Cache) Get(key string) (any, bool) {
	return c.c.Get(key)
}

func (c *Cache) Delete(key string) {
	c.c.Delete(key)
}
