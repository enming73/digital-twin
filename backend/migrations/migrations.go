package migrations

import _ "embed"

//go:embed 001_w6.sql
var Initial string

//go:embed 002_v11_assets.sql
var AssetRegistry string
