// Package carrier is the Go client for the Carrier API, MCP server, and CLI.
//
// Intents issue one eSIM. Client.Call reaches every public OpenAPI operation.
// MCP speaks Streamable HTTP at https://mcp.carrier.llc/mcp. CLI runs the
// carrier binary with an argument list. The price on a product is catalogue
// data. Neither an intent nor a product debits a wallet.
package carrier
