# Model Manager - Go Service for vLLM Hot-Swapping

A lightweight Go service that orchestrates vLLM model switching using the sleep/wake API. This service manages multiple vLLM instances and coordinates putting models to sleep while waking up others.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Model Manager                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │   Handlers   │→ │   Switcher   │→ │ vLLM Client  │  │
│  │   (HTTP)     │  │   (Logic)    │  │   (HTTP)     │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
  ┌──────────┐        ┌──────────┐        ┌──────────┐
  │  vLLM 1  │        │  vLLM 2  │        │  vLLM N  │
  │ (Active) │        │ (Sleeping)│       │ (Sleeping)│
  └──────────┘        └──────────┘        └──────────┘
```

## Project Structure

```
model-manager/
├── cmd/
│   └── switcher/
│       └── main.go           # Entry point: initializes services, starts HTTP server
├── internal/
│   ├── config/
│   │   └── config.go         # Loads config.yaml into Go structs
│   ├── handlers/
│   │   └── handlers.go       # HTTP handlers: Health, GetModels, SwitchModel
│   ├── switcher/
│   │   └── switcher.go       # Core logic: orchestrates sleep/wake operations
│   └── vllm/
│       └── client.go         # HTTP client for vLLM API (Sleep, WakeUp, Health)
├── pkg/
│   └── models/
│       └── models.go         # Shared data structures (Model, Config, etc.)
├── config.yaml               # Model definitions and switching parameters
├── Dockerfile                # Multi-stage build for production
└── go.mod                    # Go module dependencies
```

## Code Flow

### 1. Startup (`cmd/switcher/main.go`)
```go
main()
  → Load config.yaml
  → Initialize Switcher
  → Initialize Handlers
  → Setup Gin routes
  → Start HTTP server on port 9000
```

### 2. Model Switch Request (`POST /switch`)
```go
handlers.SwitchModel()
  → Parse JSON request
  → Call switcher.SwitchModel()
    → Lock mutex (thread-safe)
    → Find active model
    → Sleep active model
      → Determine sleep level (1 or 2 based on RAM)
      → POST /sleep?level=N to vLLM
    → Wake up target model
      → POST /wake_up to vLLM
      → Retry health checks until ready
    → Update model statuses
    → Unlock mutex
  → Return JSON response
```

### 3. Health Check Loop (`internal/switcher/switcher.go`)
```go
activateModel()
  → POST /wake_up to vLLM
  → Loop with retries:
    → GET /health
    → If healthy: return success
    → If not ready: sleep and retry
    → If max retries: return error
```

## Key Components

### `pkg/models/models.go`
Defines all data structures:
- `Model`: Represents a vLLM instance (ID, Name, Endpoint, Status)
- `Config`: Configuration from config.yaml
- `ModelStatus`: Enum (active, sleeping, switching, error)
- `SwitchRequest/Response`: API request/response types

### `internal/config/config.go`
- Loads `config.yaml` using `gopkg.in/yaml.v2`
- Validates configuration structure
- Returns `*models.Config` object

### `internal/vllm/client.go`
HTTP client wrapper for vLLM endpoints:

```go
type Client struct {
    baseURL string
    httpClient *http.Client
}

// Core methods:
func (c *Client) Sleep(level int) error
func (c *Client) WakeUp() error
func (c *Client) Health() error
func (c *Client) IsSleeping() (bool, error)
```

**⚠️ IMPORTANT: Development Mode Only**

The sleep mode endpoints used by this client are **ONLY available when vLLM is running in development mode**:
- Required environment variable: `VLLM_SERVER_DEV_MODE=1`
- Required server flag: `--enable-sleep-mode`
- Endpoints: `POST /sleep`, `POST /wake_up`, `GET /is_sleeping`

These are **development endpoints** per vLLM documentation and should not be exposed to end users in production. This system is designed for internal/development use cases (RLHF training, model testing, cost optimization in dev environments).

### `internal/switcher/switcher.go`
**Core business logic** - most important file for understanding the system:

```go
type Switcher struct {
    mu      sync.RWMutex  // Thread-safe concurrent access
    config  *models.Config
    clients map[string]*vllm.Client  // Model ID → vLLM client
}

