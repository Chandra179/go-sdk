package kafka

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gosdk/pkg/logger"

	"github.com/riferrei/srclient"
)

// SchemaFormat represents the serialization format for schemas
type SchemaFormat string

const (
	SchemaFormatAvro     SchemaFormat = "avro"
	SchemaFormatJSON     SchemaFormat = "json"
	SchemaFormatProtobuf SchemaFormat = "protobuf"
)

// SchemaRegistry provides integration with Confluent Schema Registry for
// data governance and schema compatibility. It supports Avro, JSON Schema,
// and Protobuf formats.
type SchemaRegistry struct {
	client   srclient.ISchemaRegistryClient
	logger   logger.Logger
	cache    map[string]*srclient.Schema
	cacheMu  sync.RWMutex
	cacheTTL time.Duration
	format   SchemaFormat
}

// SchemaRegistryConfig holds configuration for the Schema Registry client
type SchemaRegistryConfig struct {
	Enabled  bool          // Whether to enable Schema Registry
	URL      string        // Schema Registry URL (e.g., http://localhost:8081)
	Username string        // Optional: Basic auth username
	Password string        // Optional: Basic auth password
	Format   SchemaFormat  // Schema format (avro, json, protobuf)
	CacheTTL time.Duration // Cache TTL for schemas (default: 1 hour)
}

// SchemaMetadata holds schema information for a message
type SchemaMetadata struct {
	SchemaID int // Schema ID from registry
	Format   SchemaFormat
	Subject  string // Schema subject (typically topic name)
	Version  int    // Schema version
}

// SchemaMessage wraps a Kafka message with schema information
type SchemaMessage struct {
	Message
	SchemaID int         // Schema ID from registry
	Data     interface{} // Decoded/native data
}

// NewSchemaRegistry creates a new Schema Registry client with caching support.
// It automatically configures authentication if credentials are provided.
func NewSchemaRegistry(cfg SchemaRegistryConfig, logger logger.Logger) (*SchemaRegistry, error) {
	client := srclient.CreateSchemaRegistryClient(cfg.URL)

	if cfg.Username != "" && cfg.Password != "" {
		client.SetCredentials(cfg.Username, cfg.Password)
	}

	// Set default cache TTL
	cacheTTL := cfg.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = 1 * time.Hour
	}

	// Set default format
	format := cfg.Format
	if format == "" {
		format = SchemaFormatAvro
	}

	return &SchemaRegistry{
		client:   client,
		logger:   logger,
		cache:    make(map[string]*srclient.Schema),
		cacheTTL: cacheTTL,
		format:   format,
	}, nil
}

// EncodeMessage encodes data using the specified schema from the registry.
// It fetches the schema by ID and serializes the data according to the Confluent wire format:
// - Byte 0: Magic byte (0)
// - Bytes 1-4: Schema ID (big-endian int)
// - Remaining bytes: Serialized data
func (sr *SchemaRegistry) EncodeMessage(ctx context.Context, schemaID int, data interface{}) ([]byte, error) {
	schema, err := sr.getSchemaWithCache(ctx, schemaID)
	if err != nil {
		sr.logger.Error(ctx, "Failed to get schema from registry",
			logger.Field{Key: "schema_id", Value: schemaID},
			logger.Field{Key: "error", Value: err})
		return nil, fmt.Errorf("%w: failed to get schema %d: %v", ErrSchemaRegistry, schemaID, err)
	}

	// Serialize the data using the schema codec
	serialized, err := schema.Codec().BinaryFromNative(nil, data)
	if err != nil {
		sr.logger.Error(ctx, "Failed to encode message with schema",
			logger.Field{Key: "schema_id", Value: schemaID},
			logger.Field{Key: "error", Value: err})
		return nil, fmt.Errorf("%w: failed to encode data: %v", ErrSchemaEncode, err)
	}

	// Build Confluent wire format message
	buf := new(bytes.Buffer)

	// Magic byte (0)
	if err := buf.WriteByte(0); err != nil {
		return nil, fmt.Errorf("failed to write magic byte: %w", err)
	}

	// Schema ID (4 bytes, big-endian)
	if err := binary.Write(buf, binary.BigEndian, int32(schemaID)); err != nil {
		return nil, fmt.Errorf("failed to write schema ID: %w", err)
	}

	// Serialized data
	if _, err := buf.Write(serialized); err != nil {
		return nil, fmt.Errorf("failed to write serialized data: %w", err)
	}

	sr.logger.Debug(ctx, "Message encoded successfully",
		logger.Field{Key: "schema_id", Value: schemaID})

	return buf.Bytes(), nil
}

