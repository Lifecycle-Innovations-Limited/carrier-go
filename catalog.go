package carrier

import (
	"embed"
	"encoding/json"
	"sync"
)

//go:embed data/operations.json data/cli.json
var catalogFS embed.FS

// Param is one path, query, or header parameter on an operation.
type Param struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// Operation is one OpenAPI operation the generic caller can invoke.
type Operation struct {
	ID           string  `json:"id"`
	Method       string  `json:"method"`
	Path         string  `json:"path"`
	Tag          string  `json:"tag"`
	Summary      string  `json:"summary"`
	Security     string  `json:"security"`
	Body         bool    `json:"body"`
	BodyRequired bool    `json:"bodyRequired"`
	PathParams   []Param `json:"pathParams"`
	QueryParams  []Param `json:"queryParams"`
	HeaderParams []Param `json:"headerParams"`
}

// CLIOption is one flag on a carrier command.
type CLIOption struct {
	Flags       string `json:"flags"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// CLICommand is one command from the published CLI catalog.
type CLICommand struct {
	Invocation string      `json:"invocation"`
	Domain     string      `json:"domain"`
	Summary    string      `json:"summary"`
	Write      bool        `json:"write"`
	Transport  string      `json:"transport"`
	Tool       *string     `json:"tool"`
	RestMethod *string     `json:"restMethod"`
	RestPath   *string     `json:"restPath"`
	Options    []CLIOption `json:"options"`
}

var headerFields = map[string]string{
	"Idempotency-Key":   "idempotency_key",
	"X-Idempotency-Key": "idempotency_key",
	"X-Confirm-Token":   "confirm_token",
	"stripe-signature":  "stripe_signature",
}

var (
	catalogOnce sync.Once
	catalogOps  []Operation
	catalogCmds []CLICommand
	catalogByID map[string]Operation
	catalogErr  error
)

// Operations returns every operation in the public OpenAPI document.
func Operations() ([]Operation, error) {
	if err := loadCatalog(); err != nil {
		return nil, err
	}
	return catalogOps, nil
}

// CLICommands returns every command in the published CLI catalog.
func CLICommands() ([]CLICommand, error) {
	if err := loadCatalog(); err != nil {
		return nil, err
	}
	return catalogCmds, nil
}

func lookupOperation(id string) (Operation, error) {
	if err := loadCatalog(); err != nil {
		return Operation{}, err
	}
	op, ok := catalogByID[id]
	if !ok {
		return Operation{}, invalidRequest("unknown_operation", "Unknown operation "+id+".")
	}
	return op, nil
}

func loadCatalog() error {
	catalogOnce.Do(func() {
		ops, err := catalogFS.ReadFile("data/operations.json")
		if err != nil {
			catalogErr = invalidRequest("invalid_catalog", err.Error())
			return
		}
		cmds, err := catalogFS.ReadFile("data/cli.json")
		if err != nil {
			catalogErr = invalidRequest("invalid_catalog", err.Error())
			return
		}
		if err := json.Unmarshal(ops, &catalogOps); err != nil {
			catalogErr = invalidRequest("invalid_catalog", "Operation catalog could not be read.")
			return
		}
		if err := json.Unmarshal(cmds, &catalogCmds); err != nil {
			catalogErr = invalidRequest("invalid_catalog", "CLI catalog could not be read.")
			return
		}
		catalogByID = make(map[string]Operation, len(catalogOps))
		for _, op := range catalogOps {
			catalogByID[op.ID] = op
		}
	})
	return catalogErr
}
