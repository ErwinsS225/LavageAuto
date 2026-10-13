package main

import _ "embed"

// schéma SQL embarqué dans le binaire : une seule chose à déployer.
//
//go:embed schema.sql
var schemaSQL string
