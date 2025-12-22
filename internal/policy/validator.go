package policy

import (
	"bytes"
	"encoding/json"
	"fmt"

	_ "embed"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema/policy_conditions_schema.json
var conditionsSchemaData []byte

//go:embed schema/policy_actions_schema.json
var actionsSchemaData []byte

var (
	conditionsSchema *jsonschema.Schema
	actionsSchema    *jsonschema.Schema
)

func init() {
	if err := loadSchemas(); err != nil {
		panic(fmt.Sprintf("load policy schemas: %v", err))
	}
}

func loadSchemas() error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("conditions.json", bytes.NewReader(conditionsSchemaData)); err != nil {
		return err
	}
	if err := compiler.AddResource("actions.json", bytes.NewReader(actionsSchemaData)); err != nil {
		return err
	}
	var err error
	conditionsSchema, err = compiler.Compile("conditions.json")
	if err != nil {
		return err
	}
	actionsSchema, err = compiler.Compile("actions.json")
	if err != nil {
		return err
	}
	return nil
}

func ValidateConditions(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("conditions required")
	}
	return validateJSON(conditionsSchema, raw)
}

func ValidateActions(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("actions required")
	}
	return validateJSON(actionsSchema, raw)
}

func validateJSON(schema *jsonschema.Schema, raw json.RawMessage) error {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	return schema.Validate(payload)
}
