package transport

// ServerIPAddress is one of the server's global addresses (alpha.128, G14).
type ServerIPAddress struct {
	Address   string `json:"address"`
	Prefix    int    `json:"prefix"`
	Interface string `json:"interface"`
	Family    string `json:"family"`
	Private   bool   `json:"private"`
}

type ServerIPAddressesResponse struct {
	Addresses []ServerIPAddress `json:"addresses"`
	Error     string            `json:"error,omitempty"`
}
