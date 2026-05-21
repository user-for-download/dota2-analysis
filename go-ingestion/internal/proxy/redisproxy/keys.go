package redisproxy

type keys struct {
	prefix string
}

func (k keys) set() string               { return k.prefix + ":set" }
func (k keys) leased() string            { return k.prefix + ":leased" }
func (k keys) lease(token string) string { return k.prefix + ":lease:" + token }
func (k keys) stats(url string) string   { return k.prefix + ":stats:" + url }
func (k keys) limiter() string           { return k.prefix + ":limiter" }
func (k keys) cooldown() string          { return k.prefix + ":cooldown" }
func (k keys) cooldownEntry(url string) string { return k.prefix + ":cooldown:" + url }
