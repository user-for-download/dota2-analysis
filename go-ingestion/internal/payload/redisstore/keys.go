package redisstore

type keys struct {
	prefix string
}

func (k keys) blob(key string) string { return k.prefix + ":" + key }
