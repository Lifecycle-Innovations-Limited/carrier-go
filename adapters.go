package carrier

import (
	"context"
	"encoding/json"
	"strings"
)

// Tool is a dependency-free definition an agent runtime can register.
// InputSchema is a JSON Schema object. Execute returns the raw JSON result.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Execute     func(ctx context.Context, arguments map[string]any) (json.RawMessage, error)
}

// RESTTools returns one tool per OpenAPI operation.
func (c *Client) RESTTools() ([]Tool, error) {
	ops, err := Operations()
	if err != nil {
		return nil, err
	}
	tools := make([]Tool, 0, len(ops))
	for _, spec := range ops {
		spec := spec
		schema, err := inputSchema(spec)
		if err != nil {
			return nil, err
		}
		tools = append(tools, Tool{
			Name:        spec.ID,
			Description: spec.Summary + " (" + spec.Method + " " + spec.Path + ", " + spec.Security + ")",
			InputSchema: schema,
			Execute: func(ctx context.Context, arguments map[string]any) (json.RawMessage, error) {
				params, err := argumentsToCall(spec, arguments)
				if err != nil {
					return nil, err
				}
				return c.Call(ctx, spec.ID, params)
			},
		})
	}
	return tools, nil
}

// Tools returns one tool per MCP tool advertised by ListTools.
func (m *MCP) Tools(ctx context.Context) ([]Tool, error) {
	listed, err := m.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	tools := make([]Tool, 0, len(listed))
	for _, item := range listed {
		record, ok := item.(map[string]any)
		name, nameOK := record["name"].(string)
		if !ok || !nameOK || name == "" {
			continue
		}
		description, _ := record["description"].(string)
		if description == "" {
			description = name
		}
		schema := json.RawMessage(`{"type":"object","properties":{}}`)
		if raw, exists := record["inputSchema"]; exists {
			encoded, err := json.Marshal(raw)
			if err != nil {
				return nil, err
			}
			schema = encoded
		}
		toolName := name
		tools = append(tools, Tool{
			Name:        toolName,
			Description: description,
			InputSchema: schema,
			Execute: func(ctx context.Context, arguments map[string]any) (json.RawMessage, error) {
				result, err := m.CallTool(ctx, toolName, arguments)
				if err != nil {
					return nil, err
				}
				return json.Marshal(result)
			},
		})
	}
	return tools, nil
}

// Tools returns one tool per catalogued CLI command.
func (c *CLI) Tools() ([]Tool, error) {
	commands, err := CLICommands()
	if err != nil {
		return nil, err
	}
	tools := make([]Tool, 0, len(commands))
	for _, command := range commands {
		command := command
		tokens := strings.Split(command.Invocation, " ")[1:]
		flags := make([]string, 0, len(command.Options))
		for _, option := range command.Options {
			flags = append(flags, option.Flags)
		}
		flagText := "no flags"
		if len(flags) > 0 {
			flagText = strings.Join(flags, ", ")
		}
		schema := json.RawMessage(`{"type":"object","properties":{"args":{"type":"array","items":{"type":"string"},"description":"Flags and values after the command name."}},"additionalProperties":false}`)
		tools = append(tools, Tool{
			Name:        "cli_" + strings.Join(tokens, "_"),
			Description: command.Summary + " Flags: " + flagText + ".",
			InputSchema: schema,
			Execute: func(ctx context.Context, arguments map[string]any) (json.RawMessage, error) {
				extra, err := stringArgs(arguments["args"])
				if err != nil {
					return nil, err
				}
				result, err := c.Run(ctx, append(append([]string{}, tokens...), extra...))
				if err != nil {
					return nil, err
				}
				return json.Marshal(result)
			},
		})
	}
	return tools, nil
}

func inputSchema(spec Operation) (json.RawMessage, error) {
	properties := map[string]any{}
	required := []string{}
	for _, param := range spec.PathParams {
		properties[param.Name] = map[string]any{"type": "string", "description": "Path " + param.Name}
		if param.Required {
			required = append(required, param.Name)
		}
	}
	for _, param := range spec.QueryParams {
		properties[param.Name] = map[string]any{"type": "string", "description": "Query " + param.Name}
		if param.Required {
			required = append(required, param.Name)
		}
	}
	for _, param := range spec.HeaderParams {
		field := headerFields[param.Name]
		if field == "" {
			field = param.Name
		}
		properties[field] = map[string]any{"type": "string", "description": param.Name}
		if param.Required {
			required = append(required, field)
		}
	}
	if spec.Body {
		properties["body"] = map[string]any{"type": "object", "additionalProperties": true, "description": "JSON request body"}
		if spec.BodyRequired {
			required = append(required, "body")
		}
	}
	return json.Marshal(map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	})
}

func argumentsToCall(spec Operation, arguments map[string]any) (CallParams, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	params := CallParams{
		Path:    map[string]string{},
		Query:   map[string]string{},
		Headers: map[string]string{},
	}
	known := map[string]bool{}
	for _, param := range spec.PathParams {
		known[param.Name] = true
	}
	for _, param := range spec.QueryParams {
		known[param.Name] = true
	}
	headerByField := map[string]string{}
	for _, param := range spec.HeaderParams {
		field := headerFields[param.Name]
		if field == "" {
			field = param.Name
		}
		headerByField[field] = param.Name
		known[field] = true
	}
	if spec.Body {
		known["body"] = true
	}
	for key := range arguments {
		if !known[key] {
			return CallParams{}, invalidRequest("unknown_parameter", "Unknown argument "+key+".")
		}
	}
	for _, param := range spec.PathParams {
		if value, ok := arguments[param.Name]; ok {
			text, ok := value.(string)
			if !ok {
				return CallParams{}, invalidRequest("invalid_parameter", param.Name+" must be a string.")
			}
			params.Path[param.Name] = text
		}
	}
	for _, param := range spec.QueryParams {
		if value, ok := arguments[param.Name]; ok {
			text, ok := value.(string)
			if !ok {
				return CallParams{}, invalidRequest("invalid_parameter", param.Name+" must be a string.")
			}
			params.Query[param.Name] = text
		}
	}
	for field, header := range headerByField {
		value, ok := arguments[field]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return CallParams{}, invalidRequest("invalid_parameter", field+" must be a string.")
		}
		if field == "idempotency_key" {
			params.IdempotencyKey = text
			continue
		}
		params.Headers[header] = text
	}
	if body, ok := arguments["body"]; ok {
		params.Body = body
	}
	return params, nil
}

func stringArgs(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		if typed, typedOK := value.([]string); typedOK {
			return typed, nil
		}
		return nil, invalidRequest("invalid_argument", "CLI tool args must be a list of strings.")
	}
	args := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, invalidRequest("invalid_argument", "CLI tool args must be a list of strings.")
		}
		args = append(args, text)
	}
	return args, nil
}