// DecodeMessage decodes a message using the embedded schema ID from the Confluent wire format.
// It extracts the schema ID from bytes 1-4 and uses the corresponding schema from the registry.
func (sr *SchemaRegistry) DecodeMessage(ctx context.Context, data []byte) (interface{}, int, error) {
	if len(data) < 5 {
		return nil, 0, fmt.Errorf("%w: message too short to contain schema ID", ErrSchemaDecode)
	}

	// Check magic byte
	if data[0] != 0 {
		return nil, 0, fmt.Errorf("%w: invalid magic byte %d, expected 0", ErrSchemaDecode, data[0])
	}

	// Extract schema ID (bytes 1-4, big-endian)
	schemaID := int(binary.BigEndian.Uint32(data[1:5]))

	schema, err := sr.getSchemaWithCache(ctx, schemaID)
	if err != nil {
		sr.logger.Error(ctx, "Failed to get schema for decoding",
			logger.Field{Key: "schema_id", Value: schemaID},
			logger.Field{Key: "error", Value: err})
		return nil, schemaID, fmt.Errorf("%w: failed to get schema %d: %v", ErrSchemaRegistry, schemaID, err)
	}

	// Decode the remaining data
	decoded, _, err := schema.Codec().NativeFromBinary(data[5:])
	if err != nil {
		sr.logger.Error(ctx, "Failed to decode message",
			logger.Field{Key: "schema_id", Value: schemaID},
			logger.Field{Key: "error", Value: err})
		return nil, schemaID, fmt.Errorf("%w: failed to decode data: %v", ErrSchemaDecode, err)
	}

	sr.logger.Debug(ctx, "Message decoded successfully",
		logger.Field{Key: "schema_id", Value: schemaID})

	return decoded, schemaID, nil
}

// GetSchema retrieves a schema by ID directly from the registry (no caching)
func (sr *SchemaRegistry) GetSchema(ctx context.Context, schemaID int) (*srclient.Schema, error) {
	return sr.client.GetSchema(schemaID)
}

// GetLatestSchema retrieves the latest version of a schema for a subject
func (sr *SchemaRegistry) GetLatestSchema(ctx context.Context, subject string) (*srclient.Schema, error) {
	return sr.client.GetLatestSchema(subject)
}

// GetSchemaByVersion retrieves a specific version of a schema for a subject
func (sr *SchemaRegistry) GetSchemaByVersion(ctx context.Context, subject string, version int) (*srclient.Schema, error) {
	return sr.client.GetSchemaByVersion(subject, version)
}

// RegisterSchema registers a new schema for a subject and returns the schema ID
func (sr *SchemaRegistry) RegisterSchema(ctx context.Context, subject string, schema string) (*srclient.Schema, error) {
	var schemaType srclient.SchemaType

	switch sr.format {
	case SchemaFormatAvro:
		schemaType = srclient.Avro
	case SchemaFormatJSON:
		schemaType = srclient.Json
	case SchemaFormatProtobuf:
		schemaType = srclient.Protobuf
	default:
		schemaType = srclient.Avro
	}

	return sr.client.CreateSchema(subject, schema, schemaType)
}

// CheckCompatibility checks if a new schema is compatible with the existing schema for a subject
func (sr *SchemaRegistry) CheckCompatibility(ctx context.Context, subject string, newSchema string, version int) (bool, error) {
	// Get schema type from registry format
	var schemaType srclient.SchemaType
	switch sr.format {
	case SchemaFormatJSON:
		schemaType = srclient.Json
	case SchemaFormatProtobuf:
		schemaType = srclient.Protobuf
	default:
		schemaType = srclient.Avro
	}

	versionStr := fmt.Sprintf("%d", version)
	return sr.client.IsSchemaCompatible(subject, newSchema, versionStr, schemaType)
}

// getSchemaWithCache retrieves a schema from cache or fetches from registry
func (sr *SchemaRegistry) getSchemaWithCache(ctx context.Context, schemaID int) (*srclient.Schema, error) {
	cacheKey := fmt.Sprintf("%d", schemaID)

	// Try cache first
	sr.cacheMu.RLock()
	if schema, exists := sr.cache[cacheKey]; exists {
		sr.cacheMu.RUnlock()
		return schema, nil
	}
	sr.cacheMu.RUnlock()

	// Fetch from registry
	schema, err := sr.client.GetSchema(schemaID)
	if err != nil {
		return nil, err
	}

	// Cache the schema
	sr.cacheMu.Lock()
	sr.cache[cacheKey] = schema
	sr.cacheMu.Unlock()

	return schema, nil
}

// ClearCache clears the schema cache
func (sr *SchemaRegistry) ClearCache() {
	sr.cacheMu.Lock()
	sr.cache = make(map[string]*srclient.Schema)
	sr.cacheMu.Unlock()
}

// CacheStats returns current cache statistics
func (sr *SchemaRegistry) CacheStats() CacheStats {
	sr.cacheMu.RLock()
	defer sr.cacheMu.RUnlock()

	return CacheStats{
		Size: len(sr.cache),
	}
}

// CacheStats holds cache statistics
type CacheStats struct {
	Size int
}

// ValidateSchema validates that a schema string is valid for the configured format
func (sr *SchemaRegistry) ValidateSchema(schema string) error {
	switch sr.format {
	case SchemaFormatJSON:
		var js interface{}
		if err := json.Unmarshal([]byte(schema), &js); err != nil {
			return fmt.Errorf("%w: invalid JSON schema: %v", ErrInvalidSchema, err)
		}
		return nil
	case SchemaFormatAvro, SchemaFormatProtobuf:
		// For Avro and Protobuf, basic validation is handled during registration
		// More detailed validation would require specific libraries
		return nil
	default:
		return fmt.Errorf("%w: unsupported schema format: %s", ErrInvalidSchema, sr.format)
	}
}

// Format returns the configured schema format
func (sr *SchemaRegistry) Format() SchemaFormat {
	return sr.format
}
