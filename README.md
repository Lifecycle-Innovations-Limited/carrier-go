# carrier-go

Go client for the Carrier API, the hosted MCP server, and the `carrier` CLI. `Intents.Create` posts to `/v1/connectivity/intents`. `Call` uses an OpenAPI operation id. `NewMCP` speaks Streamable HTTP. `NewCLI` runs the binary with an argument list. `RESTTools`, `Tools` on MCP, and `Tools` on CLI return plain definitions a service can hand to an agent. The price on a product is catalogue data. Neither an intent nor a product debits a wallet.

```bash
go get github.com/Lifecycle-Innovations-Limited/carrier-go@latest
```

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/Lifecycle-Innovations-Limited/carrier-go"
)

func main() {
	client, err := carrier.New(os.Getenv("CARRIER_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	intent, err := client.Intents.Create(context.Background(), carrier.IntentCreateParams{
		Plan:     carrier.StringPtr("test_global_1gb"),
		Metadata: map[string]string{"order": "123"},
	}, "order_123")
	if err != nil {
		log.Fatal(err)
	}
	log.Print(intent.ID)
}
```

The import name is `carrier`. `test_global_1gb` is a practice plan. The call succeeds, `Livemode` is false, and the network is not used. The same idempotency key and body return the original intent.

Webhook deliveries use `Carrier-Signature: t=<unix>,v1=<hex>` over `{timestamp}.{body}`. The endpoint secret (`whsec_…`) is returned once, on create.

```go
ok := carrier.VerifyWebhookSignature(rawBody, header, secret, carrier.DefaultWebhookToleranceSeconds, time.Now())
```

`*carrier.Error` carries `Status`, `Code`, and `Type` from the API error envelope. A blank API key or idempotency key fails before a request is sent.

Every public operation:

```go
raw, err := client.Call(ctx, "list_reseller_accounts", carrier.CallParams{})
raw, err = client.Call(ctx, "create_connectivity_intent", carrier.CallParams{
	Body:           map[string]any{"plan": "test_global_1gb"},
	IdempotencyKey: "order_123",
})
```

`Security` on each catalog entry is `bearer`, `public`, `internal`, or `storefront`. A customer API key is `bearer`. The two internal routes do not accept that key.

```go
session, err := carrier.NewMCP(os.Getenv("ESIMVAULT_API_TOKEN"))
tools, err := session.ListTools(ctx)
result, err := session.CallTool(ctx, "list_subscribers", map[string]any{"limit": 5})

cli, err := carrier.NewCLI()
out, err := cli.Run(ctx, []string{"subscribers", "list", "--limit", "5"})
defs, err := client.RESTTools()
```

`carrier.WithMCPAuth("bearer")` sends an OAuth access token or a Clerk organization API key. `NewCLI` never invokes a shell. `WithCLIStrict(false)` runs a command that is not in the catalog.

Go 1.22 or newer. The module uses the standard library only. `go get github.com/Lifecycle-Innovations-Limited/carrier-go@latest` installs the public module.
