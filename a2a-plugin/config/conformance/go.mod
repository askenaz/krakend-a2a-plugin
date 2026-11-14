module krakend-a2a.local/config-conformance

go 1.26

require (
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	krakend-a2a.local/core v0.0.0
)

require golang.org/x/text v0.14.0 // indirect

replace krakend-a2a.local/core => ../../core
