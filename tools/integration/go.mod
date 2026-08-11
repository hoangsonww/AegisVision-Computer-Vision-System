module github.com/aegisvision/tools/integration

go 1.26.0

require (
	github.com/aegisvision/pkg/agent v0.0.0
	github.com/aegisvision/pkg/bus v0.0.0
	github.com/aegisvision/pkg/intelligence v0.0.0
	github.com/aegisvision/pkg/llm v0.0.0
)

require (
	github.com/klauspost/compress v1.18.3 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/nats-io/jwt/v2 v2.5.8 // indirect
	github.com/nats-io/nats-server/v2 v2.10.22 // indirect
	github.com/nats-io/nats.go v1.37.0 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.25 // indirect
	github.com/segmentio/kafka-go v0.4.51 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	golang.org/x/time v0.7.0 // indirect
)

replace (
	github.com/aegisvision/pkg/agent => ../../pkg/agent
	github.com/aegisvision/pkg/bus => ../../pkg/bus
	github.com/aegisvision/pkg/intelligence => ../../pkg/intelligence
	github.com/aegisvision/pkg/llm => ../../pkg/llm
	github.com/aegisvision/pkg/platform => ../../pkg/platform
	github.com/aegisvision/pkg/store => ../../pkg/store
	github.com/aegisvision/services/active-learning-service => ../../services/active-learning-service
	github.com/aegisvision/services/agent-service => ../../services/agent-service
	github.com/aegisvision/services/compliance-evidence-service => ../../services/compliance-evidence-service
	github.com/aegisvision/services/drift-detection-service => ../../services/drift-detection-service
	github.com/aegisvision/services/policy-gate-service => ../../services/policy-gate-service
)