// Main public methods:
func (s *Switcher) GetModels() models.ModelsResponse
func (s *Switcher) SwitchModel(ctx context.Context, targetModelID string) error

// Internal helpers:
func (s *Switcher) sleepModel(ctx context.Context, modelID string) error
func (s *Switcher) activateModel(ctx context.Context, modelID string) error
func (s *Switcher) determineSleepLevel(model *models.Model) int
```

**Sleep Level Selection Logic:**
```go
func (s *Switcher) determineSleepLevel(model *models.Model) int {
    if s.config.Switching.AvailableRAMGB >= 64.0 {
        return 1  // Offload to CPU RAM (fast wake-up)
    }
    return 2  // Discard weights (slow wake-up, no RAM needed)
}
```

### `internal/handlers/handlers.go`
Gin HTTP handlers that wrap Switcher methods:

```go
type Handler struct {
    switcher *switcher.Switcher
}

// Endpoints:
func (h *Handler) Health(c *gin.Context)      // GET /health
func (h *Handler) GetModels(c *gin.Context)    // GET /models
func (h *Handler) SwitchModel(c *gin.Context)  // POST /switch
```

## Configuration File (`config.yaml`)

```yaml
models:
  - id: "qwen3-vl-30b"              # Unique ID for API calls
    name: "Qwen3 VL 30B"            # Human-readable name
    endpoint: "http://vllm-qwen:8000"  # vLLM service URL
    status: "active"                # Initial status

  - id: "gpt-oss-20b"
    name: "GPT-OSS 20B"
    endpoint: "http://vllm-gptoss:8000"
    status: "sleeping"

switching:
  available_ram_gb: 128.0           # Total system RAM
  max_retries: 30                   # Health check retry count
  health_check_interval_seconds: 2  # Delay between retries
```

## Building and Running

### Local Development
```bash
# Install dependencies
go mod download

# Build
go build -o switcher ./cmd/switcher

# Run (requires vLLM instances to be running)
./switcher

# Or specify config path
CONFIG_PATH=/path/to/config.yaml ./switcher
```

### Docker Build
```bash
# Build image
docker build -t model-manager:latest .

# Run container
docker run -p 9000:9000 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  model-manager:latest
```

### With Docker Compose
```bash
cd ../docker
docker compose build model-manager
docker compose up model-manager
```

## API Reference

### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "healthy"
}
```

### GET /models
List all models and their current status.

**Response:**
```json
{
  "models": [
    {
      "id": "qwen3-vl-30b",
      "name": "Qwen3 VL 30B",
      "container_name": "vllm-qwen3-vl-30b",
      "port": 8000,
      "host_port": 8001,
      "gpu_memory_gb": 57.0,
      "startup_mode": "active",
      "status": "active",
      "last_active": "2023-11-20T10:00:00Z"
    },
    {
      "id": "gpt-oss-20b",
      "name": "GPT-OSS 20B",
      "container_name": "vllm-gpt-oss-20b",
      "port": 8000,
      "host_port": 8004,
      "gpu_memory_gb": 36.0,
      "startup_mode": "disabled",
      "status": "sleeping"
    }
  ],
  "active_model": "qwen3-vl-30b"
}
```

**Status values:**
- `active`: Model is loaded on GPU and ready for inference
- `sleeping`: Model is asleep (offloaded or discarded)
- `switching`: Model is currently transitioning
- `error`: Model encountered an error
- `disabled`: Model is disabled in configuration

### POST /switch
Switch to a different model.

**Request:**
```json
{
  "model_id": "gpt-oss-20b"
}
```

**Response (Success):**
```json
{
  "status": "success",
  "active_model": "gpt-oss-20b"
}
```

**Response (Error):**
```json
{
  "error": "model not found: invalid-id"
}
```

## Extending the Service

### Adding New Endpoints

1. Add handler method in `internal/handlers/handlers.go`:
```go
func (h *Handler) MyNewEndpoint(c *gin.Context) {
    // Handler logic
    c.JSON(http.StatusOK, gin.H{"result": "data"})
}
```

