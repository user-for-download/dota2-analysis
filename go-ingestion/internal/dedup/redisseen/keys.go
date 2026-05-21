package redisseen

type keys struct {
	prefix string
}

func (k keys) set() string              { return k.prefix + ":seen" }
func (k keys) seenKey(id string) string { return k.prefix + ":seen:" + id }
func (k keys) dlqGuard() string         { return k.prefix + ":dlq:guard" }
