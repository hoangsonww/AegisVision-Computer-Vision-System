module github.com/aegisvision/tools/integration

go 1.26.0

require (
	github.com/aegisvision/pkg/agent v0.0.0
	github.com/aegisvision/pkg/bus v0.0.0
	github.com/aegisvision/pkg/intelligence v0.0.0
	github.com/aegisvision/pkg/llm v0.0.0
)

require (
	github.com/antithesishq/antithesis-sdk-go v0.7.0-default-no-op // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/nats-io/jwt/v2 v2.8.1 // indirect
	github.com/nats-io/nats-server/v2 v2.14.1 // indirect
	github.com/nats-io/nats.go v1.51.0 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.25 // indirect
	github.com/segmentio/kafka-go v0.4.51 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.15.0 // indirect
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