2. Register route in `cmd/switcher/main.go`:
```go
r.GET("/my-endpoint", handler.MyNewEndpoint)
```

### Adding New vLLM Client Methods

Edit `internal/vllm/client.go`:
```go
func (c *Client) MyNewMethod() error {
    resp, err := c.httpClient.Post(
        c.baseURL+"/new-endpoint",
        "application/json",
        nil,
    )
    // Handle response
    return nil
}
```

### Modifying Switch Logic

The switching algorithm is in `internal/switcher/switcher.go`:

```go
func (s *Switcher) SwitchModel(ctx context.Context, targetModelID string) error {
    // Current flow:
    // 1. Validate target model
    // 2. Lock mutex
    // 3. Find active model
    // 4. Sleep active model
    // 5. Wake target model
    // 6. Update statuses
    
    // Modify any step here...
}
```

**Example: Add pre-warming:**
```go
func (s *Switcher) SwitchModel(ctx context.Context, targetModelID string) error {
    // ... existing code ...
    
    // Before waking up, pre-warm the model
    if err := s.prewarmModel(ctx, targetModelID); err != nil {
        return fmt.Errorf("prewarm failed: %w", err)
    }
    
    // Continue with wake-up...
}
```

### Adding Metrics/Logging

Add structured logging with a library like `logrus` or `zap`:

```go
import "github.com/sirupsen/logrus"

func (s *Switcher) SwitchModel(ctx context.Context, targetModelID string) error {
    logrus.WithFields(logrus.Fields{
        "target_model": targetModelID,
        "timestamp": time.Now(),
    }).Info("Starting model switch")
    
    // ... switching logic ...
    
    logrus.Info("Model switch completed successfully")
}
```

## Testing

### Unit Tests
```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test ./internal/switcher/
```

### Integration Tests
```bash
# Start vLLM instances first
cd ../docker
docker compose up vllm-qwen vllm-gptoss -d

# Run integration tests
cd ../model-manager
go test -tags=integration ./...
```

### Manual Testing
```bash
# Health check
curl http://localhost:9000/health

# List models
curl http://localhost:9000/models

# Switch model
curl -X POST http://localhost:9000/switch \
  -H "Content-Type: application/json" \
  -d '{"model_id":"gpt-oss-20b"}'

# Check vLLM sleep status
curl http://localhost:8001/is_sleeping
curl http://localhost:8002/is_sleeping
```

## Debugging

### Enable Debug Logging
```go
// In cmd/switcher/main.go
gin.SetMode(gin.DebugMode)  // Shows detailed HTTP logs
```

### Common Issues

**Port already in use:**
```bash
# Find process using port 9000
lsof -i :9000

# Kill process
kill -9 <PID>
```

**Config file not found:**
```bash
# Set environment variable
export CONFIG_PATH=/absolute/path/to/config.yaml
./switcher
```

**vLLM endpoint unreachable:**
- Check Docker network: `docker network inspect homegpt-network`
- Verify container names match config endpoints
- Test direct connection: `curl http://vllm-qwen:8000/health`

## Dependencies

```go
require (
    github.com/gin-gonic/gin v1.9.1         // HTTP framework
    gopkg.in/yaml.v2 v2.4.0                 // YAML parsing
)
```

Add new dependencies:
```bash
go get github.com/some/package
go mod tidy
```

## Performance Considerations

- **Mutex locking:** Only one switch operation at a time (prevents race conditions)
- **HTTP timeouts:** Default 30s for vLLM operations
- **Retry logic:** Configurable retries prevent false failures
- **Memory overhead:** Minimal (~10MB for the Go service itself)

## Future Improvements

- [ ] WebSocket support for real-time status updates
- [ ] Concurrent health checks for faster model discovery
- [ ] Model preloading/warming strategies
- [ ] Circuit breaker pattern for failing vLLM instances
- [ ] Prometheus metrics export
- [ ] OpenTelemetry tracing
- [ ] Database persistence for model state
- [ ] Admin API for runtime config updates
