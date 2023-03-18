package gen

//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen --config=./types.cfg.yaml https://raw.githubusercontent.com/guacamole-operator/guacamole-rest-api/main/dist/1.5.x/openapi.yaml
//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen --config=./client.cfg.yaml https://raw.githubusercontent.com/guacamole-operator/guacamole-rest-api/main/dist/1.5.x/openapi.yaml
