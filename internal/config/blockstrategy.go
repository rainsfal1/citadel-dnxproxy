package config

// BlockStrategy configures how to respond when a rule blocks a query.
// Action values: "nxdomain", "refuse", "nullroute", "sinkhole".
type BlockStrategy struct {
	Action       string
	SinkholeIPv4 string
	SinkholeIPv6 string
}
